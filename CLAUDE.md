# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o kubewise ./cmd        # build binary
go test ./...                     # run all tests
go test -run TestX ./pkg/...      # run a single test
go vet ./...                      # vet
```

No Makefile — all operations use `go` directly.

## Architecture

KubeWise is a multi-agent Kubernetes operations system with a TUI. The flow is:

```
User input (CLI or TUI) → Router Agent → Sub-agent → Tool Registry → K8s (client-go)
```

**Router Agent** (`pkg/agent/router/agent.go`) uses LLM to classify user intent into one of 5 task types: query, operation, troubleshooting, security, deploy. It then routes to the corresponding sub-agent. For TUI mode, `HandleQueryStream` creates **fresh sub-agent instances** on each request (so event channels don't bleed across queries), and uses bridge goroutines to convert synchronous confirm/select handlers into TUI event channel messages.

**5 Sub-agents**, each in its own package under `pkg/agent/`:
- `query` — ReAct loop (up to 10 tool call rounds) for cross-resource queries
- `operation` — LLM plan → user confirm → execute, with natural-language correction loop
- `troubleshooting` — systematic info gathering → root cause → fix suggestions
- `security` — 4-dimension audit (RBAC, Pod security, network policies, image security)
- `deploy` — 7-phase Helm deployment pipeline (see below)

**Tools** follow a global registration pattern: each tool file calls `tool.RegisterGlobal()` in `init()`, providing a `ToolMetadata` with a factory function. Agents load only the tools they need via `tool.LoadGlobalRegistryByCategory(dep, category)`. Tool categories: `""` (read/query), `"operation"` (write).

**TUI** (`pkg/tui/`) is bubbletea-based. The `App` model composes sidebar, chat, input, confirm, and deploy sub-models. Agents communicate with the TUI exclusively via `events.TUIEvent` sealed interface sent through a buffered channel. Key sub-models: `ChatModel` (messages + progress cards), `ConfirmModel` (operation approval), `DeployConfirmModel` (deploy plan review), `ChartSelectModel` (ArtifactHub picker).

**Sessions** are persisted as JSON to `~/.kubewise/sessions/` via `pkg/tui/session/store.go`.

## Deploy Pipeline (7 Phases)

The deploy agent (`pkg/agent/deploy/`) implements a 7-phase pipeline, emitting `PhaseEvent` / `ToolCallEvent` / `ToolDoneEvent` at each step for the TUI progress card:

1. Extract app name from entities/query
2. Resolve Chart via `ChainResolver` (builtin → local catalog)
3. `helm repo add` + `helm show values` to get defaults
4. LLM generates override values based on user intent
5. User confirmation (execute / natural-language correction loop / cancel)
6. `helm install/upgrade` via Helm v4 Go SDK
7. Verify release status and build report

If Phase 2 finds nothing, the agent falls through to ArtifactHub search + TUI selection.

## Chart Catalog Resolution

`pkg/catalog/` implements a **chain-of-responsibility** pattern:

1. `BuiltinCatalogResolver` — embedded `builtin_data.yaml` (alias → ChartInfo lookup)
2. `LocalCatalogResolver` — `~/.kubewise/catalog.yaml` (same format, user-extensible)
3. `ArtifactHubResolver` — invoked explicitly by the deploy agent when chain returns nil, delegates to TUI for interactive selection

`ChartResolver.Resolve()` returns `(nil, nil)` to signal "not found, try next" vs `(nil, err)` for hard errors.

## Key Dependencies

- **Helm v4** SDK (`helm.sh/helm/v4`) — not v3. Uses `pkg/action` for install/upgrade, `pkg/loader` for chart loading
- **client-go** v0.36 — both `kubernetes.Clientset` (typed) and `dynamic.Interface` (arbitrary GVR)
- **openai-go v3** — all LLM calls go through this, configured for OpenAI-compatible APIs (DeepSeek, GLM, Qwen, etc.)
- **bubbletea** — TUI framework with `tea.Model` / `tea.Cmd` / `tea.Msg` pattern

## Config & Secrets

- Config: Viper reads `~/.kubewise.yaml` (or `--config` flag), with env var override via `KUBEWISE_` prefix
- **Never commit** `.kube/config`, `config.yaml` with real API keys, or `.env` files — these are `.gitignore`'d
- Dev cluster: use kind as described in `docs/how-to-dev.md` — the `.kube/` directory contains dev cluster configs

## Go Module

Module path is `github.com/kubewise/kubewise`, Go 1.26. Internal packages under `pkg/` are not importable externally.
