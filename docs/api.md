# KubeWise API Documentation

## Overview

KubeWise API provides HTTP endpoints for interacting with the Kubernetes intelligent operations agent system. It supports both synchronous and streaming (SSE) query modes, operation confirmation flow, and session management.

**Base URL:** `http://localhost:8080`

## Quick Start

```bash
# Start the API server
kubewise serve --addr :8080

# Synchronous query
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"query": "列出所有命名空间"}'

# Streaming query (SSE)
curl -N http://localhost:8080/api/v1/chat/stream?query=列出所有命名空间
```

## Endpoints

### Health Check

```
GET /health
```

Returns service health status.

**Response:**
```json
{
  "status": "ok",
  "version": "dev"
}
```

### Synchronous Chat

```
POST /api/v1/chat
```

Send a natural language query and receive the final result. Blocks until the agent completes processing.

**Request:**
```json
{
  "query": "列出所有命名空间",
  "query_id": "optional-client-id"
}
```

**Response:**
```json
{
  "query_id": "optional-client-id",
  "result": "集群中共有以下命名空间：\n- default\n- kube-system\n- ..."
}
```

### Streaming Chat (SSE)

```
GET /api/v1/chat/stream?query=<url-encoded-query>
```

Opens an SSE connection and streams real-time events as the agent processes the query.

**SSE Event Types:**

| Event | Description | Data Fields |
|-------|-------------|-------------|
| `phase` | Processing phase | `query_id`, `phase` |
| `agent_start` | Agent begins | `query_id`, `agent_name` |
| `agent_done` | Agent finished | `query_id`, `duration`, `in_tokens`, `out_tokens` |
| `tool_call` | Tool started | `query_id`, `tool_name`, `step` |
| `tool_done` | Tool completed | `query_id`, `tool_name`, `step`, `elapsed` |
| `render_text` | Plain text result | `query_id`, `text` |
| `render_table` | Table result | `query_id`, `headers`, `rows` |
| `render_code` | Code block | `query_id`, `language`, `content` |
| `render_kv` | Key-value pairs | `query_id`, `pairs[{key, value}]` |
| `render_list` | Status list | `query_id`, `items[{status, text}]` |
| `interaction_request` | Human-in-the-loop | `interaction_id`, `query_id`, `kind`, `payload`, `total_steps` (operation) |
| `tool_fail` | Tool failed | `query_id`, `tool_name`, `step`, `elapsed`, `error` |
| `render_detail` | Resource detail | `query_id`, `detail` |
| `stream_done` | Query completed | `query_id`, `result` |
| `stream_err` | Query failed | `query_id`, `error` |
| `supervisor` | Supervisor event | `query_id`, `reason`, `decision`, `detail` |

**Example SSE output:**
```
event: phase
data: {"query_id":"q-abc123","phase":"classifying intent"}

event: tool_call
data: {"query_id":"q-abc123","tool_name":"list_namespaces","step":1}

event: tool_done
data: {"query_id":"q-abc123","tool_name":"list_namespaces","step":1,"elapsed":150000000}

event: render_text
data: {"query_id":"q-abc123","text":"集群中共有以下命名空间：..."}

event: stream_done
data: {"query_id":"q-abc123","result":"集群中共有以下命名空间：..."}
```

### Answer Interaction (HITL)

```
POST /api/v1/chat/interaction
```

When the agent needs user input (operation step confirm, chart selection, deploy plan review), it emits `interaction_request` with an `interaction_id` and `kind`. POST the response payload here to unblock the agent.

**Request:**
```json
{
  "interaction_id": "uuid-from-interaction-request",
  "payload": { "confirmed": true }
}
```

For `kind: operation_step`, `payload` matches operation confirm: `confirmed` (bool), optional `correction` (string).

**Response:**
```json
{
  "status": "ok"
}
```

### Sessions

#### List Sessions

```
GET /api/v1/sessions
```

Returns recent sessions sorted by last update time.

#### Create Session

```
POST /api/v1/sessions
```

**Request:**
```json
{
  "title": "My Session"
}
```

#### Get Session

```
GET /api/v1/sessions/{id}
```

Returns session detail including all messages.

#### Delete Session

```
DELETE /api/v1/sessions/{id}
```

## Error Handling

All error responses follow this format:

```json
{
  "error": "brief error description",
  "detail": "optional detailed error message"
}
```

## CORS

CORS is enabled for all origins by default. In production, configure `AllowOrigins` appropriately.

## Frontend Integration Example

```javascript
// SSE streaming
const query = encodeURIComponent("列出所有命名空间");
const es = new EventSource(`/api/v1/chat/stream?query=${query}`);

es.addEventListener("phase", (e) => {
  const data = JSON.parse(e.data);
  console.log("Phase:", data.phase);
});

es.addEventListener("interaction_request", async (e) => {
  const data = JSON.parse(e.data);
  if (data.kind !== "operation_step") return;
  const confirmed = window.confirm(`确认执行步骤?`);

  await fetch("/api/v1/chat/interaction", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      interaction_id: data.interaction_id,
      payload: { confirmed },
    }),
  });
});

es.addEventListener("stream_done", (e) => {
  const data = JSON.parse(e.data);
  console.log("Result:", data.result);
  es.close();
});

es.addEventListener("stream_err", (e) => {
  const data = JSON.parse(e.data);
  console.error("Error:", data.error);
  es.close();
});
```
