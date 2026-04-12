# Multi-Agent Task Orchestrator

A prototype Go-based backend service for routing tasks to worker agents through a message-driven architecture.

Originally built for picoclaw workers, but designed to be generic enough to support other agent implementations.

## Overview

This project explores a simple orchestration backend for agent-based workflows. A central manager accepts tasks through an HTTP API, dispatches them to workers through RabbitMQ, and tracks task status and results.

The goal is to keep the orchestration layer lightweight and loosely coupled, so different worker implementations can be plugged in without changing the core backend.

## Features

- Submit tasks through a simple HTTP API
- Route tasks to worker agents via RabbitMQ
- Track task status and results
- Keep the manager decoupled from specific worker implementations
- Run locally with Docker Compose

## Architecture

- **Manager service**  
  Accepts tasks, publishes work to the queue, and stores task state

- **Worker agents**  
  Consume tasks, process them, and return results

- **RabbitMQ**  
  Handles communication between the manager and workers

- **HTTP API**  
  Exposes endpoints for task submission and task status retrieval

## API

### `POST /tasks`

Submit a task.

**Example request body:**

```json
{
  "type": "example",
  "payload": {}
}
```

**Example response:**

```json
{
  "id": "task-123"
}
```

### `GET /tasks/{id}`

Retrieve task status and result.

## Quickstart

### Run with Docker Compose

Make sure Docker is installed, then run:

```bash
docker-compose -f infra/docker-compose.yml up --build
```

### Run locally

**Start RabbitMQ:**

```bash
docker run -d --name rabbit -p 5672:5672 -p 15672:15672 rabbitmq:3-management
```

**Start the manager:**

```bash
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go run ./cmd/manager
```

**Start a sample worker:**

```bash
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go run ./workers/sample
```

## Current Status

Prototype. Basic task submission, routing, and result retrieval are implemented.

## Future Improvements

- Richer retry and failure handling
- Better worker registration and discovery
- More robust task lifecycle management
- Support for additional worker and agent implementations
- Improved observability and monitoring
