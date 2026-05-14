# KubeWise

面向 Kubernetes 集群的智能自动运维 Agent 系统，将大语言模型的自然语言理解与推理能力与 Kubernetes 丰富的 API 生态深度融合。

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)](https://go.dev)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.24+-326CE5?logo=kubernetes)](https://kubernetes.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue)](LICENSE)

---

## 📋 功能特点

### 🔧 四大核心功能

| 功能 | 状态 | 说明 |
|------|------|------|
| **一句话操作** | ✅ 已完成 | 自然语言转 Kubernetes 操作，无需记忆复杂的 kubectl 命令 |
| **智能查询** | ✅ 已完成 | 跨资源联合推理查询，支持复杂问题的多步骤分析 |
| **自动故障排查** | ✅ 已完成 | 异常检测与根因分析，自动收集上下文信息并给出修复建议 |
| **安全合规检测** | ✅ 已完成 | RBAC/Pod 安全/网络策略/镜像安全四维审计 |

### 🎯 技术优势

- **多 Agent 协同架构**：Router Agent 统一路由，Query/Operation/Troubleshooting/Security 四大垂直 Agent 各司其职
- **强大的工具系统**：25 个内置工具（查询 11 + 操作 6 + 故障排查 4 + 安全审计 4），热插拔式注册中心
- **TUI 交互界面**：基于 bubbletea 的全功能终端界面，支持多轮对话、会话管理、操作确认
- **原生 K8s 集成**：基于官方 `client-go` v0.36，支持静态和动态客户端
- **模型无关**：兼容所有支持 OpenAI API 格式的大模型，优先支持国产大模型（GLM、Qwen、DeepSeek 等）
- **低资源占用**：Go 编译为单二进制，无外部依赖进程

---

## 🚀 快速开始

### 环境要求

- Go 1.26+
- Kubernetes 1.24+
- 可用的大模型 API 服务（智谱 AI、阿里云通义千问、DeepSeek 等）

### 编译安装

```bash
# 克隆项目
git clone https://github.com/kubewise/kubewise.git
cd kubewise

# 编译
go build -o kubewise ./cmd

# 安装到系统路径
sudo mv kubewise /usr/local/bin/
```

### 配置

1. 复制示例配置文件到用户目录：

```bash
cp examples/config.yaml ~/.kubewise.yaml
```

2. 编辑配置文件，填写你的 LLM API Key 和相关配置：

```yaml
# LLM 配置
llm:
  model: "glm-5.1"          # 模型名称
  api_key: "your-api-key"   # 你的 API Key
  api_base: "https://open.bigmodel.cn/api/paas/v4/"  # API 地址
```

### 使用方式

KubeWise 提供两种交互模式：

#### 💬 CLI 模式（单次查询）

```bash
# 查询所有命名空间
kubewise chat "列出所有命名空间"

# 查询 PV 及其挂载的 Pod
kubewise chat "哪个 PV 占用空间最大，挂载到了哪个 Pod"

# 故障排查
kubewise chat "检查有没有崩溃的 Pod，分析原因"

# 安全审计
kubewise chat "执行安全扫描，检查权限过大的 ServiceAccount"

# 执行操作（需人工确认）
kubewise chat "把 nginx Deployment 扩容到 3 个副本"
```

#### 🖥️ TUI 模式（多轮对话）

```bash
kubewise tui
```

TUI 模式提供完整的交互体验：

| 快捷键 | 功能 |
|--------|------|
| `Enter` | 发送消息 |
| `Ctrl+N` | 新建会话 |
| `Ctrl+C` | 中断当前查询 |
| `Ctrl+L` | 清空当前会话 |
| `Tab` | 切换焦点（侧边栏 ↔ 输入框） |
| `/resume` | 重发被中断的消息 |

操作 Agent 在执行高危操作前会弹出确认弹窗，支持 **确认执行 / 输入修正指令重规划 / 跳过**。

---

## 🏗️ 项目架构

```
┌──────────────────────────────────────────────────────┐
│                    用户输入                            │
│           kubewise chat / kubewise tui                │
└────────────────────────┬─────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────┐
│                   Router Agent                        │
│        LLM 意图分类 / 实体提取 / 任务路由              │
└────────────┬──────────┬──────────┬───────────────────┘
             │          │          │
             ▼          ▼          ▼          ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│  Query Agent │ │OperationAgent│ │Troubleshooting│ │Security Agent│
│ 跨资源查询    │ │ 执行资源操作   │ │ 异常根因分析   │ │ 安全审计      │
│ ReAct 10轮   │ │ 规划→确认→执行 │ │ 系统性诊断     │ │ 四维审计      │
└──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘
       │                │                │                │
       └────────────────┴────────────────┴────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────┐
│                   工具注册中心                          │
│  查询 x11   操作 x6   故障排查 x4   安全审计 x4         │
│  (动态注册 + 分类加载 + 全局注册表)                     │
└────────────────────────┬─────────────────────────────┘
                         │
                         ▼
┌──────────────────────────────────────────────────────┐
│              Kubernetes 集群 (client-go)              │
└──────────────────────────────────────────────────────┘

TUI 模式架构:
  Router Agent ───→ Event Channel ──→ bubbletea App
                                        ├── Chat 区域（消息+进度卡片）
                                        ├── Input 输入框
                                        ├── Sidebar 会话列表
                                        └── Confirm 确认弹窗
```

---

## 🛠️ 工具清单

### 查询工具（11 个）

| 工具 | 功能 |
|------|------|
| `list_namespaces` | 列出所有命名空间 |
| `list_pods_in_namespace` | 列出命名空间中的 Pod |
| `list_persistent_volumes` | 列出所有 PV |
| `list_persistent_volume_claims` | 列出所有 PVC |
| `find_pods_using_pvc` | 查找使用指定 PVC 的 Pod |
| `get_pod_resource_usage` | 获取 Pod 资源使用量 |
| `list_configmaps_in_namespace` | 列出命名空间中的 ConfigMap |
| `get_configmap_content` | 获取 ConfigMap 内容 |
| `list_custom_resources_by_gvr` | 按 GVR 列出自定义资源 |
| `get_custom_resource_by_gvr_and_name` | 按 GVR 和名称查询自定义资源 |
| `list_resources_by_gvr` | 按 GVR 列出任意资源 |

### 操作工具（6 个）

| 工具 | 功能 |
|------|------|
| `scale` | 调整 Deployment/StatefulSet 副本数 |
| `restart` | 触发滚动重启 |
| `delete` | 通过 GVR 删除任意资源 |
| `apply` | Server-Side Apply YAML |
| `cordon_drain` | 节点封锁/解封/驱逐 |
| `label_annotate` | 修改标签/注解 |

### 故障排查工具（4 个）

| 工具 | 功能 |
|------|------|
| `get_pod_logs` | 获取 Pod 容器日志 |
| `get_resource_events` | 获取资源相关 K8s 事件 |
| `get_node_status` | 获取节点状态和可分配资源 |
| `get_service_endpoints` | 获取 Service Endpoints |

### 安全审计工具（4 个）

| 工具 | 功能 |
|------|------|
| `audit_rbac` | RBAC 配置审计（cluster-admin 滥用、通配符权限、exec/portforward 授权等） |
| `audit_pod_security` | Pod 安全配置审计（privileged 容器、hostNetwork/hostPID 等） |
| `audit_network_policies` | 网络策略审计（无 NetworkPolicy 的命名空间） |
| `audit_image_security` | 镜像安全审计（latest 标签、imagePullPolicy: Never 等） |

---

## 🧩 技术栈

| 组件 | 技术选型 |
|------|----------|
| **语言** | Go 1.26 |
| **CLI 框架** | [Cobra](https://github.com/spf13/cobra) + [Viper](https://github.com/spf13/viper) |
| **K8s 客户端** | [client-go](https://github.com/kubernetes/client-go) v0.36（静态 + Dynamic Client） |
| **LLM 客户端** | [openai-go](https://github.com/openai/openai-go) v3 SDK |
| **TUI 框架** | [bubbletea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [Bubbles](https://github.com/charmbracelet/bubbles) |
| **语法高亮** | [chroma](https://github.com/alecthomas/chroma) v2 |
| **日志** | [zap](https://go.uber.org/zap) |
| **工具系统** | 自研动态注册中心（按分类加载） |
| **会话持久化** | JSON 文件 → `~/.kubewise/sessions/` |

---

## 📊 当前进展

### Agent 系统 — 全部完成 ✅

| Agent | 文件 | 核心能力 |
|-------|------|----------|
| **Router Agent** | `pkg/agent/router/agent.go` | LLM 意图分类 + 实体提取 + 任务路由 + HandleQueryStream |
| **Query Agent** | `pkg/agent/query/agent.go` | ReAct 循环（最多 10 轮工具调用），自动纠错 |
| **Operation Agent** | `pkg/agent/operation/agent.go` | LLM 规划 → 用户确认 → 执行，支持修正重规划 |
| **Troubleshooting Agent** | `pkg/agent/troubleshooting/agent.go` | 系统性信息收集 → 根因分析 → 修复建议 |
| **Security Agent** | `pkg/agent/security/agent.go` | RBAC/Pod 安全/网络策略/镜像安全四维审计 |

### TUI 交互界面 — 全部完成 ✅

| 模块 | 文件 | 功能 |
|------|------|------|
| App 主模型 | `pkg/tui/app.go` | 组合所有子模型，管理会话生命周期 |
| 事件系统 | `pkg/tui/events/events.go` | 14 种 TUIEvent 类型 |
| 会话管理 | `pkg/tui/session/` | JSON 持久化到 `~/.kubewise/sessions/` |
| 聊天区域 | `pkg/tui/model/chat.go` | 消息历史 + 进度卡片 + 动画 spinner |
| 输入框 | `pkg/tui/model/input.go` | bubbles/textarea 封装 |
| 确认弹窗 | `pkg/tui/model/confirm.go` | 操作步骤确认/修正/跳过 |
| 侧边栏 | `pkg/tui/model/sidebar.go` | 会话列表 + Tab 切换焦点 |
| 渲染器 | `pkg/tui/model/renderer.go` | 表格/代码/KV/列表 → 样式化输出 |
| 样式 | `pkg/tui/styles/styles.go` | lipgloss 颜色和边框常量 |

### 基础设施 — 全部完成 ✅

| 组件 | 状态 |
|------|------|
| CLI 入口（chat + tui 子命令） | ✅ 已完成 |
| K8s 客户端封装 | ✅ 已完成 |
| LLM 客户端（兼容 OpenAI API） | ✅ 已完成 |
| 动态工具注册中心 | ✅ 已完成 |
| 配置管理（Viper） | ✅ 已完成 |

---

## 👥 贡献指南

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

Apache License 2.0
