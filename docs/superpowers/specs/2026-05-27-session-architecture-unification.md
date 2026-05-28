# Session Architecture Unification

## Status

Design proposal for merging the two "Session" concepts in KubeWise into
a single cohesive package with clear boundaries.

## 1. Problem Summary

There are currently two unrelated types both called "Session" in different packages:

| Location | Type | What it holds |
|----------|------|---------------|
| `pkg/session.Session` | Dependency container | LLM client, K8s client, Helm client, Router |
| `pkg/tui/session.Session` | Conversation record | Messages, Title, Timestamps, Blocks |

Additionally, the conversation persistence layer (`Store`) is a concrete struct
with no interface, locked to file-based storage and coupled to the TUI package,
even though the API server also needs it.

## 2. Boundary: Session vs Conversation

**Session** is the runtime container — the "capability" layer:
- LLM client, K8s client, Helm client
- Router + all sub-agents
- Lifetime = app start to exit
- One per process (TUI has one Session, API server has one Session)

**Conversation** is the interaction record — the "history" layer:
- One Session can have N Conversations (TUI multi-tab)
- Messages + Blocks + title/timestamps
- Created by user action (new tab / new chat), not by app startup

**Relationship:** Session owns the dependencies that Conversations use to
produce messages. Conversations are persisted through a Store interface.

```
App
 ├─ Session (1)         ← dependency container, lives for app lifetime
 │    ├─ LLM *llm.Client
 │    ├─ K8s  *k8s.Client
 │    ├─ Helm *helm.Client
 │    └─ Router *router.Agent
 │
 ├─ Conversation (N)    ← one per tab, has messages
 │    └─ store.Save() / LoadRecent() / Delete()
 │
 └─ Store (1)           ← persistence interface, currently FileStore
```

## 3. Package Structure

```
pkg/session/
  ├── session.go       — Session struct (LLM, K8s, Helm, Router) + New() + Config
  ├── conversation.go  — Conversation (messages, title, timestamps, blocks)
  ├── block.go         — Block and all payload types (TablePayload, CodePayload, etc.)
  │                       (moved from pkg/tui/session/ with type names unchanged)
  └── store/
       ├── store.go    — Store interface (Save/LoadRecent/Delete)
       └── file.go     — FileStore implementation (moved from pkg/tui/session/store.go)
```

### 3.1 Session — The Dependency Container

Stays exactly as implemented. No changes beyond imports.

```go
// pkg/session/session.go (existing, unchanged)
type Session struct {
    LLM    *llm.Client
    K8s    *k8s.Client
    Helm   *helm.Client
    Router *router.Agent
}
```

### 3.2 Conversation — The Interaction Record

Renamed from `tui/session.Session` to avoid confusion with the dependency container.
Type fields and JSON tags preserved exactly for backward compatibility with
existing saved files on disk.

```go
// pkg/session/conversation.go
type Conversation struct {
    ID               string    `json:"id"`
    Title            string    `json:"title"`
    CreatedAt        time.Time `json:"created_at"`
    UpdatedAt        time.Time `json:"updated_at"`
    Messages         []Message `json:"messages"`
    InterruptedQuery string    `json:"interrupted_query,omitempty"`
}

type Message struct {
    Role      string    `json:"role"`
    Content   string    `json:"content"`
    Blocks    []Block   `json:"blocks,omitempty"`
    Timestamp time.Time `json:"timestamp"`
    InTokens  int       `json:"in_tokens,omitempty"`
    OutTokens int       `json:"out_tokens,omitempty"`
    DurationS float64   `json:"duration_s,omitempty"`
}

func NewConversation() *Conversation { ... }
func TitleFromFirstMessage(content string) string { ... }
```

All block payload types (`Block`, `TablePayload`, `CodePayload`, `KVPayload`,
`KVPair`, `ListPayload`, `ListItem`, `DetailPayload`, `ContainerInfo`,
`ConditionInfo`, `EventInfo`) move to `pkg/session/block.go` or stay alongside
`Conversation` in `conversation.go` — whichever keeps the file focused.

### 3.3 Store Interface + FileStore

```go
// pkg/session/store/store.go
type Store interface {
    Save(conv *session.Conversation) error
    LoadRecent(n int) ([]*session.Conversation, error)
    Delete(id string) error
}
```

```go
// pkg/session/store/file.go
type FileStore struct {
    Dir string
}

func NewFileStore() (*FileStore, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return nil, fmt.Errorf("get home dir: %w", err)
    }
    dir := filepath.Join(home, ".kubewise", "sessions")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, fmt.Errorf("create sessions dir: %w", err)
    }
    return &FileStore{Dir: dir}, nil
}
```

`FileStore` implements `Store`. Constructor stays same as current `NewStore()`.

## 4. Import Changes

| File | Old Import | New Import |
|------|------------|------------|
| `pkg/tui/app.go` | `pkg/tui/session` | `pkg/session` + `pkg/session/store` |
| `pkg/api/handler.go` | `pkg/tui/session` | `pkg/session` + `pkg/session/store` |
| `pkg/tui/model/chat.go` | `pkg/tui/session` | `pkg/session` |
| `pkg/tui/model/sidebar.go` | `pkg/tui/session` | `pkg/session` |
| `pkg/api/handler_session.go` | `pkg/tui/session` | `pkg/session` + `pkg/session/store` |
| `pkg/tui/session/store_test.go` | `pkg/tui/session` | `pkg/session` + `pkg/session/store` |
| `pkg/api/server_test.go` | `pkg/tui/session` | `pkg/session` + `pkg/session/store` |

Type/function renames:

- `session.Session` (TUI) → `session.Conversation`
- `session.New()` → `session.NewConversation()`
- `session.NewStore()` → `store.NewFileStore()` (returns `*store.FileStore`, satisfies `store.Store`)

## 5. Migration Order

| Step | Description |
|------|-------------|
| 1 | Create `pkg/session/conversation.go` (move + rename `tui/session.Session` → `Conversation`) |
| 2 | Create `pkg/session/store/store.go` + `store/file.go` (move + interface-ify `tui/session.Store`) |
| 3 | Update all 7 importing files with new imports and type/function renames |
| 4 | Delete `pkg/tui/session/` directory |
| 5 | Commit |

## 6. Backward Compatibility

JSON serialization for `Conversation` uses the same field names and tags as the
old `tui/session.Session`. Saved files on disk remain loadable without migration.