# Failure Modes and Design Trade-offs

For each failure scenario: what the code does today, how it should behave in production, and where it stands.

## Status summary

- **1. Crash after the provider charged but before the result was saved:** known gap
- **2. Concurrent requests with the same idempotency key:** handled (single process)
- **3. Duplicate and out-of-order webhooks:** partially handled
- **4. Provider timeout:** known gap
- **5. Provider decline:** known gap

## 1. Crash after the provider charged but before the result was saved

**Today:** State is in memory, so the payment is lost. The client gets a 500 while the provider may have charged the card.

**Production:**

- Write an intent record (`PENDING`) durably *before* calling the provider.
- Send the provider an idempotency key derived from the payment ID on every attempt, so a retry can never charge twice.
- On restart, a recovery job finds intents with no outcome and queries the provider for their status. Periodic reconciliation against settlement files is the final safety net.
- Use a transactional outbox separately, for publishing events (webhooks to merchants, analytics): state and the outbox row are written in one transaction.

## 2. Concurrent requests with the same idempotency key

**Today:** `KeyLocker` makes the second request wait. The first request performs the charge and stores the response; the second replays it. Only one provider call occurs. A test covers this: *(link the test name here)*.

**Open questions to answer in code:**

- If the first request fails, does the key cache the failure or allow a retry?
- Same key with a different payload should return `422`, not replay the old response.

**Production:**

- A unique constraint on `(merchant_id, idempotency_key)` is the actual guarantee, with the stored response replayed on conflict.
- The in-process lock then becomes an optimisation that avoids duplicate work inside one pod.
- Distributed locks are a weaker foundation than the constraint, because lock expiry and clock issues can allow two holders.

## 3. Duplicate and out-of-order webhooks

**Today:** `SaveWebhookEvent` dedupes by event ID. An out-of-order event (for example `CAPTURED` after `REFUNDED`) is rejected by the state machine, and the handler returns `200` so the provider stops retrying.

**Gaps:**

- If the event is recorded as seen and the process crashes before the transition is applied, the update is lost. Recording the event and applying the transition must be atomic.
- A webhook can arrive before the payment row is committed. Unknown payments should be parked and retried briefly, not dropped.
- Signature verification is missing. Events that fail it should be rejected with `401` and logged, not queued.

**Production:** a unique constraint on `(provider, event_id)`, event recording and state change in one transaction, per-payment ordering by version, and an append-only transition log for audit.

## 4. Provider timeout

**Today:** The router falls back to the next provider. If all fail, the payment becomes `FAILED`. This is a correctness bug, because a timeout does not mean the charge failed, and a fallback can double-charge.

**Production:**

- Treat a timeout as `UNKNOWN`, a non-terminal state that can resolve to `AUTHORIZED` or `FAILED`.
- Query the provider's status API using the idempotency key, or wait for the webhook, before deciding.
- Do not fall back to another provider until the first outcome is resolved.
- Put a context deadline on every provider call.

## 5. Provider decline

**Today:** `tryProvider` retries on any error, including declines. Retrying a hard decline is wasteful and can raise fraud and network-fee signals.

**Production: classify outcomes.**

- **Hard decline** (stolen or lost card, invalid card): fail immediately, with no retry or fallback.
- **Soft decline** (insufficient funds, issuer temporarily unavailable): fail now, since a later retry may succeed. Fall back only if policy allows.
- **Technical error** (provider 5xx, connection refused): retry with backoff and jitter, then fall back.
- **Ambiguous** (timeout, dropped connection): treat as `UNKNOWN` (see scenario 4).

## Invariants the system should hold

1. A payment is never charged more than once for one idempotency key.
2. A state transition is applied at most once and only if legal.
3. No payment is marked `FAILED` unless the provider outcome is known.
