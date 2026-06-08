package state

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/plan"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/values"
	deploytypes "github.com/kubewise/kubewise/internal/agent/subagent/deploy/types"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/catalog"
	"github.com/kubewise/kubewise/internal/utils/helm"
	"github.com/kubewise/kubewise/internal/utils/k8s"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/kubewise/kubewise/internal/agent/router/types"
		"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/agent/tool"
)

const (
	DefaultMaxCorrectionAttempts = 20
	DefaultMaxRecoveryAttempts   = 2
)

// ConfirmHandler presents a deploy plan and waits for user decision.
type ConfirmHandler interface {
	ConfirmDeploy(ctx context.Context, plan deploytypes.DeployPlan) (deploytypes.DeployDecision, error)
}

// SelectionHandler presents chart candidates for user selection.
type SelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// State holds runtime context and artifacts for one deploy HandleQuery run.
type State struct {
	// --- runtime ---
	Ctx context.Context

	// --- dependencies ---
	LLM         *llm.Client
	Helm        *helm.Client
	K8s         *k8s.Client
	Tools       *tool.Registry
	Confirm     ConfirmHandler
	Select      SelectionHandler
	BuildReport func(ctx context.Context, rel *helm.Release, chart *catalog.ChartInfo, namespace, releaseName string) string

	// --- events / logging ---
	QueryID string
	EventCh chan<- event.Event
	Log     *zap.Logger

	// --- state machine ---
	Phase Phase

	// --- input ---
	Query    string
	Entities types.Entities

	// --- chart / values artifacts ---
	AppName           string
	ReleaseName       string
	Chart             *catalog.ChartInfo
	DefaultValues     string
	GenResult         *values.Result
	Plan              plan.DeployPlan
	FinalValues       string
	CorrectionHistory []string
	Release           *helm.Release
	DeployErr         error
	RecoveryErr       error

	// --- retry counters ---
	CorrectionAttempts    int
	MaxCorrectionAttempts int
	RecoveryAttempts      int
	MaxRecoveryAttempts   int
	RecoveryPendingReview bool
	RecoveryMessages      []llm.Message

	// --- outcome ---
	Result string
	Err    error
}

// New builds initial pipeline state for a deploy run.
func New(ctx context.Context, query string, entities types.Entities, deps Deps) *State {
	log := deps.Log
	if log == nil {
		log = zap.NewNop()
	}
	maxCorr := deps.MaxCorrectionAttempts
	if maxCorr <= 0 {
		maxCorr = DefaultMaxCorrectionAttempts
	}
	maxRec := deps.MaxRecoveryAttempts
	if maxRec <= 0 {
		maxRec = DefaultMaxRecoveryAttempts
	}
	st := &State{
		Ctx:                   ctx,
		LLM:                   deps.LLM,
		Helm:                  deps.Helm,
		K8s:                   deps.K8s,
		Tools:                 deps.Tools,
		Confirm:               deps.Confirm,
		Select:                deps.Select,
		BuildReport:           deps.BuildReport,
		QueryID:               deps.QueryID,
		EventCh:               deps.EventCh,
		Log:                   log,
		Phase:                 PhaseExtractApp,
		Query:                 query,
		Entities:              entities,
		MaxCorrectionAttempts: maxCorr,
		MaxRecoveryAttempts:   maxRec,
	}
	return st
}

// Deps are injected services for a deploy run.
type Deps struct {
	LLM                   *llm.Client
	Helm                  *helm.Client
	K8s                   *k8s.Client
	Tools                 *tool.Registry
	Confirm               ConfirmHandler
	Select                SelectionHandler
	BuildReport           func(ctx context.Context, rel *helm.Release, chart *catalog.ChartInfo, namespace, releaseName string) string
	QueryID               string
	EventCh               chan<- event.Event
	Log                   *zap.Logger
	MaxCorrectionAttempts int
	MaxRecoveryAttempts   int
}

// Emit sends a non-blocking stream event.
func (s *State) Emit(e event.Event) {
	if s.EventCh == nil {
		return
	}
	select {
	case s.EventCh <- e:
	default:
	}
}

// EmitCritical sends a blocking stream event.
func (s *State) EmitCritical(e event.Event) {
	if s.EventCh == nil {
		return
	}
	s.EventCh <- e
}

// RunTool executes fn while emitting deploy tool progress events.
func (s *State) RunTool(ctx context.Context, name string, step int, fn func(context.Context) error) error {
	s.Emit(event.ToolCall{QueryID: s.QueryID, ToolName: name, Step: step})
	start := time.Now()
	err := fn(ctx)
	elapsed := time.Since(start)
	if err != nil {
		s.Emit(event.ToolFail{
			QueryID: s.QueryID, ToolName: name, Step: step, Elapsed: elapsed, Err: err.Error(),
		})
		return err
	}
	s.Emit(event.ToolDone{
		QueryID: s.QueryID, ToolName: name, Step: step, Elapsed: elapsed,
	})
	return nil
}

// RunToolWithResult executes fn and returns its result with deploy tool events.
func RunToolWithResult[T any](s *State, ctx context.Context, name string, step int, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	s.Emit(event.ToolCall{QueryID: s.QueryID, ToolName: name, Step: step})
	start := time.Now()
	out, err := fn(ctx)
	elapsed := time.Since(start)
	if err != nil {
		s.Emit(event.ToolFail{
			QueryID: s.QueryID, ToolName: name, Step: step, Elapsed: elapsed, Err: err.Error(),
		})
		return zero, err
	}
	s.Emit(event.ToolDone{
		QueryID: s.QueryID, ToolName: name, Step: step, Elapsed: elapsed,
	})
	return out, nil
}

func (s *State) baseFields() []zap.Field {
	fields := []zap.Field{zap.String("component", "deploy"), zap.String("phase", s.Phase.String())}
	if s.QueryID != "" {
		fields = append(fields, zap.String("query_id", s.QueryID))
	}
	return fields
}

func (s *State) LogDebug(msg string, fields ...zap.Field) {
	s.Log.Debug(msg, append(s.baseFields(), fields...)...)
}

func (s *State) LogInfo(msg string, fields ...zap.Field) {
	s.Log.Info(msg, append(s.baseFields(), fields...)...)
}

func (s *State) LogWarn(msg string, fields ...zap.Field) {
	s.Log.Warn(msg, append(s.baseFields(), fields...)...)
}

func (s *State) LogError(msg string, fields ...zap.Field) {
	s.Log.Error(msg, append(s.baseFields(), fields...)...)
}

// Next sets the next pipeline phase.
func (s *State) Next(phase Phase) {
	s.Phase = phase
}

// Done marks successful or user-cancelled completion with a result message.
func (s *State) Done(result string) {
	s.Result = result
	s.Phase = PhaseDone
}

// Fail marks pipeline failure.
func (s *State) Fail(err error) {
	s.Err = err
	s.Phase = PhaseFailed
}

// CountLines returns the number of lines in a YAML string.
func CountLines(yaml string) int {
	if yaml == "" {
		return 0
	}
	return len(strings.Split(yaml, "\n"))
}
