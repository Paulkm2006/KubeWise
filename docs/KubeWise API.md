# KubeWise API

KubeWise HTTP API 面向 Kubernetes 智能运维场景，提供自然语言问答、SSE 流式交互、
会话管理、集群观测、故障诊断、集群安全审计和活动流查询能力。

所有错误响应默认使用统一的 `ErrorResponse` 结构。SSE 接口会保持长连接并持续推送事件，
客户端应根据事件名解析 `data` 中的 JSON 内容。

Base URLs:

# Authentication

# System

<a id="opIdroot"></a>

## GET 服务入口

GET /

返回一段纯文本，用于快速确认 KubeWise HTTP 服务已经启动并可访问。

> 返回示例

> 200 Response

```
"KubeWise — server running"
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|服务已启动，响应体为纯文本状态信息。|string|

<a id="opIdhealth"></a>

## GET 健康检查

GET /health

返回服务健康状态和版本标识，通常用于负载均衡、部署探针或监控系统存活检测。

> 返回示例

> 200 Response

```json
{
  "status": "ok",
  "version": "dev"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|服务正常运行。|[HealthResponse](#schemahealthresponse)|

# Chat

<a id="opIdchatSync"></a>

## POST 同步问答

POST /api/v1/chats

提交一个自然语言运维问题，由 KubeWise Agent 同步处理并在完成后一次性返回最终结果。
该接口会阻塞到 Agent 处理结束，适合命令行、简单集成或无需实时过程展示的场景。

> Body 请求参数

```json
{
  "query": "列出 default 命名空间下异常的 Pod",
  "query_id": "q-web-001",
  "session_id": "18b84d8c",
  "cluster": "kind-a"
}
```

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|body|body|[ChatRequest](#schemachatrequest)| 是 |none|

> 返回示例

> 200 Response

```json
{
  "query_id": "q-web-001",
  "result": "default 命名空间中发现 1 个异常 Pod：nginx-xxx 处于 CrashLoopBackOff。"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|查询成功完成，返回本次查询 ID 和最终文本结果。|[ChatResponse](#schemachatresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|请求体无法解析，或缺少必填的自然语言问题。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|Agent 执行失败、模型调用失败或后端运行时出现未预期错误。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdchatStream"></a>

## GET 流式问答

GET /api/v1/chats/stream

通过 Server-Sent Events 实时接收 Agent 处理过程，包括阶段变化、工具调用、
模型增量文本、人机协同请求、完成事件或错误事件。

正常情况下流会以 `stream_done` 结束；失败时可能发送 `stream_err`。如果 Agent 需要用户确认，
服务会发送 `interaction_request`，客户端应调用 `/api/v1/chats/interactions` 提交回答。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|query|query|string| 是 |用户输入的自然语言问题，需要 URL 编码。|
|query_id|query|string| 否 |客户端提供的查询 ID；未提供时服务端自动生成。|
|session_id|query|string| 否 |关联的会话 ID，用于客户端维持对话上下文。|
|cluster|query|string| 否 |目标集群名称；提供后服务会把集群上下文注入 Agent 查询。|

> 返回示例

> 200 Response

> 400 Response

```json
{
  "error": "invalid request",
  "detail": "unexpected EOF"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|SSE 连接建立成功，后续响应体持续推送问答事件流。|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少必填的 `query` 查询参数。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|当前 HTTP 响应不支持流式输出，或初始化 SSE 写入器失败。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdchatInteraction"></a>

## POST 提交交互回答

POST /api/v1/chats/interactions

当流式问答发送 `interaction_request` 事件后，客户端通过该接口提交用户回答，
用于解除 Agent 的等待状态。`payload` 的结构由事件中的 `kind` 决定，例如操作步骤确认、
Helm Chart 选择或部署执行确认。

> Body 请求参数

```json
{
  "interaction_id": "550e8400-e29b-41d4-a716-446655440000",
  "payload": {
    "confirmed": true,
    "correction": "先不要删除资源，改为查看日志。"
  }
}
```

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|body|body|[InteractionAnswerRequest](#schemainteractionanswerrequest)| 是 |none|

> 返回示例

> 200 Response

```json
{
  "status": "ok"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|交互回答已接收，Agent 可以继续执行。|[StatusOKResponse](#schemastatusokresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|请求体无法解析，或缺少必填的 `interaction_id`。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定的交互 ID 不存在，通常表示该交互已超时、已被回答或所属流已结束。|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|Agent 已不再等待该交互回答，提交内容未被消费。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|处理交互回答时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|

# Sessions

<a id="opIdlistSessions"></a>

## GET 会话列表

GET /api/v1/sessions

返回最近的对话会话摘要，当前服务固定读取最近 50 个会话并按存储层顺序返回。

> 返回示例

> 200 Response

```json
{
  "sessions": [
    {
      "id": "18b84d8c",
      "title": "排查 default 命名空间异常",
      "created_at": "2019-08-24T14:15:22Z",
      "updated_at": "2019-08-24T14:15:22Z",
      "message_count": 4
    }
  ]
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|会话摘要列表读取成功。|[SessionListResponse](#schemasessionlistresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|会话存储读取失败。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdcreateSession"></a>

## POST 创建会话

POST /api/v1/sessions

创建一个新的对话会话。请求体可省略；未提供标题时服务会创建空标题会话。

> Body 请求参数

```json
{
  "title": "排查 default 命名空间异常"
}
```

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|body|body|[CreateSessionRequest](#schemacreatesessionrequest)| 否 |none|

> 返回示例

> 201 Response

```json
{
  "id": "18b84d8c",
  "title": "排查 default 命名空间异常",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z",
  "message_count": 4
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|会话创建成功，返回新会话摘要。|[SessionSummary](#schemasessionsummary)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|会话存储写入失败。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdgetSession"></a>

## GET 会话详情

GET /api/v1/sessions/{id}

根据会话 ID 获取会话详情和消息列表，用于恢复历史对话或展示完整上下文。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |会话 ID。|

> 返回示例

> 200 Response

```json
{
  "id": "string",
  "title": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z",
  "messages": [
    {
      "role": "user",
      "content": "帮我排查 nginx Pod 为什么重启。",
      "timestamp": "2019-08-24T14:15:22Z"
    }
  ]
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|会话详情读取成功。|[SessionDetail](#schemasessiondetail)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定会话不存在。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|会话存储读取失败。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIddeleteSession"></a>

## DELETE 删除会话

DELETE /api/v1/sessions/{id}

删除指定 ID 的对话会话及其持久化记录。删除成功后不可通过详情接口恢复。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |会话 ID。|

> 返回示例

> 200 Response

```json
{
  "status": "deleted"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|会话删除成功。|[DeleteResponse](#schemadeleteresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定会话不存在，或删除底层文件失败。|[ErrorResponse](#schemaerrorresponse)|

# Observability

<a id="opIdlistClusters"></a>

## GET 集群列表

GET /api/v1/clusters

返回 KubeWise 当前已知 Kubernetes 集群的概览，包括健康状态、Pod 就绪数量、节点数、命名空间数和版本信息。

> 返回示例

> 200 Response

```json
[
  {
    "id": "string",
    "name": "kind-a",
    "health": "healthy",
    "pods_ready": 12,
    "pods_total": 13,
    "issues_count": 1,
    "nodes": 3,
    "namespaces": 6,
    "version": "v1.30.0",
    "fingerprint": "string",
    "last_updated": 0
  }
]
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|集群概览列表读取成功；未配置观测数据源时返回空数组。|Inline|

### 返回数据结构

状态码 **200**

*集群概览数组。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|*anonymous*|[[ClusterSummary](#schemaclustersummary)]|false|none||集群概览数组。|
|» id|string|false|none||集群唯一标识。|
|» name|string|false|none||集群名称或 kubeconfig context 名称。|
|» health|string|false|none||集群健康状态。|
|» pods_ready|integer|false|none||已就绪 Pod 数量。|
|» pods_total|integer|false|none||Pod 总数。|
|» issues_count|integer|false|none||当前检测到的问题数量。|
|» nodes|integer|false|none||节点数量。|
|» namespaces|integer|false|none||命名空间数量。|
|» version|string|false|none||Kubernetes 服务端版本。|
|» fingerprint|string|false|none||集群指纹，用于区分同名或重建后的集群。|
|» last_updated|integer|false|none||最近更新时间戳，单位由后端采集实现决定。|

#### 枚举值

|属性|值|
|---|---|
|health|healthy|
|health|degraded|
|health|offline|

<a id="opIdlistClusterIssues"></a>

## GET 集群问题列表

GET /api/v1/clusters/{name}/issues

扫描指定集群中的 Pod 状态，返回非 Running/Succeeded Pod 及其重启次数、严重程度和存活时间。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|name|path|string| 是 |目标 Kubernetes 集群名称或上下文名称。|

> 返回示例

> 200 Response

```json
[
  {
    "severity": "high",
    "cluster": "kind-a",
    "pod": "nginx-7c79c4bf97-abcde",
    "namespace": "default",
    "status": "Pending (3/1)",
    "restarts": 3,
    "age": "5m30s"
  }
]
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|集群问题列表读取成功。|Inline|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少集群名称。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定集群未注册或无法找到。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|读取 Kubernetes Pod 列表时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|集群离线，或集群网关/观测读取器不可用。|[ErrorResponse](#schemaerrorresponse)|

### 返回数据结构

状态码 **200**

*Pod 问题数组。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|*anonymous*|[[ClusterIssue](#schemaclusterissue)]|false|none||Pod 问题数组。|
|» severity|string|false|none||问题严重程度。|
|» cluster|string|false|none||所属集群名称。|
|» pod|string|false|none||Pod 名称。|
|» namespace|string|false|none||Pod 所在命名空间。|
|» status|string|false|none||Pod 状态摘要，包含 phase、重启数和容器数。|
|» restarts|integer|false|none||容器累计重启次数。|
|» age|string|false|none||Pod 已存在时长。|

#### 枚举值

|属性|值|
|---|---|
|severity|low|
|severity|medium|
|severity|high|

状态码 **400**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **404**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **500**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **503**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

<a id="opIdlistClusterEvents"></a>

## GET 集群事件列表

GET /api/v1/clusters/{name}/events

返回指定集群全部命名空间下的 Kubernetes Event，适合用于观测面板、故障上下文和诊断前置材料。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|name|path|string| 是 |目标 Kubernetes 集群名称或上下文名称。|

> 返回示例

> 200 Response

```json
[
  {
    "metadata": {},
    "involvedObject": {},
    "reason": "Failed",
    "message": "string",
    "type": "Warning",
    "count": 0,
    "firstTimestamp": "2019-08-24T14:15:22Z",
    "lastTimestamp": "2019-08-24T14:15:22Z"
  }
]
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|Kubernetes Event 列表读取成功。|Inline|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少集群名称。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定集群未注册或无法找到。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|读取 Kubernetes Event 时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|集群离线，或集群网关/观测读取器不可用。|[ErrorResponse](#schemaerrorresponse)|

### 返回数据结构

状态码 **200**

*Kubernetes Event 数组，字段来自 Kubernetes core/v1 Event。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|*anonymous*|[[KubernetesEvent](#schemakubernetesevent)]|false|none||Kubernetes Event 数组，字段来自 Kubernetes core/v1 Event。|
|» metadata|object|false|none||Kubernetes 对象元数据。|
|» involvedObject|object|false|none||事件关联的 Kubernetes 对象引用。|
|» reason|string|false|none||事件原因。|
|» message|string|false|none||事件详细消息。|
|» type|string|false|none||事件类型，例如 Normal 或 Warning。|
|» count|integer|false|none||事件出现次数。|
|» firstTimestamp|string(date-time)|false|none||首次出现时间。|
|» lastTimestamp|string(date-time)|false|none||最近出现时间。|

状态码 **400**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **404**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **500**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **503**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

# Diagnoses

<a id="opIdstartDiagnosis"></a>

## POST 启动诊断

POST /api/v1/diagnoses

针对指定集群、命名空间和 Pod 启动异步故障诊断任务。接口返回任务 ID 后，客户端可轮询详情或订阅事件流获取进度。

> Body 请求参数

```json
{
  "cluster": "kind-a",
  "namespace": "default",
  "pod": "nginx-7c79c4bf97-abcde"
}
```

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|body|body|[DiagnosisStartRequest](#schemadiagnosisstartrequest)| 是 |none|

> 返回示例

> 202 Response

```json
{
  "diagnosis_id": "diag_01HX7Z6QP3",
  "status": "running"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|202|[Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3)|诊断任务已创建并开始运行。|[DiagnosisStartResponse](#schemadiagnosisstartresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|请求体无法解析，或缺少 `cluster`、`namespace`、`pod` 中任一必填字段。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|创建诊断任务时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|诊断服务依赖不可用，例如数据库或 Agent 运行时未就绪。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdlistDiagnoses"></a>

## GET 诊断列表

GET /api/v1/diagnoses

分页查询历史诊断任务，返回任务摘要数组和总数。`limit` 超出 1 到 100 范围时服务使用默认值 20。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|limit|query|integer| 否 |每页返回数量；有效范围为 1 到 100，超出范围时服务使用默认值 20。|
|offset|query|integer| 否 |分页偏移量，从 0 开始。|

> 返回示例

> 200 Response

```json
{
  "diagnoses": [
    {
      "id": "string",
      "cluster_fingerprint": "string",
      "cluster_display": "string",
      "namespace": "string",
      "pod": "string",
      "status": "pending",
      "report": {
        "verdict": "confirmed",
        "root_cause": {
          "category": "string",
          "title": "string",
          "confidence_score": 0.1,
          "confidence_label": "string",
          "summary": "string"
        },
        "evidence": [
          {
            "id": null,
            "source": null,
            "signal": null,
            "strength": null,
            "summary": null,
            "detail": null,
            "raw_excerpt": null,
            "num": null,
            "text": null
          }
        ],
        "hypotheses": [
          {
            "id": null,
            "category": null,
            "title": null,
            "status": null,
            "confidence_delta": null,
            "supporting_evidence": null,
            "refuting_evidence": null,
            "rationale": null
          }
        ],
        "actions": [
          {
            "priority": null,
            "type": null,
            "description": null,
            "command": null,
            "risk": null
          }
        ],
        "impact": {
          "severity": "string",
          "description": "string"
        },
        "limitations": [
          "string"
        ],
        "enrichment": {
          "status": "string",
          "degraded_steps": [
            null
          ],
          "message": "string"
        },
        "markdown": "string",
        "duration_ms": 0
      },
      "root_cause": "string",
      "confidence": "string",
      "evidence": [
        {
          "id": "string",
          "source": "string",
          "signal": "string",
          "strength": "string",
          "summary": "string",
          "detail": "string",
          "raw_excerpt": "string",
          "num": 0,
          "text": "string"
        }
      ],
      "fix_actions": [
        {
          "priority": "string",
          "type": "string",
          "description": "string",
          "command": "string",
          "risk": "string"
        }
      ],
      "impact": "string",
      "duration_ms": 0,
      "created_at": "2019-08-24T14:15:22Z"
    }
  ],
  "total": 42
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|诊断任务列表读取成功。|[DiagnosisListResponse](#schemadiagnosislistresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|查询诊断列表时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|诊断服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdgetLatestDiagnosis"></a>

## GET 最新诊断

GET /api/v1/diagnoses/latest

根据集群、命名空间和 Pod 查询最近一次诊断结果及事件列表，用于页面刷新后恢复最新诊断状态。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|cluster|query|string| 是 |目标 Kubernetes 集群名称。|
|namespace|query|string| 是 |Pod 所在 Kubernetes 命名空间。|
|pod|query|string| 是 |目标 Pod 名称。|

> 返回示例

> 200 Response

```json
{
  "diagnosis_id": "string",
  "status": "pending",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string",
    "namespace": "string",
    "pod": "string"
  },
  "events": [
    {
      "diagnosis_id": "string",
      "seq_num": 0,
      "event_type": "phase",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "token_in": 0,
      "token_out": 0,
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "verdict": "confirmed",
    "root_cause": {
      "category": "string",
      "title": "string",
      "confidence_score": 0.1,
      "confidence_label": "string",
      "summary": "string"
    },
    "evidence": [
      {
        "id": "string",
        "source": "string",
        "signal": "string",
        "strength": "string",
        "summary": "string",
        "detail": "string",
        "raw_excerpt": "string",
        "num": 0,
        "text": "string"
      }
    ],
    "hypotheses": [
      {
        "id": "string",
        "category": "string",
        "title": "string",
        "status": "string",
        "confidence_delta": 0.1,
        "supporting_evidence": [
          "string"
        ],
        "refuting_evidence": [
          "string"
        ],
        "rationale": "string"
      }
    ],
    "actions": [
      {
        "priority": "string",
        "type": "string",
        "description": "string",
        "command": "string",
        "risk": "string"
      }
    ],
    "impact": {
      "severity": "string",
      "description": "string"
    },
    "limitations": [
      "string"
    ],
    "enrichment": {
      "status": "string",
      "degraded_steps": [
        "string"
      ],
      "message": "string"
    },
    "markdown": "string",
    "duration_ms": 0
  }
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|最新诊断读取成功。|[DiagnosisStatusResponse](#schemadiagnosisstatusresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少 `cluster`、`namespace` 或 `pod` 查询参数。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定目标没有历史诊断记录。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|查询最新诊断时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|诊断服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdgetDiagnosis"></a>

## GET 诊断详情

GET /api/v1/diagnoses/{id}

根据诊断 ID 获取任务状态、目标对象、已记录事件以及最终诊断结果。任务完成前 `result` 可能为空。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |诊断任务 ID。|

> 返回示例

> 200 Response

```json
{
  "diagnosis_id": "string",
  "status": "pending",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string",
    "namespace": "string",
    "pod": "string"
  },
  "events": [
    {
      "diagnosis_id": "string",
      "seq_num": 0,
      "event_type": "phase",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "token_in": 0,
      "token_out": 0,
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "verdict": "confirmed",
    "root_cause": {
      "category": "string",
      "title": "string",
      "confidence_score": 0.1,
      "confidence_label": "string",
      "summary": "string"
    },
    "evidence": [
      {
        "id": "string",
        "source": "string",
        "signal": "string",
        "strength": "string",
        "summary": "string",
        "detail": "string",
        "raw_excerpt": "string",
        "num": 0,
        "text": "string"
      }
    ],
    "hypotheses": [
      {
        "id": "string",
        "category": "string",
        "title": "string",
        "status": "string",
        "confidence_delta": 0.1,
        "supporting_evidence": [
          "string"
        ],
        "refuting_evidence": [
          "string"
        ],
        "rationale": "string"
      }
    ],
    "actions": [
      {
        "priority": "string",
        "type": "string",
        "description": "string",
        "command": "string",
        "risk": "string"
      }
    ],
    "impact": {
      "severity": "string",
      "description": "string"
    },
    "limitations": [
      "string"
    ],
    "enrichment": {
      "status": "string",
      "degraded_steps": [
        "string"
      ],
      "message": "string"
    },
    "markdown": "string",
    "duration_ms": 0
  }
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|诊断详情读取成功。|[DiagnosisStatusResponse](#schemadiagnosisstatusresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定诊断任务不存在。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdcancelDiagnosis"></a>

## POST 取消诊断

POST /api/v1/diagnoses/{id}/cancel

请求取消指定诊断任务。只有仍可取消的运行中任务会成功；已完成、已失败或不可取消状态会返回冲突。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |诊断任务 ID。|

> 返回示例

> 200 Response

```json
{
  "status": "cancelled",
  "diagnosis_id": "string"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|诊断任务已取消。|[DiagnosisCancelResponse](#schemadiagnosiscancelresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少诊断 ID。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定诊断任务不存在。|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|诊断任务当前状态不允许取消。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|诊断服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdstreamDiagnosisEvents"></a>

## GET 诊断事件流

GET /api/v1/diagnoses/{id}/events

订阅指定诊断任务的 SSE 事件流。服务每 500ms 查询一次新事件，并使用事件序号作为 SSE `id`。
客户端可通过 `since` 查询参数或 `Last-Event-ID` 请求头断点续传。任务进入终态且事件发送完成后，
服务发送 `stream_complete` 事件并关闭连接。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |诊断任务 ID。|
|since|query|integer| 否 |只返回序号大于该值的事件，用于 SSE 断点续传。|
|Last-Event-ID|header|string| 否 |SSE 客户端自动携带的最后事件 ID；服务会从下一个事件继续推送。|

> 返回示例

> 200 Response

> 400 Response

```json
{
  "error": "invalid request",
  "detail": "unexpected EOF"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|SSE 连接建立成功，后续推送 `diagnosis_event` 和 `stream_complete` 事件。|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少诊断 ID。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|当前 HTTP 响应不支持流式输出，或初始化 SSE 写入器失败。|[ErrorResponse](#schemaerrorresponse)|

# Audits

<a id="opIdstartAudit"></a>

## POST 启动审计

POST /api/v1/audits

针对指定 Kubernetes 集群启动异步安全审计任务，检查 RBAC、Pod 安全、网络策略和镜像安全等风险。

> Body 请求参数

```json
{
  "cluster": "kind-a"
}
```

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|body|body|[AuditStartRequest](#schemaauditstartrequest)| 是 |none|

> 返回示例

> 202 Response

```json
{
  "audit_id": "audit_01HX7Z6QP3",
  "status": "running"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|202|[Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3)|审计任务已创建并开始运行。|[AuditStartResponse](#schemaauditstartresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|请求体无法解析，或缺少必填的 `cluster` 字段。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|创建审计任务时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|审计服务依赖不可用，例如数据库或 Agent 运行时未就绪。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdlistAudits"></a>

## GET 审计列表

GET /api/v1/audits

分页查询历史审计任务，返回审计摘要数组和总数。`limit` 超出 1 到 100 范围时服务使用默认值 20。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|limit|query|integer| 否 |每页返回数量；有效范围为 1 到 100，超出范围时服务使用默认值 20。|
|offset|query|integer| 否 |分页偏移量，从 0 开始。|

> 返回示例

> 200 Response

```json
{
  "audits": [
    {
      "id": "string",
      "cluster_fingerprint": "string",
      "cluster_display": "string",
      "status": "running",
      "result": {
        "findings": [
          {
            "severity": null,
            "category": null,
            "resource": null,
            "risk": null,
            "impact": null,
            "suggestion": null
          }
        ],
        "summary": {
          "total": 0,
          "critical": 0,
          "high": 0,
          "medium": 0,
          "low": 0
        },
        "markdown": "string",
        "duration_ms": 0
      },
      "error_message": "string",
      "duration_ms": 0,
      "created_at": "2019-08-24T14:15:22Z"
    }
  ],
  "total": 18
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|审计任务列表读取成功。|[AuditListResponse](#schemaauditlistresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|查询审计列表时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|审计服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdgetLatestAudit"></a>

## GET 最新审计

GET /api/v1/audits/latest

根据集群名称查询最近一次审计任务详情和事件列表，用于恢复集群安全审计页面状态。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|cluster|query|string| 是 |目标 Kubernetes 集群名称。|

> 返回示例

> 200 Response

```json
{
  "audit_id": "string",
  "status": "running",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string"
  },
  "events": [
    {
      "audit_id": "string",
      "seq_num": 0,
      "event_type": "string",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "findings": [
      {
        "severity": "critical",
        "category": "string",
        "resource": "string",
        "risk": "string",
        "impact": "string",
        "suggestion": "string"
      }
    ],
    "summary": {
      "total": 0,
      "critical": 0,
      "high": 0,
      "medium": 0,
      "low": 0
    },
    "markdown": "string",
    "duration_ms": 0
  },
  "error_message": "string"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|最新审计读取成功。|[AuditStatusResponse](#schemaauditstatusresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少 `cluster` 查询参数。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定集群没有历史审计记录。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|查询最新审计时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|审计服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdgetAudit"></a>

## GET 审计详情

GET /api/v1/audits/{id}

根据审计 ID 获取任务状态、目标集群、已记录事件、最终审计结果和失败信息。任务完成前 `result` 可能为空。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |审计任务 ID。|

> 返回示例

> 200 Response

```json
{
  "audit_id": "string",
  "status": "running",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string"
  },
  "events": [
    {
      "audit_id": "string",
      "seq_num": 0,
      "event_type": "string",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "findings": [
      {
        "severity": "critical",
        "category": "string",
        "resource": "string",
        "risk": "string",
        "impact": "string",
        "suggestion": "string"
      }
    ],
    "summary": {
      "total": 0,
      "critical": 0,
      "high": 0,
      "medium": 0,
      "low": 0
    },
    "markdown": "string",
    "duration_ms": 0
  },
  "error_message": "string"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|审计详情读取成功。|[AuditStatusResponse](#schemaauditstatusresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定审计任务不存在。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdcancelAudit"></a>

## POST 取消审计

POST /api/v1/audits/{id}/cancel

请求取消指定审计任务。只有仍可取消的运行中任务会成功；已完成、已失败或不可取消状态会返回冲突。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |审计任务 ID。|

> 返回示例

> 200 Response

```json
{
  "status": "cancelled",
  "audit_id": "string"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|审计任务已取消。|[AuditCancelResponse](#schemaauditcancelresponse)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少审计 ID。|[ErrorResponse](#schemaerrorresponse)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|指定审计任务不存在。|[ErrorResponse](#schemaerrorresponse)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|审计任务当前状态不允许取消。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|审计服务不可用。|[ErrorResponse](#schemaerrorresponse)|

<a id="opIdstreamAuditEvents"></a>

## GET 审计事件流

GET /api/v1/audits/{id}/events

订阅指定审计任务的 SSE 事件流。服务每 500ms 查询一次新事件，并使用事件序号作为 SSE `id`。
客户端可通过 `since` 查询参数或 `Last-Event-ID` 请求头断点续传。任务进入终态且事件发送完成后，
服务发送 `stream_complete` 事件并关闭连接。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|id|path|string| 是 |审计任务 ID。|
|since|query|integer| 否 |只返回序号大于该值的事件，用于 SSE 断点续传。|
|Last-Event-ID|header|string| 否 |SSE 客户端自动携带的最后事件 ID；服务会从下一个事件继续推送。|

> 返回示例

> 200 Response

> 400 Response

```json
{
  "error": "invalid request",
  "detail": "unexpected EOF"
}
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|SSE 连接建立成功，后续推送 `audit_event` 和 `stream_complete` 事件。|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|缺少审计 ID。|[ErrorResponse](#schemaerrorresponse)|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|当前 HTTP 响应不支持流式输出，或初始化 SSE 写入器失败。|[ErrorResponse](#schemaerrorresponse)|

# Activities

<a id="opIdlistActivities"></a>

## GET 活动列表

GET /api/v1/activities

分页查询系统活动流，返回诊断、集群切换和系统事件等时间线记录。非法分页参数会被忽略并使用默认值。

### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|limit|query|integer| 否 |活动流每页返回数量；非法值会被忽略并使用默认值 20。|
|offset|query|integer| 否 |活动流分页偏移量；非法值会被忽略并使用默认值 0。|

> 返回示例

> 200 Response

```json
[
  {
    "id": "string",
    "type": "diagnosis",
    "text": "完成 default/nginx 的故障诊断。",
    "cluster_display": "kind-a",
    "diagnosis_id": "string",
    "created_at": "2019-08-24T14:15:22Z"
  }
]
```

### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|活动列表读取成功。|Inline|
|500|[Internal Server Error](https://tools.ietf.org/html/rfc7231#section-6.6.1)|查询活动流时发生未预期错误。|[ErrorResponse](#schemaerrorresponse)|
|503|[Service Unavailable](https://tools.ietf.org/html/rfc7231#section-6.6.4)|活动流服务或存储不可用。|[ErrorResponse](#schemaerrorresponse)|

### 返回数据结构

状态码 **200**

*活动记录数组。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|*anonymous*|[[Activity](#schemaactivity)]|false|none||活动记录数组。|
|» id|string|false|none||活动 ID。|
|» type|string|false|none||活动类型。|
|» text|string|false|none||活动展示文本。|
|» cluster_display|string|false|none||关联集群展示名称。|
|» diagnosis_id|string|false|none||关联诊断任务 ID，仅诊断类活动可能存在。|
|» created_at|string(date-time)|false|none||活动创建时间。|

#### 枚举值

|属性|值|
|---|---|
|type|diagnosis|
|type|cluster_switch|
|type|system|

状态码 **500**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

状态码 **503**

*统一错误响应。*

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|» detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

# 数据模型

<h2 id="tocS_HealthResponse">HealthResponse</h2>

<a id="schemahealthresponse"></a>
<a id="schema_HealthResponse"></a>
<a id="tocShealthresponse"></a>
<a id="tocshealthresponse"></a>

```json
{
  "status": "ok",
  "version": "dev"
}

```

服务健康检查响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||服务健康状态。|
|version|string|false|none||服务版本标识。|

<h2 id="tocS_StatusOKResponse">StatusOKResponse</h2>

<a id="schemastatusokresponse"></a>
<a id="schema_StatusOKResponse"></a>
<a id="tocSstatusokresponse"></a>
<a id="tocsstatusokresponse"></a>

```json
{
  "status": "ok"
}

```

通用成功状态响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||操作状态。|

<h2 id="tocS_DeleteResponse">DeleteResponse</h2>

<a id="schemadeleteresponse"></a>
<a id="schema_DeleteResponse"></a>
<a id="tocSdeleteresponse"></a>
<a id="tocsdeleteresponse"></a>

```json
{
  "status": "deleted"
}

```

删除成功响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||删除操作状态。|

<h2 id="tocS_ErrorResponse">ErrorResponse</h2>

<a id="schemaerrorresponse"></a>
<a id="schema_ErrorResponse"></a>
<a id="tocSerrorresponse"></a>
<a id="tocserrorresponse"></a>

```json
{
  "error": "invalid request",
  "detail": "unexpected EOF"
}

```

统一错误响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|error|string|false|none||简短错误信息，适合直接展示或用于日志定位。|
|detail|string|false|none||可选的详细错误信息，通常包含底层异常内容。|

<h2 id="tocS_ChatRequest">ChatRequest</h2>

<a id="schemachatrequest"></a>
<a id="schema_ChatRequest"></a>
<a id="tocSchatrequest"></a>
<a id="tocschatrequest"></a>

```json
{
  "query": "列出 default 命名空间下异常的 Pod",
  "query_id": "q-web-001",
  "session_id": "18b84d8c",
  "cluster": "kind-a"
}

```

同步问答请求。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|query|string|true|none||用户输入的自然语言运维问题。|
|query_id|string|false|none||可选的客户端查询 ID；未提供时服务端自动生成。|
|session_id|string|false|none||可选的会话 ID，用于客户端把问答关联到已有会话。|
|cluster|string|false|none||可选的目标集群名称；提供后 Agent 会在该集群上下文中处理问题。|

<h2 id="tocS_ChatResponse">ChatResponse</h2>

<a id="schemachatresponse"></a>
<a id="schema_ChatResponse"></a>
<a id="tocSchatresponse"></a>
<a id="tocschatresponse"></a>

```json
{
  "query_id": "q-web-001",
  "result": "default 命名空间中发现 1 个异常 Pod：nginx-xxx 处于 CrashLoopBackOff。"
}

```

同步问答响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|query_id|string|false|none||本次查询 ID。|
|result|string|false|none||Agent 生成的最终文本结果。|

<h2 id="tocS_InteractionAnswerRequest">InteractionAnswerRequest</h2>

<a id="schemainteractionanswerrequest"></a>
<a id="schema_InteractionAnswerRequest"></a>
<a id="tocSinteractionanswerrequest"></a>
<a id="tocsinteractionanswerrequest"></a>

```json
{
  "interaction_id": "550e8400-e29b-41d4-a716-446655440000",
  "payload": {
    "confirmed": true,
    "correction": "先不要删除资源，改为查看日志。"
  }
}

```

人机协同回答请求。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|interaction_id|string|true|none||`interaction_request` SSE 事件中返回的交互 ID。|
|payload|any|false|none||根据交互类型变化的回答内容；为空时服务按空对象处理。|

oneOf

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» *anonymous*|[OperationStepPayload](#schemaoperationsteppayload)|false|none||操作步骤确认回答。|

xor

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» *anonymous*|[ChartSelectPayload](#schemachartselectpayload)|false|none||Helm Chart 选择回答。|

xor

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» *anonymous*|[DeployConfirmPayload](#schemadeployconfirmpayload)|false|none||部署执行确认回答。|

<h2 id="tocS_InteractionRequestData">InteractionRequestData</h2>

<a id="schemainteractionrequestdata"></a>
<a id="schema_InteractionRequestData"></a>
<a id="tocSinteractionrequestdata"></a>
<a id="tocsinteractionrequestdata"></a>

```json
{
  "interaction_id": "string",
  "query_id": "string",
  "kind": "operation_step",
  "payload": {},
  "total_steps": 0
}

```

`interaction_request` SSE 事件数据。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|interaction_id|string|false|none||本次交互 ID，提交回答时必须带回。|
|query_id|string|false|none||所属查询 ID。|
|kind|string|false|none||交互类型，决定 `payload` 和回答结构。|
|payload|object|false|none||交互展示所需的上下文数据，具体结构由 `kind` 决定。|
|total_steps|integer|false|none||操作步骤总数；仅 `operation_step` 类型可能出现。|

#### 枚举值

|属性|值|
|---|---|
|kind|operation_step|
|kind|chart_select|
|kind|deploy_confirm|
|kind|unknown|

<h2 id="tocS_OperationStepPayload">OperationStepPayload</h2>

<a id="schemaoperationsteppayload"></a>
<a id="schema_OperationStepPayload"></a>
<a id="tocSoperationsteppayload"></a>
<a id="tocsoperationsteppayload"></a>

```json
{
  "confirmed": true,
  "correction": "先不要删除资源，改为查看日志。"
}

```

操作步骤确认回答。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|confirmed|boolean|false|none||是否确认执行当前操作步骤。|
|correction|string|false|none||用户拒绝或修正时提供的补充说明。|

<h2 id="tocS_ChartSelectPayload">ChartSelectPayload</h2>

<a id="schemachartselectpayload"></a>
<a id="schema_ChartSelectPayload"></a>
<a id="tocSchartselectpayload"></a>
<a id="tocschartselectpayload"></a>

```json
{
  "cancelled": false,
  "use_manual_chart": false,
  "candidate_index": 0,
  "manual_repo_url": "https://charts.bitnami.com/bitnami",
  "manual_chart_name": "nginx"
}

```

Helm Chart 选择回答。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|cancelled|boolean|false|none||是否取消 Chart 选择流程。|
|use_manual_chart|boolean|false|none||是否使用手动填写的 Chart 信息。|
|candidate_index|integer|false|none||被选中的候选 Chart 下标，从 0 开始。|
|manual_repo_url|string|false|none||手动 Chart 仓库地址，仅使用手动 Chart 时填写。|
|manual_chart_name|string|false|none||手动 Chart 名称，仅使用手动 Chart 时填写。|

<h2 id="tocS_DeployConfirmPayload">DeployConfirmPayload</h2>

<a id="schemadeployconfirmpayload"></a>
<a id="schema_DeployConfirmPayload"></a>
<a id="tocSdeployconfirmpayload"></a>
<a id="tocsdeployconfirmpayload"></a>

```json
{
  "action": "execute",
  "values": "string",
  "correction": "string"
}

```

部署执行确认回答。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|action|string|false|none||部署决策，`execute` 表示执行，`cancel` 表示取消。|
|values|string|false|none||用户调整后的 Helm values YAML 内容。|
|correction|string|false|none||用户对部署计划的修正说明。|

#### 枚举值

|属性|值|
|---|---|
|action|execute|
|action|cancel|

<h2 id="tocS_CreateSessionRequest">CreateSessionRequest</h2>

<a id="schemacreatesessionrequest"></a>
<a id="schema_CreateSessionRequest"></a>
<a id="tocScreatesessionrequest"></a>
<a id="tocscreatesessionrequest"></a>

```json
{
  "title": "排查 default 命名空间异常"
}

```

创建会话请求。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|title|string|false|none||会话标题；为空时由后续消息或客户端自行维护展示。|

<h2 id="tocS_SessionSummary">SessionSummary</h2>

<a id="schemasessionsummary"></a>
<a id="schema_SessionSummary"></a>
<a id="tocSsessionsummary"></a>
<a id="tocssessionsummary"></a>

```json
{
  "id": "18b84d8c",
  "title": "排查 default 命名空间异常",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z",
  "message_count": 4
}

```

会话摘要。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||会话 ID。|
|title|string|false|none||会话标题。|
|created_at|string(date-time)|false|none||会话创建时间。|
|updated_at|string(date-time)|false|none||会话最近更新时间。|
|message_count|integer|false|none||会话内消息数量。|

<h2 id="tocS_SessionListResponse">SessionListResponse</h2>

<a id="schemasessionlistresponse"></a>
<a id="schema_SessionListResponse"></a>
<a id="tocSsessionlistresponse"></a>
<a id="tocssessionlistresponse"></a>

```json
{
  "sessions": [
    {
      "id": "18b84d8c",
      "title": "排查 default 命名空间异常",
      "created_at": "2019-08-24T14:15:22Z",
      "updated_at": "2019-08-24T14:15:22Z",
      "message_count": 4
    }
  ]
}

```

会话列表响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|sessions|[[SessionSummary](#schemasessionsummary)]|false|none||会话摘要数组。|

<h2 id="tocS_SessionDetail">SessionDetail</h2>

<a id="schemasessiondetail"></a>
<a id="schema_SessionDetail"></a>
<a id="tocSsessiondetail"></a>
<a id="tocssessiondetail"></a>

```json
{
  "id": "string",
  "title": "string",
  "created_at": "2019-08-24T14:15:22Z",
  "updated_at": "2019-08-24T14:15:22Z",
  "messages": [
    {
      "role": "user",
      "content": "帮我排查 nginx Pod 为什么重启。",
      "timestamp": "2019-08-24T14:15:22Z"
    }
  ]
}

```

会话详情。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||会话 ID。|
|title|string|false|none||会话标题。|
|created_at|string(date-time)|false|none||会话创建时间。|
|updated_at|string(date-time)|false|none||会话最近更新时间。|
|messages|[[SessionMessage](#schemasessionmessage)]|false|none||会话消息列表。|

<h2 id="tocS_SessionMessage">SessionMessage</h2>

<a id="schemasessionmessage"></a>
<a id="schema_SessionMessage"></a>
<a id="tocSsessionmessage"></a>
<a id="tocssessionmessage"></a>

```json
{
  "role": "user",
  "content": "帮我排查 nginx Pod 为什么重启。",
  "timestamp": "2019-08-24T14:15:22Z"
}

```

会话消息。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|role|string|false|none||消息角色。|
|content|string|false|none||消息文本内容。|
|timestamp|string(date-time)|false|none||消息创建时间。|

#### 枚举值

|属性|值|
|---|---|
|role|user|
|role|assistant|

<h2 id="tocS_ClusterSummary">ClusterSummary</h2>

<a id="schemaclustersummary"></a>
<a id="schema_ClusterSummary"></a>
<a id="tocSclustersummary"></a>
<a id="tocsclustersummary"></a>

```json
{
  "id": "string",
  "name": "kind-a",
  "health": "healthy",
  "pods_ready": 12,
  "pods_total": 13,
  "issues_count": 1,
  "nodes": 3,
  "namespaces": 6,
  "version": "v1.30.0",
  "fingerprint": "string",
  "last_updated": 0
}

```

Kubernetes 集群概览。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||集群唯一标识。|
|name|string|false|none||集群名称或 kubeconfig context 名称。|
|health|string|false|none||集群健康状态。|
|pods_ready|integer|false|none||已就绪 Pod 数量。|
|pods_total|integer|false|none||Pod 总数。|
|issues_count|integer|false|none||当前检测到的问题数量。|
|nodes|integer|false|none||节点数量。|
|namespaces|integer|false|none||命名空间数量。|
|version|string|false|none||Kubernetes 服务端版本。|
|fingerprint|string|false|none||集群指纹，用于区分同名或重建后的集群。|
|last_updated|integer|false|none||最近更新时间戳，单位由后端采集实现决定。|

#### 枚举值

|属性|值|
|---|---|
|health|healthy|
|health|degraded|
|health|offline|

<h2 id="tocS_ClusterIssue">ClusterIssue</h2>

<a id="schemaclusterissue"></a>
<a id="schema_ClusterIssue"></a>
<a id="tocSclusterissue"></a>
<a id="tocsclusterissue"></a>

```json
{
  "severity": "high",
  "cluster": "kind-a",
  "pod": "nginx-7c79c4bf97-abcde",
  "namespace": "default",
  "status": "Pending (3/1)",
  "restarts": 3,
  "age": "5m30s"
}

```

集群中的 Pod 问题摘要。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|severity|string|false|none||问题严重程度。|
|cluster|string|false|none||所属集群名称。|
|pod|string|false|none||Pod 名称。|
|namespace|string|false|none||Pod 所在命名空间。|
|status|string|false|none||Pod 状态摘要，包含 phase、重启数和容器数。|
|restarts|integer|false|none||容器累计重启次数。|
|age|string|false|none||Pod 已存在时长。|

#### 枚举值

|属性|值|
|---|---|
|severity|low|
|severity|medium|
|severity|high|

<h2 id="tocS_KubernetesEvent">KubernetesEvent</h2>

<a id="schemakubernetesevent"></a>
<a id="schema_KubernetesEvent"></a>
<a id="tocSkubernetesevent"></a>
<a id="tocskubernetesevent"></a>

```json
{
  "metadata": {},
  "involvedObject": {},
  "reason": "Failed",
  "message": "string",
  "type": "Warning",
  "count": 0,
  "firstTimestamp": "2019-08-24T14:15:22Z",
  "lastTimestamp": "2019-08-24T14:15:22Z"
}

```

Kubernetes core/v1 Event。实际响应保留 Kubernetes 原始字段，本 schema 只列出常用字段。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|metadata|object|false|none||Kubernetes 对象元数据。|
|involvedObject|object|false|none||事件关联的 Kubernetes 对象引用。|
|reason|string|false|none||事件原因。|
|message|string|false|none||事件详细消息。|
|type|string|false|none||事件类型，例如 Normal 或 Warning。|
|count|integer|false|none||事件出现次数。|
|firstTimestamp|string(date-time)|false|none||首次出现时间。|
|lastTimestamp|string(date-time)|false|none||最近出现时间。|

<h2 id="tocS_DiagnosisStartRequest">DiagnosisStartRequest</h2>

<a id="schemadiagnosisstartrequest"></a>
<a id="schema_DiagnosisStartRequest"></a>
<a id="tocSdiagnosisstartrequest"></a>
<a id="tocsdiagnosisstartrequest"></a>

```json
{
  "cluster": "kind-a",
  "namespace": "default",
  "pod": "nginx-7c79c4bf97-abcde"
}

```

启动诊断请求。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|cluster|string|true|none||目标集群名称。|
|namespace|string|true|none||目标 Pod 所在命名空间。|
|pod|string|true|none||目标 Pod 名称。|

<h2 id="tocS_DiagnosisStartResponse">DiagnosisStartResponse</h2>

<a id="schemadiagnosisstartresponse"></a>
<a id="schema_DiagnosisStartResponse"></a>
<a id="tocSdiagnosisstartresponse"></a>
<a id="tocsdiagnosisstartresponse"></a>

```json
{
  "diagnosis_id": "diag_01HX7Z6QP3",
  "status": "running"
}

```

启动诊断响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|diagnosis_id|string|false|none||新建诊断任务 ID。|
|status|string|false|none||任务初始状态。|

<h2 id="tocS_DiagnosisCancelResponse">DiagnosisCancelResponse</h2>

<a id="schemadiagnosiscancelresponse"></a>
<a id="schema_DiagnosisCancelResponse"></a>
<a id="tocSdiagnosiscancelresponse"></a>
<a id="tocsdiagnosiscancelresponse"></a>

```json
{
  "status": "cancelled",
  "diagnosis_id": "string"
}

```

取消诊断响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||取消状态。|
|diagnosis_id|string|false|none||被取消的诊断任务 ID。|

<h2 id="tocS_DiagnosisListResponse">DiagnosisListResponse</h2>

<a id="schemadiagnosislistresponse"></a>
<a id="schema_DiagnosisListResponse"></a>
<a id="tocSdiagnosislistresponse"></a>
<a id="tocsdiagnosislistresponse"></a>

```json
{
  "diagnoses": [
    {
      "id": "string",
      "cluster_fingerprint": "string",
      "cluster_display": "string",
      "namespace": "string",
      "pod": "string",
      "status": "pending",
      "report": {
        "verdict": "confirmed",
        "root_cause": {
          "category": "string",
          "title": "string",
          "confidence_score": 0.1,
          "confidence_label": "string",
          "summary": "string"
        },
        "evidence": [
          {
            "id": null,
            "source": null,
            "signal": null,
            "strength": null,
            "summary": null,
            "detail": null,
            "raw_excerpt": null,
            "num": null,
            "text": null
          }
        ],
        "hypotheses": [
          {
            "id": null,
            "category": null,
            "title": null,
            "status": null,
            "confidence_delta": null,
            "supporting_evidence": null,
            "refuting_evidence": null,
            "rationale": null
          }
        ],
        "actions": [
          {
            "priority": null,
            "type": null,
            "description": null,
            "command": null,
            "risk": null
          }
        ],
        "impact": {
          "severity": "string",
          "description": "string"
        },
        "limitations": [
          "string"
        ],
        "enrichment": {
          "status": "string",
          "degraded_steps": [
            null
          ],
          "message": "string"
        },
        "markdown": "string",
        "duration_ms": 0
      },
      "root_cause": "string",
      "confidence": "string",
      "evidence": [
        {
          "id": "string",
          "source": "string",
          "signal": "string",
          "strength": "string",
          "summary": "string",
          "detail": "string",
          "raw_excerpt": "string",
          "num": 0,
          "text": "string"
        }
      ],
      "fix_actions": [
        {
          "priority": "string",
          "type": "string",
          "description": "string",
          "command": "string",
          "risk": "string"
        }
      ],
      "impact": "string",
      "duration_ms": 0,
      "created_at": "2019-08-24T14:15:22Z"
    }
  ],
  "total": 42
}

```

诊断列表响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|diagnoses|[[Diagnosis](#schemadiagnosis)]|false|none||诊断任务数组。|
|total|integer|false|none||符合条件的诊断任务总数。|

<h2 id="tocS_DiagnosisStatusResponse">DiagnosisStatusResponse</h2>

<a id="schemadiagnosisstatusresponse"></a>
<a id="schema_DiagnosisStatusResponse"></a>
<a id="tocSdiagnosisstatusresponse"></a>
<a id="tocsdiagnosisstatusresponse"></a>

```json
{
  "diagnosis_id": "string",
  "status": "pending",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string",
    "namespace": "string",
    "pod": "string"
  },
  "events": [
    {
      "diagnosis_id": "string",
      "seq_num": 0,
      "event_type": "phase",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "token_in": 0,
      "token_out": 0,
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "verdict": "confirmed",
    "root_cause": {
      "category": "string",
      "title": "string",
      "confidence_score": 0.1,
      "confidence_label": "string",
      "summary": "string"
    },
    "evidence": [
      {
        "id": "string",
        "source": "string",
        "signal": "string",
        "strength": "string",
        "summary": "string",
        "detail": "string",
        "raw_excerpt": "string",
        "num": 0,
        "text": "string"
      }
    ],
    "hypotheses": [
      {
        "id": "string",
        "category": "string",
        "title": "string",
        "status": "string",
        "confidence_delta": 0.1,
        "supporting_evidence": [
          "string"
        ],
        "refuting_evidence": [
          "string"
        ],
        "rationale": "string"
      }
    ],
    "actions": [
      {
        "priority": "string",
        "type": "string",
        "description": "string",
        "command": "string",
        "risk": "string"
      }
    ],
    "impact": {
      "severity": "string",
      "description": "string"
    },
    "limitations": [
      "string"
    ],
    "enrichment": {
      "status": "string",
      "degraded_steps": [
        "string"
      ],
      "message": "string"
    },
    "markdown": "string",
    "duration_ms": 0
  }
}

```

诊断详情响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|diagnosis_id|string|false|none||诊断任务 ID。|
|status|string|false|none||当前诊断状态。|
|created_at|string(date-time)|false|none||诊断创建时间。|
|target|[DiagnosisTarget](#schemadiagnosistarget)|false|none||诊断目标对象。|
|events|[[DiagnosisEventRecord](#schemadiagnosiseventrecord)]|false|none||已记录的诊断事件。|
|result|[DiagnosisResult](#schemadiagnosisresult)|false|none||结构化诊断结果。|

#### 枚举值

|属性|值|
|---|---|
|status|pending|
|status|running|
|status|completed|
|status|failed|
|status|cancelled|

<h2 id="tocS_Diagnosis">Diagnosis</h2>

<a id="schemadiagnosis"></a>
<a id="schema_Diagnosis"></a>
<a id="tocSdiagnosis"></a>
<a id="tocsdiagnosis"></a>

```json
{
  "id": "string",
  "cluster_fingerprint": "string",
  "cluster_display": "string",
  "namespace": "string",
  "pod": "string",
  "status": "pending",
  "report": {
    "verdict": "confirmed",
    "root_cause": {
      "category": "string",
      "title": "string",
      "confidence_score": 0.1,
      "confidence_label": "string",
      "summary": "string"
    },
    "evidence": [
      {
        "id": "string",
        "source": "string",
        "signal": "string",
        "strength": "string",
        "summary": "string",
        "detail": "string",
        "raw_excerpt": "string",
        "num": 0,
        "text": "string"
      }
    ],
    "hypotheses": [
      {
        "id": "string",
        "category": "string",
        "title": "string",
        "status": "string",
        "confidence_delta": 0.1,
        "supporting_evidence": [
          "string"
        ],
        "refuting_evidence": [
          "string"
        ],
        "rationale": "string"
      }
    ],
    "actions": [
      {
        "priority": "string",
        "type": "string",
        "description": "string",
        "command": "string",
        "risk": "string"
      }
    ],
    "impact": {
      "severity": "string",
      "description": "string"
    },
    "limitations": [
      "string"
    ],
    "enrichment": {
      "status": "string",
      "degraded_steps": [
        "string"
      ],
      "message": "string"
    },
    "markdown": "string",
    "duration_ms": 0
  },
  "root_cause": "string",
  "confidence": "string",
  "evidence": [
    {
      "id": "string",
      "source": "string",
      "signal": "string",
      "strength": "string",
      "summary": "string",
      "detail": "string",
      "raw_excerpt": "string",
      "num": 0,
      "text": "string"
    }
  ],
  "fix_actions": [
    {
      "priority": "string",
      "type": "string",
      "description": "string",
      "command": "string",
      "risk": "string"
    }
  ],
  "impact": "string",
  "duration_ms": 0,
  "created_at": "2019-08-24T14:15:22Z"
}

```

诊断任务摘要或持久化记录。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||诊断任务 ID。|
|cluster_fingerprint|string|false|none||集群指纹。|
|cluster_display|string|false|none||面向用户展示的集群名称。|
|namespace|string|false|none||诊断目标命名空间。|
|pod|string|false|none||诊断目标 Pod。|
|status|string|false|none||诊断状态。|
|report|[DiagnosisResult](#schemadiagnosisresult)|false|none||结构化诊断结果。|
|root_cause|string|false|none||兼容旧客户端的根因摘要。|
|confidence|string|false|none||兼容旧客户端的置信度标签。|
|evidence|[[DiagnosisEvidence](#schemadiagnosisevidence)]|false|none||兼容旧客户端的证据列表。|
|fix_actions|[[FixAction](#schemafixaction)]|false|none||兼容旧客户端的修复动作列表。|
|impact|string|false|none||兼容旧客户端的影响说明。|
|duration_ms|integer(int64)|false|none||诊断耗时，单位毫秒。|
|created_at|string(date-time)|false|none||诊断创建时间。|

#### 枚举值

|属性|值|
|---|---|
|status|pending|
|status|running|
|status|completed|
|status|failed|
|status|cancelled|

<h2 id="tocS_DiagnosisTarget">DiagnosisTarget</h2>

<a id="schemadiagnosistarget"></a>
<a id="schema_DiagnosisTarget"></a>
<a id="tocSdiagnosistarget"></a>
<a id="tocsdiagnosistarget"></a>

```json
{
  "cluster": "string",
  "cluster_display": "string",
  "namespace": "string",
  "pod": "string"
}

```

诊断目标对象。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|cluster|string|false|none||集群指纹或内部集群标识。|
|cluster_display|string|false|none||面向用户展示的集群名称。|
|namespace|string|false|none||Pod 所在命名空间。|
|pod|string|false|none||Pod 名称。|

<h2 id="tocS_DiagnosisEventRecord">DiagnosisEventRecord</h2>

<a id="schemadiagnosiseventrecord"></a>
<a id="schema_DiagnosisEventRecord"></a>
<a id="tocSdiagnosiseventrecord"></a>
<a id="tocsdiagnosiseventrecord"></a>

```json
{
  "diagnosis_id": "string",
  "seq_num": 0,
  "event_type": "phase",
  "message": "string",
  "summary": "string",
  "detail": "string",
  "payload_kind": "string",
  "payload_json": "string",
  "token_in": 0,
  "token_out": 0,
  "elapsed_ms": 0,
  "created_at": 0
}

```

诊断事件记录。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|diagnosis_id|string|false|none||所属诊断任务 ID。|
|seq_num|integer|false|none||事件序号，递增并用于 SSE 断点续传。|
|event_type|string|false|none||事件类型。|
|message|string|false|none||面向用户的事件消息。|
|summary|string|false|none||事件摘要。|
|detail|string|false|none||事件详细信息。|
|payload_kind|string|false|none||事件载荷类型。|
|payload_json|string|false|none||JSON 字符串形式的事件载荷。|
|token_in|integer|false|none||本步骤输入 token 数。|
|token_out|integer|false|none||本步骤输出 token 数。|
|elapsed_ms|integer|false|none||本步骤耗时，单位毫秒。|
|created_at|integer(int64)|false|none||事件创建时间戳。|

<h2 id="tocS_DiagnosisResult">DiagnosisResult</h2>

<a id="schemadiagnosisresult"></a>
<a id="schema_DiagnosisResult"></a>
<a id="tocSdiagnosisresult"></a>
<a id="tocsdiagnosisresult"></a>

```json
{
  "verdict": "confirmed",
  "root_cause": {
    "category": "string",
    "title": "string",
    "confidence_score": 0.1,
    "confidence_label": "string",
    "summary": "string"
  },
  "evidence": [
    {
      "id": "string",
      "source": "string",
      "signal": "string",
      "strength": "string",
      "summary": "string",
      "detail": "string",
      "raw_excerpt": "string",
      "num": 0,
      "text": "string"
    }
  ],
  "hypotheses": [
    {
      "id": "string",
      "category": "string",
      "title": "string",
      "status": "string",
      "confidence_delta": 0.1,
      "supporting_evidence": [
        "string"
      ],
      "refuting_evidence": [
        "string"
      ],
      "rationale": "string"
    }
  ],
  "actions": [
    {
      "priority": "string",
      "type": "string",
      "description": "string",
      "command": "string",
      "risk": "string"
    }
  ],
  "impact": {
    "severity": "string",
    "description": "string"
  },
  "limitations": [
    "string"
  ],
  "enrichment": {
    "status": "string",
    "degraded_steps": [
      "string"
    ],
    "message": "string"
  },
  "markdown": "string",
  "duration_ms": 0
}

```

结构化诊断结果。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|verdict|string|false|none||诊断结论可信程度。|
|root_cause|[RootCause](#schemarootcause)|false|none||根因分析结果。|
|evidence|[[DiagnosisEvidence](#schemadiagnosisevidence)]|false|none||支撑或反驳诊断结论的证据。|
|hypotheses|[[Hypothesis](#schemahypothesis)]|false|none||诊断过程中验证过的候选假设。|
|actions|[[FixAction](#schemafixaction)]|false|none||建议执行的修复动作。|
|impact|[Impact](#schemaimpact)|false|none||故障影响。|
|limitations|[string]|false|none||诊断限制或仍缺失的信息。|
|enrichment|[EnrichmentInfo](#schemaenrichmentinfo)|false|none||诊断增强信息。|
|markdown|string|false|none||适合直接展示的 Markdown 诊断报告。|
|duration_ms|integer(int64)|false|none||诊断耗时，单位毫秒。|

#### 枚举值

|属性|值|
|---|---|
|verdict|confirmed|
|verdict|likely|
|verdict|inconclusive|

<h2 id="tocS_RootCause">RootCause</h2>

<a id="schemarootcause"></a>
<a id="schema_RootCause"></a>
<a id="tocSrootcause"></a>
<a id="tocsrootcause"></a>

```json
{
  "category": "string",
  "title": "string",
  "confidence_score": 0.1,
  "confidence_label": "string",
  "summary": "string"
}

```

根因分析结果。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|category|string|false|none||根因类别。|
|title|string|false|none||根因标题。|
|confidence_score|number(double)|false|none||置信度分数。|
|confidence_label|string|false|none||置信度标签。|
|summary|string|false|none||根因说明。|

<h2 id="tocS_DiagnosisEvidence">DiagnosisEvidence</h2>

<a id="schemadiagnosisevidence"></a>
<a id="schema_DiagnosisEvidence"></a>
<a id="tocSdiagnosisevidence"></a>
<a id="tocsdiagnosisevidence"></a>

```json
{
  "id": "string",
  "source": "string",
  "signal": "string",
  "strength": "string",
  "summary": "string",
  "detail": "string",
  "raw_excerpt": "string",
  "num": 0,
  "text": "string"
}

```

诊断证据。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||证据 ID。|
|source|string|false|none||证据来源，例如事件、日志、资源状态。|
|signal|string|false|none||证据信号名称。|
|strength|string|false|none||证据强度。|
|summary|string|false|none||证据摘要。|
|detail|string|false|none||证据详情。|
|raw_excerpt|string|false|none||原始内容摘录。|
|num|integer|false|none||兼容旧客户端的证据序号。|
|text|string|false|none||兼容旧客户端的证据文本。|

<h2 id="tocS_Hypothesis">Hypothesis</h2>

<a id="schemahypothesis"></a>
<a id="schema_Hypothesis"></a>
<a id="tocShypothesis"></a>
<a id="tocshypothesis"></a>

```json
{
  "id": "string",
  "category": "string",
  "title": "string",
  "status": "string",
  "confidence_delta": 0.1,
  "supporting_evidence": [
    "string"
  ],
  "refuting_evidence": [
    "string"
  ],
  "rationale": "string"
}

```

候选诊断假设。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||假设 ID。|
|category|string|false|none||假设类别。|
|title|string|false|none||假设标题。|
|status|string|false|none||验证状态。|
|confidence_delta|number(double)|false|none||该假设验证后带来的置信度变化。|
|supporting_evidence|[string]|false|none||支持该假设的证据 ID 列表。|
|refuting_evidence|[string]|false|none||反驳该假设的证据 ID 列表。|
|rationale|string|false|none||假设判断理由。|

<h2 id="tocS_FixAction">FixAction</h2>

<a id="schemafixaction"></a>
<a id="schema_FixAction"></a>
<a id="tocSfixaction"></a>
<a id="tocsfixaction"></a>

```json
{
  "priority": "string",
  "type": "string",
  "description": "string",
  "command": "string",
  "risk": "string"
}

```

修复建议动作。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|priority|string|false|none||动作优先级。|
|type|string|false|none||动作类型。|
|description|string|false|none||修复动作说明。|
|command|string|false|none||可选的建议命令。|
|risk|string|false|none||执行动作可能带来的风险。|

<h2 id="tocS_Impact">Impact</h2>

<a id="schemaimpact"></a>
<a id="schema_Impact"></a>
<a id="tocSimpact"></a>
<a id="tocsimpact"></a>

```json
{
  "severity": "string",
  "description": "string"
}

```

故障影响。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|severity|string|false|none||影响严重程度。|
|description|string|false|none||影响说明。|

<h2 id="tocS_EnrichmentInfo">EnrichmentInfo</h2>

<a id="schemaenrichmentinfo"></a>
<a id="schema_EnrichmentInfo"></a>
<a id="tocSenrichmentinfo"></a>
<a id="tocsenrichmentinfo"></a>

```json
{
  "status": "string",
  "degraded_steps": [
    "string"
  ],
  "message": "string"
}

```

诊断增强信息。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||增强数据处理状态。|
|degraded_steps|[string]|false|none||降级处理的步骤列表。|
|message|string|false|none||增强处理说明。|

<h2 id="tocS_AuditStartRequest">AuditStartRequest</h2>

<a id="schemaauditstartrequest"></a>
<a id="schema_AuditStartRequest"></a>
<a id="tocSauditstartrequest"></a>
<a id="tocsauditstartrequest"></a>

```json
{
  "cluster": "kind-a"
}

```

启动审计请求。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|cluster|string|true|none||目标集群名称。|

<h2 id="tocS_AuditStartResponse">AuditStartResponse</h2>

<a id="schemaauditstartresponse"></a>
<a id="schema_AuditStartResponse"></a>
<a id="tocSauditstartresponse"></a>
<a id="tocsauditstartresponse"></a>

```json
{
  "audit_id": "audit_01HX7Z6QP3",
  "status": "running"
}

```

启动审计响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|audit_id|string|false|none||新建审计任务 ID。|
|status|string|false|none||任务初始状态。|

<h2 id="tocS_AuditCancelResponse">AuditCancelResponse</h2>

<a id="schemaauditcancelresponse"></a>
<a id="schema_AuditCancelResponse"></a>
<a id="tocSauditcancelresponse"></a>
<a id="tocsauditcancelresponse"></a>

```json
{
  "status": "cancelled",
  "audit_id": "string"
}

```

取消审计响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|status|string|false|none||取消状态。|
|audit_id|string|false|none||被取消的审计任务 ID。|

<h2 id="tocS_AuditListResponse">AuditListResponse</h2>

<a id="schemaauditlistresponse"></a>
<a id="schema_AuditListResponse"></a>
<a id="tocSauditlistresponse"></a>
<a id="tocsauditlistresponse"></a>

```json
{
  "audits": [
    {
      "id": "string",
      "cluster_fingerprint": "string",
      "cluster_display": "string",
      "status": "running",
      "result": {
        "findings": [
          {
            "severity": null,
            "category": null,
            "resource": null,
            "risk": null,
            "impact": null,
            "suggestion": null
          }
        ],
        "summary": {
          "total": 0,
          "critical": 0,
          "high": 0,
          "medium": 0,
          "low": 0
        },
        "markdown": "string",
        "duration_ms": 0
      },
      "error_message": "string",
      "duration_ms": 0,
      "created_at": "2019-08-24T14:15:22Z"
    }
  ],
  "total": 18
}

```

审计列表响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|audits|[[Audit](#schemaaudit)]|false|none||审计任务数组。|
|total|integer|false|none||符合条件的审计任务总数。|

<h2 id="tocS_AuditStatusResponse">AuditStatusResponse</h2>

<a id="schemaauditstatusresponse"></a>
<a id="schema_AuditStatusResponse"></a>
<a id="tocSauditstatusresponse"></a>
<a id="tocsauditstatusresponse"></a>

```json
{
  "audit_id": "string",
  "status": "running",
  "created_at": "2019-08-24T14:15:22Z",
  "target": {
    "cluster": "string",
    "cluster_display": "string"
  },
  "events": [
    {
      "audit_id": "string",
      "seq_num": 0,
      "event_type": "string",
      "message": "string",
      "summary": "string",
      "detail": "string",
      "payload_kind": "string",
      "payload_json": "string",
      "elapsed_ms": 0,
      "created_at": 0
    }
  ],
  "result": {
    "findings": [
      {
        "severity": "critical",
        "category": "string",
        "resource": "string",
        "risk": "string",
        "impact": "string",
        "suggestion": "string"
      }
    ],
    "summary": {
      "total": 0,
      "critical": 0,
      "high": 0,
      "medium": 0,
      "low": 0
    },
    "markdown": "string",
    "duration_ms": 0
  },
  "error_message": "string"
}

```

审计详情响应。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|audit_id|string|false|none||审计任务 ID。|
|status|string|false|none||当前审计状态。|
|created_at|string(date-time)|false|none||审计创建时间。|
|target|[AuditTarget](#schemaaudittarget)|false|none||审计目标集群。|
|events|[[AuditEventRecord](#schemaauditeventrecord)]|false|none||已记录的审计事件。|
|result|[AuditResult](#schemaauditresult)|false|none||结构化安全审计结果。|
|error_message|string|false|none||审计失败时的错误信息。|

#### 枚举值

|属性|值|
|---|---|
|status|running|
|status|completed|
|status|failed|
|status|cancelled|

<h2 id="tocS_Audit">Audit</h2>

<a id="schemaaudit"></a>
<a id="schema_Audit"></a>
<a id="tocSaudit"></a>
<a id="tocsaudit"></a>

```json
{
  "id": "string",
  "cluster_fingerprint": "string",
  "cluster_display": "string",
  "status": "running",
  "result": {
    "findings": [
      {
        "severity": "critical",
        "category": "string",
        "resource": "string",
        "risk": "string",
        "impact": "string",
        "suggestion": "string"
      }
    ],
    "summary": {
      "total": 0,
      "critical": 0,
      "high": 0,
      "medium": 0,
      "low": 0
    },
    "markdown": "string",
    "duration_ms": 0
  },
  "error_message": "string",
  "duration_ms": 0,
  "created_at": "2019-08-24T14:15:22Z"
}

```

审计任务摘要或持久化记录。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||审计任务 ID。|
|cluster_fingerprint|string|false|none||集群指纹。|
|cluster_display|string|false|none||面向用户展示的集群名称。|
|status|string|false|none||审计状态。|
|result|[AuditResult](#schemaauditresult)|false|none||结构化安全审计结果。|
|error_message|string|false|none||审计失败时的错误信息。|
|duration_ms|integer(int64)|false|none||审计耗时，单位毫秒。|
|created_at|string(date-time)|false|none||审计创建时间。|

#### 枚举值

|属性|值|
|---|---|
|status|running|
|status|completed|
|status|failed|
|status|cancelled|

<h2 id="tocS_AuditTarget">AuditTarget</h2>

<a id="schemaaudittarget"></a>
<a id="schema_AuditTarget"></a>
<a id="tocSaudittarget"></a>
<a id="tocsaudittarget"></a>

```json
{
  "cluster": "string",
  "cluster_display": "string"
}

```

审计目标集群。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|cluster|string|false|none||集群指纹或内部集群标识。|
|cluster_display|string|false|none||面向用户展示的集群名称。|

<h2 id="tocS_AuditEventRecord">AuditEventRecord</h2>

<a id="schemaauditeventrecord"></a>
<a id="schema_AuditEventRecord"></a>
<a id="tocSauditeventrecord"></a>
<a id="tocsauditeventrecord"></a>

```json
{
  "audit_id": "string",
  "seq_num": 0,
  "event_type": "string",
  "message": "string",
  "summary": "string",
  "detail": "string",
  "payload_kind": "string",
  "payload_json": "string",
  "elapsed_ms": 0,
  "created_at": 0
}

```

审计事件记录。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|audit_id|string|false|none||所属审计任务 ID。|
|seq_num|integer|false|none||事件序号，递增并用于 SSE 断点续传。|
|event_type|string|false|none||事件类型。|
|message|string|false|none||面向用户的事件消息。|
|summary|string|false|none||事件摘要。|
|detail|string|false|none||事件详细信息。|
|payload_kind|string|false|none||事件载荷类型。|
|payload_json|string|false|none||JSON 字符串形式的事件载荷。|
|elapsed_ms|integer|false|none||本步骤耗时，单位毫秒。|
|created_at|integer(int64)|false|none||事件创建时间戳。|

<h2 id="tocS_AuditResult">AuditResult</h2>

<a id="schemaauditresult"></a>
<a id="schema_AuditResult"></a>
<a id="tocSauditresult"></a>
<a id="tocsauditresult"></a>

```json
{
  "findings": [
    {
      "severity": "critical",
      "category": "string",
      "resource": "string",
      "risk": "string",
      "impact": "string",
      "suggestion": "string"
    }
  ],
  "summary": {
    "total": 0,
    "critical": 0,
    "high": 0,
    "medium": 0,
    "low": 0
  },
  "markdown": "string",
  "duration_ms": 0
}

```

结构化安全审计结果。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|findings|[[AuditFinding](#schemaauditfinding)]|false|none||安全风险发现列表。|
|summary|[AuditSummary](#schemaauditsummary)|false|none||审计风险数量汇总。|
|markdown|string|false|none||适合直接展示的 Markdown 审计报告。|
|duration_ms|integer(int64)|false|none||审计耗时，单位毫秒。|

<h2 id="tocS_AuditFinding">AuditFinding</h2>

<a id="schemaauditfinding"></a>
<a id="schema_AuditFinding"></a>
<a id="tocSauditfinding"></a>
<a id="tocsauditfinding"></a>

```json
{
  "severity": "critical",
  "category": "string",
  "resource": "string",
  "risk": "string",
  "impact": "string",
  "suggestion": "string"
}

```

安全审计风险发现。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|severity|string|false|none||风险严重程度。|
|category|string|false|none||风险类别。|
|resource|string|false|none||关联 Kubernetes 资源。|
|risk|string|false|none||风险描述。|
|impact|string|false|none||影响说明。|
|suggestion|string|false|none||修复或缓解建议。|

#### 枚举值

|属性|值|
|---|---|
|severity|critical|
|severity|high|
|severity|medium|
|severity|low|

<h2 id="tocS_AuditSummary">AuditSummary</h2>

<a id="schemaauditsummary"></a>
<a id="schema_AuditSummary"></a>
<a id="tocSauditsummary"></a>
<a id="tocsauditsummary"></a>

```json
{
  "total": 0,
  "critical": 0,
  "high": 0,
  "medium": 0,
  "low": 0
}

```

审计风险数量汇总。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|total|integer|false|none||风险总数。|
|critical|integer|false|none||严重风险数量。|
|high|integer|false|none||高风险数量。|
|medium|integer|false|none||中风险数量。|
|low|integer|false|none||低风险数量。|

<h2 id="tocS_Activity">Activity</h2>

<a id="schemaactivity"></a>
<a id="schema_Activity"></a>
<a id="tocSactivity"></a>
<a id="tocsactivity"></a>

```json
{
  "id": "string",
  "type": "diagnosis",
  "text": "完成 default/nginx 的故障诊断。",
  "cluster_display": "kind-a",
  "diagnosis_id": "string",
  "created_at": "2019-08-24T14:15:22Z"
}

```

活动流记录。

### 属性

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|id|string|false|none||活动 ID。|
|type|string|false|none||活动类型。|
|text|string|false|none||活动展示文本。|
|cluster_display|string|false|none||关联集群展示名称。|
|diagnosis_id|string|false|none||关联诊断任务 ID，仅诊断类活动可能存在。|
|created_at|string(date-time)|false|none||活动创建时间。|

#### 枚举值

|属性|值|
|---|---|
|type|diagnosis|
|type|cluster_switch|
|type|system|

