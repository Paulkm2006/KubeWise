# KubeWise 操作指南

本指南帮助你快速上手 KubeWise 的各项功能，覆盖从首次登录到日常运维的完整操作流程。

---

## 目录

- [首次启动与登录](#首次启动与登录)
- [Dashboard 仪表盘](#dashboard-仪表盘)
  - [查看集群概览](#查看集群概览)
  - [查看异常 Pod 列表](#查看异常-pod-列表)
  - [发起 Pod 诊断](#发起-pod-诊断)
- [Pod 诊断](#pod-诊断)
  - [查看诊断进度](#查看诊断进度)
  - [阅读诊断报告](#阅读诊断报告)
- [安全审计](#安全审计)
  - [发起安全审计](#发起安全审计)
  - [查看审计报告](#查看审计报告)
  - [导出审计报告](#导出审计报告)
- [智能对话](#智能对话)
  - [常用提问示例](#常用提问示例)
  - [人机协同操作](#人机协同操作)
  - [Helm 应用部署](#helm-应用部署)
- [TUI 终端交互](#tui-终端交互)
  - [启动 TUI](#启动-tui)
  - [TUI 常用操作](#tui-常用操作)
- [CLI 命令行](#cli-命令行)
  - [单次查询](#单次查询)
  - [启动 API 服务](#启动-api-服务)
- [常见问题排查](#常见问题排查)

---

## 首次启动与登录

### 1. 启动服务

**Docker Compose 方式**（推荐快速体验）：

```bash
export KUBEWISE_LLM_API_KEY="your-api-key"
export KUBEWISE_LLM_MODEL="glm-5.1"
export KUBEWISE_LLM_API_BASE="https://open.bigmodel.cn/api/paas/v4/"
docker compose up --build
```

**本地运行方式**：

```bash
# 终端 1：启动后端
go build -o kubewise ./cmd
./kubewise serve --addr :8080

# 终端 2：启动前端
cd frontend
npm install
VITE_PROXY_TARGET=http://localhost:8080 npm run dev
```

### 2. 访问 Web 控制台

打开浏览器访问：

- Docker Compose：`http://localhost:3000`
- 本地开发：`http://localhost:5173`

<p align="center">
  <img src="assets\usage\first-time.png" alt="Web 控制台首页" width="800">
</p>

<p align="center">
  <sub>首次打开 Web 控制台，Dashboard 显示所有已连接集群的概览信息。</sub>
</p>

### 3. 验证连接

确认后端服务正常运行：

```bash
curl http://localhost:8080/health
# 应返回 OK 或类似的成功响应
```

确认集群连接正常 — Dashboard 页面应显示你的集群列表和节点数量。

---

## Dashboard 仪表盘

Dashboard 是 KubeWise 的默认首页，提供多集群的全局视角。

### 查看集群概览

<p align="center">
  <img src="assets/readme/dashboard.png" alt="Dashboard 集群概览" width="800">
</p>

Dashboard 顶部展示以下关键指标：

| 指标 | 说明 |
|---|---|
| **集群数量** | 从 kubeconfig 中读取的可用集群总数 |
| **节点总数** | 所有集群的节点数汇总 |
| **命名空间数** | 所有集群的命名空间总数 |
| **问题 Pod 数** | 非 Running/Succeeded 状态的 Pod 总数 |

每个集群单独显示健康状态卡片，包含该集群的节点数、命名空间数、Pod 就绪率和问题数。

### 查看异常 Pod 列表

Dashboard 下方的「问题 Pod」列表聚合了所有非正常状态的 Pod：

- **按严重程度排序**：`CrashLoopBackOff`、`OOMKilled`、`ImagePullBackOff`、`Pending` 等
- **按集群分组**：可以快速定位是哪个集群出了问题
- **显示关键信息**：Pod 名称、命名空间、状态、重启次数、所在节点

<p align="center">
  <img src="assets\usage\error-pods.png" alt="异常 Pod 列表" width="800">
</p>

<p align="center">
  <sub>异常 Pod 列表按严重程度排序，每个 Pod 旁有「诊断」按钮。</sub>
</p>

### 发起 Pod 诊断

在异常 Pod 列表中，点击目标 Pod 旁的「诊断」按钮即可启动证据链式诊断。

你也可以在 Chat 标签页中通过自然语言发起诊断：

```
诊断 default 命名空间的 nginx-pod
```

---

## Pod 诊断

Pod 诊断是 KubeWise 的核心功能，通过多阶段证据采集和假设验证，给出结构化的根因分析报告。

### 查看诊断进度

诊断启动后，界面会自动跳转到诊断视图，实时展示诊断进度：

<p align="center">
  <img src="assets\usage\diag-status.png" alt="诊断进度" width="800">
</p>

<p align="center">
  <sub>诊断进度面板实时显示当前阶段、已采集证据数、工具调用记录和候选假设。</sub>
</p>

诊断流程的六个阶段：

| 阶段 | 说明 | 预期耗时 |
|---|---|---|
| **基础采集** | 读取 Pod 状态、Events、容器日志 | 5-10 秒 |
| **补充观测** | 按需读取资源用量、Pod 详情、关联对象 | 10-20 秒 |
| **证据构建** | 将原始数据整理为结构化证据清单 | 5-10 秒 |
| **候选根因** | 生成多个可验证的故障假设 | 5-10 秒 |
| **假设验证** | 用工具调用和证据逐一验证/反驳假设 | 15-30 秒 |
| **报告生成** | 输出结构化诊断报告 | 5-10 秒 |

> 整个诊断过程通常在 1-2 分钟内完成，具体时间取决于集群规模和故障复杂度。

### 阅读诊断报告

诊断完成后，会展示结构化报告：

<p align="center">
  <img src="assets/readme/diagnosis-report.png" alt="诊断报告" width="600">
</p>

报告包含以下部分：

| 部分 | 说明 |
|---|---|
| **根因判断** | 最终认定的根本原因 |
| **证据链** | 支持根因判断的证据列表，附带来源和采集时间 |
| **已验证假设** | 被验证通过或被反驳的假设及其依据 |
| **修复建议** | 针对根因的具体修复步骤和命令 |
| **影响范围** | 该问题可能影响的其他资源或服务 |
| **局限说明** | 诊断过程中的信息盲区或不确定因素 |

> **提示**：报告中的「局限说明」很重要 — 它说明了 AI 在哪些地方缺乏信息或存在不确定性，帮助你判断建议的可信度。

---

## 安全审计

安全审计功能对 Kubernetes 集群进行四类安全风险检查。

### 发起安全审计

**方式一：Web 控制台**

1. 点击顶部导航栏的「Audit」标签页
2. 选择目标集群
3. 点击「开始审计」按钮

<p align="center">
  <img src="assets\usage\sec-audit.png" alt="安全审计页面" width="800">
</p>

**方式二：Chat 对话**

```
扫描当前集群的安全风险
对 my-cluster 做安全审计
```

### 查看审计报告

审计完成后，报告按四大类别展示发现的风险：

| 类别 | 检查内容 |
|---|---|
| **RBAC** | 过度授权的 ClusterRole/Role、绑定到 default ServiceAccount 的权限等 |
| **Pod Security** | 特权容器、hostNetwork/hostPID/hostIPC、runAsRoot、缺少 SecurityContext |
| **NetworkPolicy** | 缺少 NetworkPolicy 的命名空间、过于宽松的出入站规则 |
| **镜像配置** | 使用 latest 标签、镜像来自不可信仓库、缺少镜像摘要 |

每条风险项包含：

- 风险等级（高/中/低）
- 涉及的资源名称和命名空间
- 风险描述
- 修复建议

### 导出审计报告

在审计报告页面，点击「导出」按钮可以将报告下载为文件，方便存档或分享给团队。

---

## 智能对话

Chat 标签页提供自然语言交互界面，支持五种意图类型的操作。

<p align="center">
  <img src="assets\usage\chat.png" alt="Chat 对话界面" width="800">
</p>

<p align="center">
  <sub>Chat 界面支持自然语言输入，AI 会自动识别意图并执行相应操作。</sub>
</p>

### 常用提问示例

**查询类**

```
列出所有 namespace
default 命名空间有哪些 Pod？
查看 nginx-deployment 的详情
哪些节点的内存使用率最高？
```

**排障类**

```
检查有没有 CrashLoopBackOff 的 Pod
default 命名空间的 nginx-pod 为什么一直重启？
这个 Pod 的事件有什么异常？
```

**安全类**

```
扫描当前集群的安全风险
检查有没有特权容器
哪些 ServiceAccount 权限过大？
```

**操作类**

```
把 nginx-deployment 扩容到 3 个副本
重启 default 命名空间的 nginx-pod
给 production 命名空间打上 env=prod 标签
```

**部署类**

```
部署一个 nginx 到 default 命名空间
搜索 Artifact Hub 上的 redis chart
安装 bitnami/redis 到 redis 命名空间
```

### 人机协同操作

当你请求执行写操作（扩缩容、重启、删除、apply、部署）时，KubeWise 不会直接执行，而是进入确认流程：

<p align="center">
  <img src="assets\usage\exec-confirm.png" alt="操作确认" width="600">
</p>

<p align="center">
  <sub>写操作会展示计划详情，用户确认后才执行。</sub>
</p>

确认流程：

1. **计划展示**：AI 展示即将执行的操作、目标资源和关键参数
2. **用户审查**：检查操作是否符合预期
3. **确认执行**：点击「确认」按钮执行，或点击「取消」放弃

> **安全提示**：永远在确认前检查操作详情。KubeWise 不会对你的确认做二次猜测 — 你点击确认就意味着你认可这个操作。

### Helm 应用部署

通过自然语言触发 Helm 应用部署的完整流程：

**第 1 步：表达部署意图**

```
部署一个 redis 到 default 命名空间
```

**第 2 步：选择 Chart**

AI 会搜索 Artifact Hub 并展示可用的 Chart 列表，你选择一个合适的。

**第 3 步：配置 Values**

AI 会根据 Chart 生成默认 values 配置，你可以要求修改：

```
把 redis 密码改成 mypassword，内存限制设为 512Mi
```

**第 4 步：预检与确认**

AI 执行预检（命名空间是否存在、资源是否充足等），然后展示部署计划供你确认。

**第 5 步：执行与验证**

确认后 AI 执行 `helm install`，并验证部署是否成功。

---

## TUI 终端交互

TUI 提供完全在终端内的交互体验，适合 SSH 远程运维或偏好终端的用户。

### 启动 TUI

```bash
kubewise tui
```

<p align="center">
  <img src="assets\readme\tui.png" alt="TUI 界面" width="700">
</p>

### TUI 常用操作

| 操作 | 说明 |
|---|---|
| **输入消息** | 在底部输入框中输入自然语言，按 `Enter` 发送 |
| **查看历史** | 左侧边栏显示会话列表，点击切换 |
| **新建会话** | 按 `Ctrl+N` 创建新会话 |
| **操作确认** | 写操作会弹出确认对话框，按 `y` 确认 / `n` 取消 |
| **滚动内容** | 使用 `↑`/`↓` 或 `PgUp`/`PgDn` 滚动查看对话内容 |
| **退出** | 按 `Ctrl+C` 或 `q` 退出 |

TUI 的对话体验与 Web 控制台的 Chat 一致，支持所有五种意图类型的操作。

---

## CLI 命令行

### 单次查询

CLI 适合快速执行一次查询或操作，执行完毕后自动退出：

```bash
# 查询
kubewise chat "列出所有 namespace"

# 排障
kubewise chat "检查 default 命名空间有没有异常 Pod"

# 安全
kubewise chat "扫描安全风险"

# 操作（会进入确认流程）
kubewise chat "把 nginx 扩容到 3 个副本"
```

### 启动 API 服务

将 KubeWise 作为 HTTP API 服务运行，供其他系统集成：

```bash
# 默认监听 :8080
kubewise serve

# 指定地址和端口
kubewise serve --addr :9090

# 启用详细日志
kubewise serve --addr :8080 -v
```

API 服务启动后，可以通过 HTTP 请求访问所有功能：

```bash
# 集群列表
curl http://localhost:8080/api/v1/clusters

# 创建会话
curl -X POST http://localhost:8080/api/v1/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "my session"}'

# 同步对话
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "列出所有 namespace", "session_id": ""}'

# SSE 流式对话
curl -N http://localhost:8080/api/v1/chat/stream \
  -H "Content-Type: application/json" \
  -d '{"message": "检查异常 Pod", "session_id": ""}'

# 创建诊断
curl -X POST http://localhost:8080/api/v1/diagnoses \
  -H "Content-Type: application/json" \
  -d '{"cluster": "my-cluster", "namespace": "default", "pod_name": "my-pod"}'
```

完整的 API 文档：

- [OpenAPI 规范](openapi.yaml)
- [API 详细文档](KubeWise%20API.md)
- [API 概览](api.md)

---

## 常见问题排查

### 服务启动失败

**问题**：`docker compose up` 后后端容器反复重启

**排查步骤**：

1. 查看后端日志：`docker compose logs backend`
2. 检查 LLM 配置是否正确（API Key、API Base URL）
3. 确认 kubeconfig 文件存在且可读：`ls -la ~/.kube/config`
4. 确认 kubeconfig 中的集群可达：`kubectl cluster-info`

---

**问题**：前端无法连接后端

**排查步骤**：

1. 确认后端健康检查通过：`curl http://localhost:8080/health`
2. Docker Compose 方式：确认前端容器的 Nginx 配置正确代理到 backend 服务
3. 本地开发方式：确认 `VITE_PROXY_TARGET` 环境变量指向正确的后端地址

---

### 诊断相关

**问题**：诊断一直卡在「基础采集」阶段

**排查步骤**：

1. 确认目标 Pod 存在：`kubectl get pod <pod-name> -n <namespace>`
2. 确认 kubeconfig 有权限读取该 Pod 的信息
3. 查看后端日志是否有权限错误

---

**问题**：诊断报告中提示信息不足

**说明**：这是正常情况。AI 会明确告知哪些信息无法获取（如权限不足、指标未安装等）。你可以：

1. 根据「局限说明」中提到的信息盲区，手动补充检查
2. 安装 metrics-server 等工具以提供更丰富的数据源
3. 参考「证据链」中的已有证据，结合自身经验做判断

---

### LLM 服务相关

**问题**：对话返回错误或超时

**排查步骤**：

1. 确认 LLM API Key 有效且有余额
2. 确认 API Base URL 正确（不同模型服务商的地址不同）
3. 检查网络是否能访问 LLM 服务（特别是使用国外模型时）
4. 尝试更换模型或服务商

---

**问题**：Agent 执行到一半被终止

**说明**：KubeWise 内置 Supervisor 机制，当 Agent 陷入循环时会自动干预。你可以在配置中调整阈值：

```yaml
agent:
  max_steps: 30          # 增加最大步数
  supervisor:
    max_extensions: 3     # 增加继续授权次数
    extension_step_grant: 15  # 每次授权增加更多步数
```

或临时禁用 Supervisor（不推荐用于生产环境）：

```bash
kubewise chat "你的问题" --no-supervisor
```

---

### 集群连接相关

**问题**：Dashboard 显示的集群信息不正确

**排查步骤**：

1. 确认 kubeconfig 中的 context 信息正确：`kubectl config get-contexts`
2. 如果使用 Kind 集群，确认集群正在运行：`docker ps | grep kind`
3. 重启后端服务以刷新集群连接

---

**问题**：多集群场景下部分集群无法连接

**说明**：KubeWise 会尝试连接 kubeconfig 中的所有 context。无法连接的集群会在 Dashboard 上标记为离线状态，不影响其他集群的正常使用。检查对应集群的网络连通性和认证信息即可。
