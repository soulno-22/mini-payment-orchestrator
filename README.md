# Mini Payment Orchestrator

A portfolio project demonstrating payment orchestration, idempotency, optimistic locking, high-availability routing, and circuit breakers. Written in Go with zero external dependencies.

## 🏗 Architecture

```text
HTTP Client
    │
    ▼
[ RequestID Middleware ] ──► (Trace propagation)
[ Logging Middleware   ] ──► (JSON structured logs)
    │
    ▼
┌────────────────────────────────────────────────────────┐
│ Router (net/http mux)                                  │
│                                                        │
│  POST /payments       ──► PaymentHandler               │
│  POST /webhooks/{p}   ──► WebhookHandler               │
│  GET  /metrics        ──► Metrics Registry             │
└──────┬──────────────────────────────┬──────────────────┘
       │                              │
       ▼                              ▼
┌──────────────┐               ┌─────────────────────────┐
│ Idempotency  │               │ State Machine           │
│ KeyLocker    │               │ (Pure transition fn)    │
└──────┬───────┘               └────────────┬────────────┘
       │                                    │
       ▼                                    ▼
┌────────────────────────────────────────────────────────┐
│ Store Interface (MemoryStore)                          │
│ - Create, Get                                          │
│ - Update (Optimistic Locking via Version)              │
│ - SaveIdempotencyKey, GetIdempotencyKey                │
│ - SaveWebhookEvent                                     │
└──────┬─────────────────────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────────────────────┐
│ Routing Layer                                          │
│ - SlidingWindowStrategy (Picks best provider)          │
│ - CircuitBreaker (Closed/Open/Half-Open)               │
│ - Retry & Fallback loop                                │
└──────┬─────────────────────────────────────────────────┘
       │
       ▼
┌────────────────────────────────────────────────────────┐
│ ProviderAdapter Interface                              │
│ - Provider A (Mock)                                    │
│ - Provider B (Mock)                                    │
└────────────────────────────────────────────────────────┘

State Machine
       [CREATED]
           │
      ┌────┴────┐
      ▼         ▼
[AUTHORIZED]  [FAILED] (Terminal)
      │
 ┌────┴────┐
 ▼         ▼
[CAPTURED] [FAILED] (Terminal)
 │
 ▼
[REFUNDED] (Terminal)

Core Design Decisions

#	Decision	Alternative Rejected	Why
1	Go stdlib mux	chi, gin	Go 1.22 has method+path patterns. Zero deps = every line is defensible.
2	In-memory Store behind an interface	Postgres	The interface is the artifact; Postgres is a single-file swap.
3	Optimistic Locking via expectedVersion	Pessimistic row locks	Maps 1:1 to UPDATE ... WHERE version=$1. Prevents lost updates under concurrency without DB-specific locking.
4	Custom KeyLocker for Idempotency	DB unique constraints + retry	DB constraints force clients to retry and get 500s. KeyLocker makes the 2nd request wait and replay the 1st response seamlessly.
5	Circuit Breaker (Half-Open)	Retrying indefinitely	Prevents thundering herd. When a provider is dead, fail fast to avoid hanging connections.
6	Sliding Window Strategy	Total lifetime success rate	Recent failures matter more. If a provider was great yesterday but is dying today, the window catches it instantly.
7	Webhook Dedupe via EventID	Processing every webhook	Providers retry webhooks. Deduping by EventID guarantees we don't double-apply a state transition.
8	Return 200 OK on stale webhooks	Return 400 Bad Request	If we return an error, the provider retries forever. Returning 200 tells the provider "I received it," while our state machine safely ignores the illegal transition.
9	Full refunds only	Partial refunds	Partial refunds require a ledger, currency rounding logic, and multiple refund events. Out of scope, documented as an extension point.

Extension Points
1. Adding a Provider: Implement ProviderAdapter interface. Pass it to routing.NewRouter.
2. Adding a Routing Strategy: Implement RoutingStrategy interface. Pass it to routing.NewRouter.
3. Adding a New State: Add to domain.State constants, update CanTransitionTo matrix.

"What I'd change for 300M transactions/day"
1. Postgres + Sharding: The MemoryStore becomes a Postgres cluster. Idempotency-Key becomes the shard key so requests hit the same DB node.
2. Async Persistence: Right now we write to the DB synchronously. At scale, we'd write to Kafka, and a consumer would persist the payment state.
3. Event Sourcing: Instead of just saving the final State, we append every transition to an event log. This gives us an immutable audit trail.
4. Distributed Locking: The KeyLocker is in-memory. At scale, we'd use Redis Redlock to prevent concurrent requests across multiple API pods.

How to Run
# Run the tests (requires a C compiler for the race detector)
go test -race ./...

# Run the server
go run ./cmd/server

# In a separate terminal, make a payment
curl -X POST http://localhost:8080/payments \
  -H "Idempotency-Key: my-key-1" \
  -d '{"amount_cents": 5000, "currency": "INR"}'

# Check metrics
curl http://localhost:8080/metrics

**Ctrl+S**.

---

### Step 2 — Run the server and test it manually
1. In Command Prompt, run `go run ./cmd/server`.
2. In a **new** Command Prompt, run this `curl`:
```cmd
curl -X POST http://localhost:8080/payments -H "Idempotency-Key: key-1" -H "Content-Type: application/json" -d "{\"amount_cents\": 5000, \"currency\": \"INR\"}"
3. Run it again with the exact same Idempotency-Key. You should get the exact same response, but if you look at the server terminal, you'll see the idempotency_cache_hits_total metric went up.
4. Run this to see the metrics:
curl http://localhost:8080/metrics
You should see payments_created_total, provider_success_total_provider_a, and idempotency_cache_hits_total.