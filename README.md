# KubeWise

面向Kubernetes集群的智能自动运维Agent系统，将大语言模型的自然语言理解与推理能力与Kubernetes丰富的API生态深度融合。

## 功能特点

### 🔧 四大核心功能
1. **一句话操作**：自然语言转Kubernetes操作，无需记忆复杂的kubectl命令
2. **智能查询**：跨资源联合推理查询，支持复杂问题的多步骤分析
3. **自动故障排查**：异常检测与根因分析，自动收集上下文信息并给出修复建议
4. **安全合规检测**：RBAC权限审计，识别权限过大的ServiceAccount和高危权限配置

### 🎯 技术优势
- 基于ADK架构的有状态多Agent协作，支持复杂工作流编排
- 原生支持Kubernetes Go客户端，性能优异，资源占用低
- 兼容所有支持OpenAI API格式的大模型，优先支持国产大模型（GLM、Qwen、DeepSeek等）
- 工具接口符合MCP协议标准，可与Claude Desktop、Cursor等主流AI助手原生集成

## 快速开始

### 环境要求
- Go 1.22+
- Kubernetes 1.24+
- 可用的大模型API服务（智谱AI、阿里云通义千问、DeepSeek等）

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

2. 编辑配置文件，填写你的LLM API Key和相关配置。

### 使用示例

#### 查询类操作
```bash
# 列出所有命名空间
kubewise chat "列出所有命名空间"

# 查询default命名空间下的所有Pod
kubewise chat "列出default命名空间下的Pod"

# 查找最大的PV及其挂载的Pod
kubewise chat "哪个PV占用空间最大，挂载到了哪个Pod"

# 查询Pod的资源配置
kubewise chat "查看default命名空间下nginx Pod的资源配置"
```

#### 操作类操作（开发中）
```bash
# 部署Nginx应用
kubewise chat "帮我在default命名空间部署一个Nginx应用，副本数2个"

# 扩容Deployment
kubewise chat "把default命名空间下的nginx Deployment扩容到3个副本"
```

#### 故障排查（开发中）
```bash
# 排查启动失败的Pod
kubewise chat "检查有没有崩溃的Pod，分析原因"

# 排查服务访问问题
kubewise chat "为什么default命名空间下的nginx服务访问不了"
```

#### 安全审计（开发中）
```bash
# 检查RBAC权限配置
kubewise chat "执行安全扫描，检查有没有权限过大的ServiceAccount"

# 审计高危权限
kubewise chat "检查有没有拥有exec权限的Role"
```

## 项目架构

```
┌───────────────────────────────────────────────────┐
│                      用户输入                      │
└───────────────────────────────────────────────────┘
                            │
                            ▼
┌───────────────────────────────────────────────────┐
│                   Router Agent                    │
│  意图分类 / 任务路由 / 实体提取 / 多步骤任务拆分   │
└───────────────────────────────────────────────────┘
             ┌──────────┬──────────┬──────────┐
             ▼          ▼          ▼          ▼
┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ 操作Agent    │ │ 查询Agent    │ │ 故障排查Agent│ │ 安全审计Agent│
│ 执行资源操作 │ │ 跨资源查询   │ │ 异常根因分析 │ │ RBAC审计     │
└──────────────┘ └──────────────┘ └──────────────┘ └──────────────┘
             └──────────┬──────────┴──────────┘
                        ▼
┌───────────────────────────────────────────────────┐
│                   工具注册表                      │
│ K8s API / Helm SDK / 日志查询 / 事件监听 / RBAC   │
└───────────────────────────────────────────────────┘
                        │
                        ▼
┌───────────────────────────────────────────────────┐
│                   Kubernetes集群                  │
└───────────────────────────────────────────────────┘
```

## 技术栈
- **Agent框架**: ADK-Go (有状态多Agent编排)
- **LLM后端**: 支持所有兼容OpenAI API的大模型，优先支持国产模型
- **K8s接入**: 官方Kubernetes Go Client
- **应用部署**: Helm SDK v3
- **工具协议**: MCP (Model Context Protocol)
- **可观测性**: K8s Events Watch API
- **安全审计**: 自研RBAC分析引擎 + Kubescape集成

## 当前进展
✅ 基础架构搭建
✅ Router Agent 意图分类功能
✅ Query Agent 基础查询功能
✅ K8s客户端封装
✅ LLM客户端封装
✅ 基础工具集（PV/PVC/Pod查询等）
🚧 操作Agent功能开发中
🚧 故障排查Agent功能开发中
🚧 安全审计Agent功能开发中
🚧 ADK状态流集成

## 贡献指南
欢迎提交Issue和Pull Request！

## 许可证
Apache License 2.0
