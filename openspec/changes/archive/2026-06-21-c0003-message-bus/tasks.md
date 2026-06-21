## 1. Broker backend interface

- [x] 1.1 Define a `Broker` interface (`Publish(topic, msg)`, `Subscribe(topics, fromEventID) -> stream`, `Ack(msgID)`) in a new `server/bus` package
- [x] 1.2 Implement the default Postgres `LISTEN/NOTIFY` backend, backed by a durable messages table for resume/redelivery
- [x] 1.3 Add an in-memory backend for tests
- [x] 1.4 Define the topic naming scheme (`runner.<id>.dispatch`, `session.<id>.control`, …) as constants/helpers

## 2. Server SSE endpoint

- [x] 2.1 Add `GET /api/v1/.../subscribe` returning `text/event-stream`, honoring requested topics and `Last-Event-ID`
- [x] 2.2 Write SSE events with monotonic ids; send periodic heartbeat comments to keep connections alive
- [x] 2.3 Add the POST acknowledgement endpoint and wire lease/redelivery for unacked messages
- [x] 2.4 Enforce subscription authorization (a subscriber may only subscribe to topics it is entitled to)

## 3. Publish path

- [x] 3.1 Change webhook ingestion to publish an event to the target topic instead of enqueuing a job as the transport (`server/webhook/handler.go`)
- [x] 3.2 Preserve idempotency: at most one published message per `(provider_code, external_event_id)`
- [x] 3.3 Reframe the `jobs` table as the durable backing store behind the bus (keep the transactional state machine)

## 4. Client (daemon) SSE consumer

- [x] 4.1 Replace the 5s poll loop (`client/runner`) with an SSE subscriber in `client/subscribe`
- [x] 4.2 Track and persist `Last-Event-ID`; reconnect with resume on disconnect
- [x] 4.3 POST acknowledgements after terminal handling

## 5. Remove the poll transport

- [x] 5.1 Remove the client-facing `POST /api/v1/jobs/claim` route and `Claim` client method
- [x] 5.2 Update affected tests; keep the state-machine and sweeper tests for the backing store

## 6. Validation

- [x] 6.1 Integration test: publish → SSE delivery → ack → no redelivery; and reconnect-with-resume
- [x] 6.2 `openspec validate --change message-bus --strict`
- [x] 6.3 e2e pointer: flip `tests/e2e/08_message_bus_test.go` from Skip to Green for BUS-01..16, and `tests/e2e/14_concurrency_test.go` for CONC-04 (per-topic ordering) and CONC-05 (concurrent sessions never cross-deliver). The MODIFIED job-queue requirement (BUS-10) is exercised by retaining the existing state-machine tests in `internal/server/jobs/` — they stay green. The MODIFIED webhook requirement (BUS-11) is exercised by `tests/e2e/06_webhook_intake_test.go` HOOK-08 (idempotent enqueue).
