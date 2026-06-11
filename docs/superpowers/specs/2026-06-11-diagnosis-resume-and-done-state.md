# Diagnosis 恢复历史 & Done 临时状态

## 决策记录

### 1. Dashboard 按钮外观

仅保留两种：

- **`✓ Done`** — 临时通知徽章，会话内有诊断完成时出现
- **`Diagnose →`** — 默认状态

去掉 `⟳ ...` 状态。按钮被点击后直接在 Overlay 内部展示 loading。

### 2. 点击按钮的统一流程

无论按钮外观是 Done 还是 Diagnose →，点进去逻辑统一：

1. 打开 Overlay，显示 loading
2. 调用后端 API 查询该 pod 的最新诊断（`GET /api/v1/diagnoses/latest?cluster=X&namespace=Y&pod=Z`）
3. **有旧的** → 展示旧诊断：
   - running → 接上 SSE 流继续接收
   - completed / failed / cancelled → 显示上次报告
   - 显示"上次诊断时间"及距今多久
   - 提供 **[重新跑]** 按钮
4. **无旧的** → POST 创建新诊断 → 接 SSE 流

### 3. 重新跑行为

点击 [重新跑] 时：

1. 如果旧的是 running → 先调用 cancel API 取消（停止 LLM 调用，释放资源），标记旧诊断为 cancelled
2. POST 创建新诊断 → 接新 SSE 流
3. Done 徽章消失

### 4. Done 徽章生命周期

- **创建**：诊断完成时（本会话内），Dashboard 该 pod 按钮变为 `✓ Done`
- **销毁-1**：用户点击 Done 按钮进去看报告 → 徽章消失
- **销毁-2**：页面刷新 → 所有 Done 徽章消失
- **销毁-3**：用户点击 [重新跑] → Done 徽章消失

**定位**：Done 是临时状态。只在用户没有退出网页期间有 diagnose 完成时才出现。点进去走"有旧的优先显示旧的"逻辑。

### 5. 后端 API 需求

| 端点 | 用途 |
|------|------|
| `GET /api/v1/diagnoses/latest?cluster=X&namespace=Y&pod=Z` | 按 target 查找最新诊断 |
| `POST /api/v1/diagnoses/:id/cancel` | 取消正在运行的诊断 |

### 6. 前端改动范围

| 文件 | 改动 |
|------|------|
| `stores/diagnosisStore.ts` | `findExisting()` 扩展为查后端 API + 支持所有状态；新增 `cancelAndRecreate()` |
| `App.tsx` | `diagnosedPods` 改为临时通知语义；`handleDiagnose` 改为先查历史再决定；新增重新跑逻辑 |
| `components/Dashboard.tsx` | 去掉 `⟳ ...` 状态和 `diagnosingPods`；Done 按钮点进即消 |
| `components/DiagnosisOverlay.tsx` | 新增"上次诊断时间"显示；新增 [重新跑] 按钮；支持 cancelled 状态展示 |
| `api/client.ts` | 新增 `diagnoses.latest()` 和 `diagnoses.cancel()` |
| `api/types.ts` | `DiagnosisSummary.status` 新增 `cancelled` |
