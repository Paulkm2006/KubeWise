# Dashboard 集群聚焦与工作上下文分离设计

> 日期: 2026-06-11
> 状态: 设计稿 / 待实现
> 涉及组件: App.tsx, Dashboard.tsx, Header.tsx
> 改动范围: 纯前端，限于 `frontend/src/components/`

## 问题背景

Dashboard 中存在两层 cluster 选择语义不清晰的问题：

1. **Header 右上角集群切换器** — 切换后改变全局 `activeCluster`，影响整个应用（deploy、diagnose 等操作的目标）
2. **Dashboard 的集群卡片行** — 点击卡片后设置 `filterCluster`，影响 Dashboard 右侧面板显示的数据

两者互不耦合，用户会产生困惑：「右侧面板的状态到底听谁的？」「单击卡片应该只过滤数据，还是同时切换工作上下文？」

此外，当前「Filtered by: [X] ✕」筛选条件条使用了**多选 tag** 的视觉模式，暗示可以叠加筛选条件，但实际只支持单选，存在视觉误导。

## 设计目标

1. 明确区分**数据聚焦（data lens）**与**工作上下文（working context）**两个概念
2. Dashboard 右侧面板永远听数据聚焦的
3. 单击/双击自然区分两种操作，互不冲突
4. 零文本标注，纯视觉语义
5. 删除有误导的筛选条件条
6. 回到 All 的操作不需要额外 UI 元素

## 核心概念

### 两个独立维度

| 状态 | 命名 | 含义 | 驱动方式 | 视觉标注 | 影响范围 |
|------|------|------|---------|---------|---------|
| 工作上下文 | `activeCluster` | 操作（deploy、diagnose）的目标集群 | Header 下拉选择、**双击**卡片 | 卡片名字左侧 ◆ 符号（`text-accent`） | 全局 |
| 数据聚焦 | `focusCluster` | Dashboard 当前显示哪个集群的数据 | **单击**卡片、单击已选中卡取消 | 实线边框（`border-l-[3px] border-accent`） | Dashboard 右侧面板 |

`activeCluster` **永远有值**（默认加载第一个集群），不存在「未选中」状态。

`focusCluster` 可以为 `null`，表示 Dashboard 处于「全部集群视图」模式。

### 视觉区分 — 三种卡片状态

```
┌──────────────────────┐
│ ◆ prod-us        ●  │  ← ◆ = activeCluster，● = 健康状态
│ █ 12/12 pods ready   │
│ 3 issues             │
│ ◈ 8 nodes ▣ 28 ns    │
└──────────────────────┘
```

| 状态 | 边框 | ◆ 所在 | 含义 |
|------|------|--------|------|
| **全部视图** | 全部灰边 `border border-border` | ◆ 在其中一张卡上 | 看全部集群的数据，操作上下文在 ◆ 那个集群 |
| **聚焦视图** | 聚焦卡片实线边框，其他灰边 | ◆ 可能在聚焦卡片上，也可能在另一张 | 聚焦看一个集群的数据，操作上下文可能不同 |
| **聚焦 + 工作上下文对齐** | 实线边框 | ◆ 在实线卡片上 | 数据聚焦和操作上下文指向同一集群 |

## 交互规则

| 操作 | `focusCluster` | `activeCluster` | 视觉变化 |
|------|:-------------:|:---------------:|---------|
| 单击未选中卡 A | `A` | 不变 | A 出现实线边框 |
| 单击已选中卡 A | `null`（回 All） | 不变 | A 边框消失 |
| 双击卡 A | `A` | `A` | A 实线边框 + ◆ 移到 A |
| Header 下拉选 B | 不变 | `B` | ◆ 移到 B |

### 双击防抖

双击会触发两次 `onClick`，需要在第一次单击时做识别：

```
onClick → 启动 200ms 延迟
         ├─ 200ms 内又来一次 onClick → 识别为双击 → 执行双击逻辑
         └─ 200ms 后无第二次点击 → 执行单击逻辑

使用 useRef flag + setTimeout 实现
```

## 删除项

**删除**「Filtered by: [X] ✕」筛选条件条（第 190-203 行）。理由：
- 使用多选 tag 的视觉模式暗示可叠加筛选，与「不支持多选」的实际行为矛盾
- 回到 All 的方式改为「再单击一次已选中卡片」即可，不需要 ✕ 按钮

## Dashboard 右侧面板渲染规则

| `focusCluster` | 右侧 Stats 面板 | 右侧 Cluster Info 面板 |
|:--------------:|----------------|----------------------|
| `null`（All） | **跨集群聚合统计** — 总 pods ready/total、总 issues 数、集群数量、健康度分布摘要 | 隐藏，或显示简短汇总文字 |
| `"prod-us"`   | **该集群的详细 stats** — pods ready/total、issues count、nodes、namespaces（同当前行为） | **该集群详细信息** — version、fingerprint（同当前行为） |

### 聚合统计的初始方案

当 `focusCluster === null` 时，右侧统计卡片改为：

```
┌─────────────────────┐
│  4 connected         │  ← 集群数量 + 状态摘要
│  3 healthy · 1 degraded
└─────────────────────┘
┌──────┬──────┐
│144/156│  12  │
│ Pods  │Issues│
├──────┼──────┤
│  32   │  84  │
│ Nodes │  NS  │
└──────┴──────┘
```

统计卡片复用现有的 grid 布局，数据从 `clusters` 数组聚合计算。

## 涉及改动

### App.tsx

- 新增 `focusCluster: string | null` 状态，默认 `null`
- 传给 `<Dashboard>` 作为 prop
- `handleClusterChange`（Header 切换用）行为不变

### Dashboard.tsx

- **新增 props**：`activeCluster`, `focusCluster`, `onClusterChange`, `onFocusChange`
- **单击逻辑**：`handleClusterClick` 改为 toggle（已选中 → null，未选中 → 该集群）
- **双击逻辑**：新增 `handleDoubleClick`，同时设置 `focusCluster` + 调用 `onClusterChange`
- **防抖**：使用 `useRef` + `setTimeout` 抑制双击的首次单击
- **删除**：filter bar 区域（第 190-203 行）
- **卡片渲染**：根据 `focusCluster` 和 `activeCluster` 决定实线边框和 ◆ 符号
- **右侧面板**：`focusCluster === null` 时渲染聚合统计

### Header.tsx

- **不需要改动**。Header 绑定的是 `activeCluster`，不受 `focusCluster` 影响

## 组件属性接口

```tsx
// Dashboard 新接口
interface DashboardProps {
  activeCluster: string;
  focusCluster: string | null;
  onClusterChange: (name: string) => void;   // 切换 activeCluster
  onFocusChange: (name: string | null) => void;  // 切换 focusCluster
}
```

## 交互流程示例

### 场景一：日常巡检

1. 进入 Dashboard，无聚焦 → 右侧显示全集群聚合统计
2. 发现某集群 issue 较多，单击该集群卡 → 实线边框，右侧切换为该集群详情
3. 看完点一下已选中卡 → 取消聚焦，回到全集群聚合视图

### 场景二：发现后操作

1. 在全部视图中发现 prod-us 有异常
2. 单击 prod-us → 聚焦查看详情，确认 3 个 issue
3. 双击 prod-us → ◆ 标记跟随过来，Header 同步切换
4. 发起诊断 → 诊断目标自动是 prod-us（`activeCluster`）

### 场景三：对比数据 + 切换目标

1. 当前工作上下文为 prod-us（◆ 在 prod-us）
2. 单击 staging → 聚焦 staging，看其数据详情，但 Header 仍显示 prod-us
3. 看完后双击 staging → ◆ 移到 staging，Header 同步切换
4. 所有后续操作目标变为 staging

## 未实现/后续考虑

- **多选集群子集筛选** — 经过讨论，确认 All + 单集群聚焦已满足需求，无需多选。如未来需要对比视图，建议新建独立页面而非在 Dashboard 上加多选
- **聚合统计的具体内容** — 初始版本使用简单统计数据，后续可以增加健康分布图、趋势线等
- **键盘导航** — 方向键在卡片间移动、Enter 聚焦、Ctrl+Enter 切换上下文（Phase 2）
