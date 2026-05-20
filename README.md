# KubeWise

面向 Kubernetes 集群的智能自动运维 Agent 系统，基于大语言模型的自然语言理解与推理能力实现集群管理的自然语言交互。

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.24+-326CE5?logo=kubernetes)](https://kubernetes.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue)](LICENSE)

---

## 功能

| 功能 | 说明 |
|------|------|
| 查询 | 跨资源联合推理查询，自然语言描述即可获取集群状态 |
| 操作 | 扩缩容、重启、删除、apply YAML 等，执行前需用户确认 |
| 故障排查 | 自动检查 Pod 状态、事件、日志，分析根因并给出修复建议 |
| 安全审计 | RBAC/Pod 安全/网络策略/镜像安全四维审计 |
| 应用部署 | 基于 Helm Chart 的一键部署，LLM 自动生成配置，支持 NL 修正 |

## 快速开始

### 环境要求

- Go 1.26+
- Kubernetes 1.24+（kubeconfig 已配置）
- 大模型 API 服务（智谱 AI、阿里云通义千问、DeepSeek 等）

### 编译安装

```bash
git clone https://github.com/kubewise/kubewise.git
cd kubewise
go build -o kubewise ./cmd
```

### 配置

```bash
cp examples/config.yaml ~/.kubewise.yaml
```

编辑 `~/.kubewise.yaml`，填入 API Key：

```yaml
llm:
  model: "glm-5.1"
  api_key: "your-api-key"
  api_base: "https://open.bigmodel.cn/api/paas/v4/"
```

### 使用

**CLI 模式（单次查询）：**

```bash
# 查资源
kubewise chat "列出所有 namespaces"

# 查异常
kubewise chat "检查有没有 CrashLoopBackOff 的 Pod"

# 安全审计
kubewise chat "扫描集群中的安全风险"

# 执行操作
kubewise chat "把 nginx 扩容到 3 个副本"

# 部署应用
kubewise chat "部署一个 nginx ingress controller"
```

**TUI 模式（多轮对话）：**

```bash
kubewise tui
```

**API 模式：**

```bash
kubewise serve
```

REST + SSE，详见 [docs/api.md](docs/api.md)。

TUI 快捷键：

| 按键 | 功能 |
|------|------|
| Enter | 发送消息 |
| Ctrl+N | 新建会话 |
| Ctrl+C | 中断当前查询 |
| Ctrl+L | 清空会话 |
| Tab | 切换焦点（侧边栏 ↔ 输入框） |
| /resume | 重发被中断的消息 |

操作和部署在执行前会弹出确认界面，支持审查配置、按 Y 执行、按 E 编辑、按 C 输入修正指令。

---

## 架构

```
用户输入 (CLI / TUI / API)
       │
       ▼
┌──────────────────┐
│   Router Agent   │── LLM 意图分类 → 路由到子 Agent
└──────┬───────────┘
       │
       ├── Query Agent          ── 多工具 ReAct 推理
       ├── Operation Agent      ── 规划 → 确认 → 执行
       ├── Troubleshooting Agent── 系统诊断 → 根因分析
       ├── Security Agent       ── RBAC/网络/镜像/容器审计
       └── Deploy Agent         ── Helm 多阶段部署流水线
               │
               ▼
┌──────────────────┐
│  工具注册中心     │── 按分类加载，热插拔
└──────┬───────────┘
       │
       ▼
┌──────────────────┐
│  Kubernetes 集群  │── client-go v0.36
└──────────────────┘
```

### Deploy Agent 部署流程

```
解析应用 → ArtifactHub 选 Chart → LLM 生成配置 → 用户确认
→ 预检 → Helm 部署 → 校验；失败时 ReAct 诊断修复
```

---

## Agent 详情

| Agent | 方式 | 说明 |
|-------|------|------|
| Router | 单次 LLM 分类 | 识别 5 种意图类型，提取实体，路由到子 Agent |
| Query | ReAct 循环（默认 20 轮） | 调用查询工具，自动纠错，跨资源联合推理 |
| Operation | 规划 → 确认 → 执行 | LLM 生成操作计划，逐步骤用户确认，支持 NL 修正重规划 |
| Troubleshooting | 收集 → 分析 → 建议 | 自动检查 Pod 状态、事件、日志，输出根因和修复建议 |
| Security | 四维并行审计 | RBAC 权限、Pod 安全配置、网络策略覆盖、镜像安全风险 |
| Deploy | 状态机流水线 | 选 Chart、生成配置、确认、预检部署、校验，失败自动修复 |

---

## 工具清单

| 类别 | 数量 | 说明 |
|------|------|------|
| 查询 | 13 | 资源列表、详情、用量、ConfigMap、CRD、GVR 查询 |
| 操作 | 6 | 扩缩容、滚动重启、删除、apply、节点封锁/驱逐、标签编辑 |
| 故障排查 | 4 | Pod 日志、K8s 事件、节点状态、Service Endpoints |
| 安全审计 | 4 | RBAC、Pod 安全、网络策略、镜像安全 |

---

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go 1.26 |
| CLI | Cobra + Viper |
| K8s | client-go v0.36（静态 + Dynamic Client） |
| LLM | openai-go v3（兼容 GLM、Qwen、DeepSeek 等） |
| TUI | bubbletea + lipgloss + bubbles |
| Helm | helm.sh/helm/v4 |
| 日志 | zap |
| 会话 | JSON 持久化到 `~/.kubewise/sessions/` |

---

## 许可证

Apache License 2.0
