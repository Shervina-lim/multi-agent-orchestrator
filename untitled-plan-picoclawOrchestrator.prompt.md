## Plan: Picoclaw Agent Orchestrator

TL;DR - Create a Go-based backend orchestration service that accepts tasks (from a user or manager bot), routes them to specialized "picoclaw" worker agents, tracks iterations and task state, and exposes APIs and observability for retry, status, and results. For initial implementation use in-memory state for simplicity, but design with clear persistence adapters for future Postgres support. Recommended broker: RabbitMQ for flexible routing and easy RPC-like patterns; provide adapters so teams can swap to Redis Streams or Kafka later. Deliverables include a modular API, manager core, sample Go worker adapter, schemas, and docker-compose for local testing.

**Steps**
1. Discovery: confirm language, broker, and scale preferences with the stakeholder (*depends on alignment Qs*).
2. Design schemas: define task payloads, status lifecycle, retry/backoff policy, and error categories.
3. Define transport: choose message broker and protocol (AMQP/Redis/Kafka or HTTP/gRPC) and create stable naming conventions for queues/topics.
4. Scaffold service layout:
   - `api/` - REST/gRPC endpoints for submitting tasks, querying status, and receiving callbacks.
   - `manager/` - core orchestration: enqueues tasks, tracks state, coordinates iterative interactions with workers, enforces retries and timeouts.
   - `workers/` - example picoclaw worker adapters showing how to implement request/response and iteration loops.
   - `schemas/` - JSON Schema or Protobuf definitions for task/request/response/status.
   - `infra/` - `docker-compose.yml` with broker (RabbitMQ/Redis), optional Postgres for state, and a local manager + sample worker.
5. Implement core features (phase 1):
   - Submit task API and validation.
   - Enqueue and persist initial task state.
   - Worker protocol adapter and a sample worker that consumes tasks and emits responses.
   - Manager loop: handle incoming worker responses, decide next steps, re-enqueue follow-ups, and mark terminal states.
6. Implement advanced features (phase 2, parallelizable):
   - Retries, exponential backoff, and dead-letter handling.
   - Iteration/session management: correlation IDs, sequence numbers, and idempotency keys.
   - Observability: metrics (Prometheus), logs (structured), and a simple UI or endpoints for status.
   - Authentication/authorization for API access and worker registration.
7. Verification and testing:
   - Unit tests for schema validation, manager decision logic, and adapters.
   - Integration tests using `docker-compose` that simulate manager + worker interactions.
   - End-to-end sample: managerbot submits a task and observes a complete lifecycle.
8. Documentation & examples: README, architecture diagram, `examples/` showing how to write new picoclaw worker agents.

**Relevant files (to create)**
- `README.md` — project overview and quickstart
- `ARCHITECTURE.md` — component diagram and message flows
- `api/` — API server entrypoints and validation
- `manager/` — orchestration core and decision engine
- `workers/sample_picoclaw/` — example worker implementation
- `schemas/task.schema.json` or `schemas/task.proto` — canonical task schema
- `infra/docker-compose.yml` — broker, optional DB, manager, worker
- `tests/` — unit and integration tests

**Verification**
1. Run `docker-compose up` and execute a sample submission: manager accepts task, worker processes it, and final status recorded.
2. Unit tests: `npm test` / `pytest` covering schema validation and manager logic.
3. Integration test: scripted scenario where worker returns multiple iterative responses and manager coordinates completion.
4. Load smoke test: submit N concurrent tasks and verify throughput & retries behave as expected.

**Decisions & Assumptions**
- The orchestrator is a separate service (not embedded into worker agents).
- Workers will be language-agnostic and connect via the chosen broker/protocol.
- Tasks have a lifecycle: `queued` → `in_progress` → (`awaiting_followup` → `in_progress`)* → `completed` | `failed`.
- Persistence (DB) is optional but recommended for reliable state tracking; ephemeral-only setups are possible for low-scale testing.

**Further Considerations**
1. Broker choice tradeoffs: RabbitMQ (AMQP) for RPC-like patterns, Redis Streams for speed/simplicity, Kafka for high-throughput and long retention — recommend decision before scaffold.
2. Security: will workers be trusted internal services or need mTLS/auth tokens?
3. SLA/scale: expected concurrency will affect design (single manager vs sharded managers).

Please answer the short clarifying questions I will send next so I can finalize an implementation plan with concrete tech choices and a prioritized task list.


**Alignment (confirmed)**
- **Language:** Go (confirmed by user).
- **Broker:** RabbitMQ (user confirmed "rmqm").
- **Persistence:** In-memory for initial iteration (user opted for dev/testing), with pluggable Postgres adapter later.
- **Auth:** No auth required for initial internal/trusted environment.
- **Scale:** Start low but design for medium/high scaling later.
