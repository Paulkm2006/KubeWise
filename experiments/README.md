# KubeWise 多集群故障实验

## 概述

本实验在本地使用 [kind](https://kind.sigs.k8s.io/) 创建 **3 个 K8s 集群**，分别注入不同类型的故障，用来验证 KubeWise 的前端 Dashboard 展示效果和多集群诊断能力。

## 前提

| 工具 | 版本要求 | 用途 |
|------|---------|------|
| Docker | ≥ 24.0 | 运行 kind 节点容器 |
| kind | ≥ 0.20 | 创建本地 K8s 集群 |
| kubectl | ≥ 1.28 | 操作集群 |
| go | ≥ 1.22（可选） | 编译 KubeWise 后端 |

运行 `bash setup.sh` 会自动检测上述依赖。

## 快速开始

```bash
# 1. 搭建实验环境（创建 3 个集群 + 注入故障）
cd experiments
bash setup.sh        # Linux / macOS
# 或 pwsh setup.ps1  # Windows / PowerShell

# 2. 修改 KubeWise 配置指向实验集群
#    编辑 config.yaml，将 kubeconfig 改为：
#    kubeconfig: "experiments/.kube/merged.kubeconfig"

# 3. 启动 KubeWise
go build -o kubewise ./cmd && ./kubewise serve --config config.yaml -v --log-file stderr

# 4. 启动前端
cd frontend && npx run dev
```

打开浏览器访问 `http://localhost:5173`，Dashboard 上应该能看到 3 个集群和对应的故障。

## 环境清理

```bash
cd experiments
bash cleanup.sh          # Linux / macOS
# 或 pwsh cleanup.ps1    # Windows / PowerShell
```

## 实验设计

### 集群分布

```
┌─────────────────────────────────────────────────┐
│                    kind 宿主机                      │
│                                                   │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────┐ │
│  │  kw-exp-a    │  │  kw-exp-b    │  │ kw-exp-c │ │
│  │  control-p   │  │  control-p   │  │ control-p│ │
│  │  worker ×2   │  │  worker ×2   │  │ worker ×2│ │
│  │              │  │              │  │          │ │
│  │ ● nginx     │  │ ● nginx     │  │ ● nginx  │ │
│  │ ● crashloop │  │ ● oom       │  │ ● pending│ │
│  │              │  │ ● imagepull │  │          │ │
│  └──────────────┘  └──────────────┘  └──────────┘ │
└─────────────────────────────────────────────────┘
```

| 集群 | API 端口 | 健康应用 | 注入故障 | 类型 |
|------|---------|---------|---------|------|
| kw-exp-a | 16500 | nginx | CrashLoopBackOff | high severity |
| kw-exp-b | 16501 | nginx | OOMKilled + ImagePullBackOff | high / medium |
| kw-exp-c | 16502 | nginx | Pending（资源不足） | medium |

### 故障说明

| 故障 | 模拟方式 | 预期表现 | 诊断验证点 |
|------|---------|---------|-----------|
| CrashLoopBackOff | `busybox` 启动 3s 后 `exit 1`，K8s 反复重启 | Pod 状态反复 Crash → Running → Crash | troubleshooting Agent 应检测到非零退出码 |
| OOMKilled | `stress --vm-bytes 300M` + memory limit 100Mi | Pod OOMKilled，K8s 自动重启 | Agent 应发现内存超限 |
| ImagePullBackOff | 不存在的镜像 `nonexistent.example.com/fake-app:v99.9.9` | Pod 持续 ImagePullBackOff | Agent 应发现镜像拉取失败 |
| Pending | 请求 1000 CPU + 512Gi 内存 | Pod 一直 Pending（无法调度） | Agent 应发现资源不足 |