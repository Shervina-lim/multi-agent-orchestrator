# Picoclaw Agent Orchestrator (prototype)

Simple Go-based orchestration service for routing tasks to picoclaw worker agents.

Quickstart (local): ensure Go 1.20+ is installed and Docker (optional) is running.

Run RabbitMQ + manager + worker via docker-compose:

```bash
docker-compose -f infra/docker-compose.yml up --build
```

Or run manager and worker locally (start RabbitMQ separately):

```bash
# start rabbitmq using docker
docker run -d --name rabbit -p 5672:5672 -p 15672:15672 rabbitmq:3-management

RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go run ./cmd/manager &
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go run ./workers/sample
```

API:
- POST /tasks  {"type":"...","payload":{...}} -> {"id":"..."}
- GET  /tasks/{id} -> task status + result
