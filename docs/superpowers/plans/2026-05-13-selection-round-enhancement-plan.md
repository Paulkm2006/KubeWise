# KubeWise 选拔赛强化计划

> **生成时间**: 2026-05-13  
> **目标**: 2-4 周内完成核心功能增强，准备选拔赛提交  
> **策略**: 方向 A（Helm 集成 + 可视化工作流）+ 方向 C 部分（Watch API 监听）

---

## 一、现状诊断

### 1.1 项目完成度

| 模块 | 状态 | 评价 |
|------|------|------|
| 4 个 Agent（Router/Query/Operation/Troubleshooting/Security） | ✅ 完成 | 基本功能可用，8500 行代码，编译通过，有测试 |
| TUI 交互界面 | ✅ 完成 | bubbletea 多轮对话、会话持久化、确认弹窗 |
| 25 个工具（查询 11 + 操作 6 + 故障排查 4 + 安全审计 4） | ✅ 完成 | 动态注册中心，热插拔 |

### 1.2 核心 Gap 分析

| 文档承诺 | 实际状态 | 影响 | 优先级 |
|---------|---------|------|--------|
| **Helm SDK 集成** | ❌ 完全没有 | 无法实现"一句话部署应用"演示场景 | 🔴 P0 |
| **工作流可视化** | ❌ 没有 | 视频演示缺乏"Wow Factor" | 🔴 P0 |
| **Watch API 主动监控** | ❌ 没有 | 系统是被动响应，不是主动运维 | 🟡 P1 |
| ADK-Go 有状态执行图 | ❌ 没有 | 技术创新点未兑现（但重构风险高） | 🟢 P2 |
| MCP Server 支持 | ❌ 没有 | 生态集成缺失 | 🟢 P2 |
| Web UI | ❌ 没有 | 界面友好性不足 | 🟢 P3 |

### 1.3 选拔赛评分标准

| 维度 | 权重 | 当前短板 | 强化方向 |
|------|------|---------|---------|
| 创意 | 20% | 中等 | 保持现有定位 |
| **技术** | **30%** | **缺少 Helm 集成** | **补齐 Helm + 展示多步推理** |
| 应用 | 20% | 被动响应 | 增加 Watch 监听 |
| **设计** | **30%** | **缺少可视化** | **工作流可视化 + 高质量视频** |

**结论**: 必须补齐 Helm 集成（技术 30%）和工作流可视化（设计 30%），这两项占总分 60%。

---

## 二、战略方向

### 核心策略：方向 A + 方向 C 部分

**方向 A：Helm 集成 + 可视化工作流** [P0]
- 补齐文档承诺的核心功能
- 实现"一句话部署应用"演示场景
- TUI 增加工作流可视化，展示 Agent 推理过程

**方向 C 部分：Watch API 监听** [P1]
- 实现主动监控能力（不做自动修复，降低复杂度）
- 展示"实时发现问题"能力
- 与竞品形成差异化

**不做的事情**：
- ❌ ADK-Go 重构（风险高，时间不够）
- ❌ MCP Server（非核心，可后续补充）
- ❌ Web UI（TUI 已足够，视频演示效果好）

---

## 三、详细实施计划（14 天）

### 第 1 周：Helm 集成 + 可视化工作流

#### Day 1-3: Helm SDK 集成 [P0]

**目标**: 实现 3 个 Helm 工具，支持"一句话部署应用"

**任务清单**:

1. **添加 Helm 依赖** (0.5 天)
   ```bash
   go get helm.sh/helm/v3@latest
   ```
   - 修改 `go.mod`，添加 `helm.sh/helm/v3` 依赖
   - 验证编译通过

2. **创建 Helm 客户端封装** (0.5 天)
   - 文件: `pkg/helm/client.go`
   - 功能: 封装 Helm SDK，提供统一接口
   ```go
   type Client struct {
       actionConfig *action.Configuration
       settings     *cli.EnvSettings
   }
   
   func NewClient(kubeconfig string, namespace string) (*Client, error)
   func (c *Client) AddRepo(name, url string) error
   func (c *Client) UpdateRepo() error
   func (c *Client) Install(releaseName, chart string, values map[string]interface{}) error
   func (c *Client) Upgrade(releaseName, chart string, values map[string]interface{}) error
   func (c *Client) Uninstall(releaseName string) error
   func (c *Client) List() ([]*release.Release, error)
   func (c *Client) GetValues(releaseName string) (map[string]interface{}, error)
   ```

3. **实现 3 个 Helm 工具** (1.5 天)
   - 文件: `pkg/tools/v1/helm/install.go`
   ```go
   // helm_install: 安装 Helm Chart
   // 参数: release_name, chart, repo_url, namespace, values
   ```
   - 文件: `pkg/tools/v1/helm/upgrade.go`
   ```go
   // helm_upgrade: 升级 Helm Release
   // 参数: release_name, chart, values
   ```
   - 文件: `pkg/tools/v1/helm/uninstall.go`
   ```go
   // helm_uninstall: 卸载 Helm Release
   // 参数: release_name, namespace
   ```

4. **注册到工具中心** (0.5 天)
   - 修改 `pkg/tools/v1/helm/init.go`，实现 `init()` 自动注册
   - 分类: `operation`（操作类工具）
   - 测试: 编写单元测试 `pkg/tools/v1/helm/install_test.go`

**验收标准**:
- ✅ `go build ./...` 编译通过
- ✅ `go test ./pkg/tools/v1/helm` 测试通过
- ✅ CLI 测试: `kubewise chat "帮我安装 nginx-ingress"`

---

#### Day 4-5: 实现"一句话部署应用"场景 [P0]

**目标**: Operation Agent 能够规划并执行 Helm 部署任务

**任务清单**:

1. **扩展 Operation Agent 的 [REDACTED]** (0.5 天)
   - 文件: `pkg/agent/operation/agent.go`
   - 在 `buildPlanningSystemPrompt()` 中增加 Helm 工具说明
   ```
   支持的操作类型：
   - helm_install: 安装 Helm Chart，需填写 release_name, chart, repo_url, namespace, values
   - helm_upgrade: 升级 Helm Release
   - helm_uninstall: 卸载 Helm Release
   
   常见应用部署示例：
   - ArgoCD: chart="argo/argo-cd", repo_url="https://argoproj.github.io/argo-helm"
   - Prometheus: chart="prometheus-community/prometheus", repo_url="https://prometheus-community.github.io/helm-charts"
   - Nginx Ingress: chart="ingress-nginx/ingress-nginx", repo_url="https://kubernetes.github.io/ingress-nginx"
   ```

2. **实现 3 个演示场景** (1 天)
   - 场景 1: "帮我在 dev 命名空间部署 ArgoCD"
     - 预期: Agent 规划 → 添加 Repo → 安装 Chart → 验证状态 → 返回访问地址
   - 场景 2: "部署 Prometheus 监控"
     - 预期: Agent 规划 → 安装 Prometheus + Grafana → 配置 Service
   - 场景 3: "安装 Nginx Ingress Controller"
     - 预期: Agent 规划 → 安装 Ingress → 暴露 NodePort

3. **编写测试脚本** (0.5 天)
   - 文件: `examples/helm_scenarios.sh`
   ```bash
   #!/bin/bash
   # 场景 1: 部署 ArgoCD
   kubewise chat "帮我在 argocd 命名空间部署最新版的 ArgoCD，并暴露 NodePort"
   
   # 场景 2: 部署 Prometheus
   kubewise chat "部署 Prometheus 到 monitoring 命名空间"
   
   # 场景 3: 部署 Nginx Ingress
   kubewise chat "安装 Nginx Ingress Controller"
   ```

**验收标准**:
- ✅ 3 个场景全部能成功执行
- ✅ Agent 能正确规划多步骤（添加 Repo → 安装 → 验证）
- ✅ 用户确认弹窗正常工作

---

#### Day 6-7: TUI 工作流可视化 [P0]

**目标**: 在 TUI 中实时展示 Agent 的推理步骤和工具调用链

**任务清单**:

1. **设计工作流卡片组件** (0.5 天)
   - 文件: `pkg/tui/model/workflow.go`
   - 功能: 展示 Agent 的推理过程
   ```
   ┌─ 工作流执行中 ────────────────────────────┐
   │ ✓ 步骤 1: 意图分类 → operation           │
   │ ⟳ 步骤 2: 规划阶段                        │
   │   ├─ 调用工具: list_namespaces           │
   │   ├─ 调用工具: helm_search               │
   │   └─ 生成计划: 3 个操作步骤               │
   │ ○ 步骤 3: 等待用户确认                    │
   │ ○ 步骤 4: 执行阶段                        │
   └──────────────────────────────────────────┘
   ```

2. **扩展事件系统** (0.5 天)
   - 文件: `pkg/tui/events/events.go`
   - 新增事件类型:
   ```go
   type WorkflowStepEvent struct {
       QueryID     string
       StepIndex   int
       StepName    string
       Status      string // "running", "completed", "failed"
       Detail      string
       ToolCalls   []string
   }
   ```

3. **Agent 发送工作流事件** (0.5 天)
   - 修改 `pkg/agent/operation/agent.go`
   - 在关键节点发送 `WorkflowStepEvent`:
     - 意图分类完成
     - 开始规划阶段
     - 每次工具调用
     - 生成计划完成
     - 等待用户确认
     - 开始执行
     - 每个步骤执行完成

4. **TUI 渲染工作流卡片** (0.5 天)
   - 修改 `pkg/tui/model/chat.go`
   - 在消息列表中插入工作流卡片
   - 实时更新卡片状态（spinner 动画）

**验收标准**:
- ✅ TUI 中能看到 Agent 的推理过程
- ✅ 工具调用链清晰可见
- ✅ 状态更新实时（spinner 动画流畅）

---

### 第 2 周：Watch 监听 + 文档 + 视频

#### Day 8-10: Watch API 主动监听 [P1]

**目标**: 实现 K8s 事件监听，主动发现异常 Pod

**任务清单**:

1. **实现 Watch 客户端** (1 天)
   - 文件: `pkg/k8s/watcher.go`
   ```go
   type Watcher struct {
       client    *Client
       stopCh    chan struct{}
       eventCh   chan WatchEvent
   }
   
   type WatchEvent struct {
       Type      string // "ADDED", "MODIFIED", "DELETED"
       Resource  string // "Pod", "Deployment", etc.
       Namespace string
       Name      string
       Reason    string // "CrashLoopBackOff", "OOMKilled", etc.
       Message   string
   }
   
   func (w *Watcher) WatchPods(namespace string) error
   func (w *Watcher) WatchEvents(namespace string) error
   func (w *Watcher) Stop()
   ```

2. **实现异常检测逻辑** (0.5 天)
   - 文件: `pkg/k8s/detector.go`
   - 检测规则:
     - Pod 状态: `CrashLoopBackOff`, `OOMKilled`, `ImagePullBackOff`, `Error`
     - Event 类型: `Warning` 且 Reason 包含关键词
   ```go
   func DetectAnomalies(event WatchEvent) (bool, string)
   ```

3. **集成到 TUI** (1 天)
   - 文件: `pkg/tui/app.go`
   - 新增"监控模式"子命令: `kubewise tui --watch`
   - 在 TUI 侧边栏增加"监控面板"
   ```
   ┌─ 监控面板 ────────────────┐
   │ 🟢 集群状态: 正常          │
   │ 📊 监控中: 3 个命名空间     │
   │                           │
   │ ⚠️  最近告警:              │
   │ • default/nginx-xxx       │
   │   CrashLoopBackOff        │
   │   2 分钟前                 │
   │                           │
   │ [点击查看详情]             │
   └───────────────────────────┘
   ```

4. **实现告警通知** (0.5 天)
   - 检测到异常时，在 TUI 中弹出通知
   - 用户可点击"自动分析"按钮，触发 Troubleshooting Agent

**验收标准**:
- ✅ `kubewise tui --watch` 能启动监控模式
- ✅ 手动创建一个崩溃的 Pod，TUI 能检测到并告警
- ✅ 点击"自动分析"能触发故障排查

---

#### Day 11-12: 完善设计文档 [P0]

**目标**: 补充技术文档，满足选拔赛"设计 30%"要求

**任务清单**:

1. **更新架构设计文档** (0.5 天)
   - 文件: `docs/architecture.md`
   - 内容:
     - 系统总体架构图（Mermaid）
     - 4 个 Agent 的详细设计
     - 工具注册中心机制
     - TUI 事件驱动架构
     - Helm 集成方案
     - Watch 监听机制

2. **编写技术创新点说明** (0.5 天)
   - 文件: `docs/innovation.md`
   - 内容:
     - 多 Agent 协作架构（vs 单一 Agent）
     - 有状态推理（规划 → 确认 → 执行）
     - 工作流可视化（vs 黑盒 LLM）
     - 主动监控（vs 被动响应）
     - 模型无关设计（vs 绑定 OpenAI）

3. **补充用户手册** (0.5 天)
   - 文件: `docs/user-guide.md`
   - 内容:
     - 快速开始
     - 配置说明
     - 使用场景示例
     - 常见问题 FAQ

4. **更新 README** (0.5 天)
   - 突出核心功能
   - 补充 Helm 集成说明
   - 补充 Watch 监听说明
   - 添加演示视频链接（占位）

**验收标准**:
- ✅ 文档结构完整，逻辑清晰
- ✅ 架构图美观，技术路线可行
- ✅ 创新点说明有说服力

---

#### Day 13-14: 录制演示视频 + 部署在线环境 [P0]

**目标**: 制作高质量演示视频，部署在线演示环境

**任务清单**:

1. **准备演示环境** (0.5 天)
   - 使用 kind 或 minikube 创建本地 K8s 集群
   - 预先部署一些资源（Deployment、Service、PVC 等）
   - 准备演示脚本

2. **录制演示视频** (1 天)
   - 工具: OBS Studio / Asciinema
   - 时长: 5-8 分钟
   - 内容结构:
     ```
     00:00 - 00:30  项目介绍（字幕 + 旁白）
     00:30 - 02:00  场景 1: 一句话部署 ArgoCD（展示工作流可视化）
     02:00 - 03:30  场景 2: 智能查询（跨资源联查 PV/PVC/Pod）
     03:30 - 05:00  场景 3: 故障排查（自动分析崩溃 Pod）
     05:00 - 06:30  场景 4: 安全审计（RBAC 权限扫描）
     06:30 - 07:30  场景 5: Watch 监听（实时发现异常）
     07:30 - 08:00  总结 + 技术亮点
     ```
   - 注意事项:
     - 画面清晰，字体大（终端字号 18+）
     - 操作流畅，避免卡顿
     - 配字幕和旁白
     - 突出工作流可视化效果

3. **部署在线演示环境** (0.5 天)
   - 方案 1: 使用 GitHub Codespaces（免费，但需要 GitHub 账号）
   - 方案 2: 使用 Killercoda（免费 K8s 环境）
   - 方案 3: 自建服务器 + ttyd（Web 终端）
   - 提供访问链接和使用说明

**验收标准**:
- ✅ 视频时长 5-8 分钟，画面清晰
- ✅ 5 个场景全部演示成功
- ✅ 在线演示环境可访问

---

## 四、风险控制

### 4.1 技术风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| Helm SDK 集成复杂度超预期 | 中 | 高 | 提前调研，参考 Helm 官方示例，预留 1 天 buffer |
| Watch API 性能问题 | 低 | 中 | 限制监听范围（只监听特定命名空间），增加过滤条件 |
| TUI 工作流可视化实现困难 | 中 | 中 | 简化设计，先实现基本功能，后续优化 |
| 视频录制质量不佳 | 低 | 高 | 提前准备脚本，多次彩排，必要时重录 |

### 4.2 时间风险

| 里程碑 | 截止日期 | 关键路径 | 应急方案 |
|--------|---------|---------|---------|
| Helm 集成完成 | Day 3 | 是 | 如延期，缩减演示场景数量（3 → 2） |
| 工作流可视化完成 | Day 7 | 是 | 如延期，简化 UI 设计，只展示关键步骤 |
| Watch 监听完成 | Day 10 | 否 | 如延期，可砍掉此功能，不影响核心演示 |
| 视频录制完成 | Day 14 | 是 | 预留 2 天 buffer，必要时加班完成 |

### 4.3 质量保证

- **每日自测**: 每天下班前运行完整测试套件
- **代码审查**: 关键模块（Helm 集成、Watch 监听）完成后进行 self-review
- **演示彩排**: Day 12 进行完整演示彩排，发现问题及时修复

---

## 五、成功标准

### 5.1 功能完成度

- ✅ Helm 集成: 3 个工具（install/upgrade/uninstall）全部可用
- ✅ 一句话部署: 3 个演示场景（ArgoCD/Prometheus/Nginx Ingress）全部成功
- ✅ 工作流可视化: TUI 中能实时展示 Agent 推理过程
- ✅ Watch 监听: 能检测到 CrashLoopBackOff/OOMKilled 等异常
- ✅ 文档完善: 架构设计、技术创新点、用户手册全部完成
- ✅ 演示视频: 5-8 分钟，5 个场景，画面清晰

### 5.2 评分预期

| 维度 | 权重 | 预期得分 | 理由 |
|------|------|---------|------|
| 创意 | 20% | 16-18 分 | 多 Agent 协作 + 主动监控，有一定创新性 |
| 技术 | 30% | 24-27 分 | Helm 集成 + 工作流可视化 + Watch 监听，技术完整 |
| 应用 | 20% | 16-18 分 | 真实运维场景，有产业化潜力 |
| 设计 | 30% | 25-28 分 | 架构清晰 + TUI 友好 + 视频演示效果好 |
| **总分** | **100%** | **81-91 分** | **预期进入挑战赛** |

---

## 六、后续优化方向（挑战赛阶段）

如果进入挑战赛（现场展示 + 答辩），可考虑以下增强：

1. **ADK-Go 重构** [技术深度]
   - 将 Operation Agent 重构为真正的有状态执行图
   - 支持条件分支、并行执行、错误回退

2. **MCP Server 支持** [生态集成]
   - 实现 MCP Server 协议
   - 与 Claude Desktop / Cursor 集成

3. **Web UI** [用户体验]
   - React + TailwindCSS 实现 Web 界面
   - 支持多用户、权限管理

4. **多集群支持** [企业级功能]
   - 支持管理多个 K8s 集群
   - 集群切换、资源聚合查询

5. **自动修复** [智能化]
   - Watch 监听 + 自动修复决策 + 执行
   - 形成完整的自动化运维闭环

---

## 七、总结

### 核心策略

**2 周时间，聚焦 2 个核心方向**：
1. **Helm 集成 + 工作流可视化**（兑现文档承诺，强化设计 30%）
2. **Watch API 监听**（展示主动监控能力，强化应用 20%）

### 关键成功因素

1. **Helm 集成是必须项**：文档承诺的核心功能，不做会被质疑
2. **工作流可视化是加分项**：视频演示效果好，评委印象深刻
3. **Watch 监听是差异化**：竞品都没做，展示真正的自动化运维
4. **高质量视频是关键**：选拔赛是网络评审，视频质量决定第一印象

### 预期结果

- **功能完整度**: 80%+（核心功能全部实现）
- **演示效果**: 优秀（5 个场景，工作流可视化）
- **技术深度**: 良好（Helm + Watch + 多 Agent）
- **文档质量**: 优秀（架构清晰，创新点突出）
- **预期得分**: 81-91 分（进入挑战赛）

---

**下一步行动**: 开始 Day 1 任务 — 添加 Helm 依赖并创建客户端封装。
