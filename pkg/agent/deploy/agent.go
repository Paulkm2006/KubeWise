// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/recovery"
	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	wfpkg "github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"

	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

// helmClient is the minimal helm.Client interface for deploy and tests.
type helmClient interface {
	helmtools.Client
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
	ListReleases(ctx context.Context) ([]helm.Release, error)
}

// llmClient is the minimal llm.Client interface for tests.
type llmClient interface {
	values.LLMClient
	recovery.LLMClient
}

// DeployConfirmationHandler presents a deploy plan and waits for user decision.
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error)
}

// ChartSelectionHandler presents chart candidates for user selection.
type ChartSelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// Agent orchestrates the deploy pipeline.
type Agent struct {
	llmClient        llmClient
	helmClient       helmClient
	confirmHandler   DeployConfirmationHandler
	selectionHandler ChartSelectionHandler
	eventCh          chan<- events.TUIEvent
	queryID          string
	log              *zap.Logger
	toolRegistry     *tool.Registry
	k8sClient        *k8s.Client
}

// SetLogger injects a logger for debug output.
func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// Option configures the Agent.
type Option func(*Agent)

// WithConfirmHandler sets a custom confirmation handler.
func WithConfirmHandler(h DeployConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// WithSelectionHandler sets a custom chart selection handler.
func WithSelectionHandler(h ChartSelectionHandler) Option {
	return func(a *Agent) { a.selectionHandler = h }
}

// WithEventChannel sets the TUI event channel.
func WithEventChannel(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// New creates a Deploy Agent.
func New(llmClient *llm.Client, helmClient *helm.Client, k8sClient *k8s.Client, opts ...Option) *Agent {
	toolDep := tool.ToolDependency{K8sClient: k8sClient}
	registry, err := tool.LoadGlobalRegistryByCategory(toolDep, "")
	if err != nil {
		registry, _ = tool.LoadGlobalRegistryByCategory(tool.ToolDependency{}, "none")
	}

	a := &Agent{
		llmClient:    llmClient,
		helmClient:   helmClient,
		toolRegistry: registry,
		k8sClient:    k8sClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *Agent) workflowRunner() *wfpkg.Runner {
	return &wfpkg.Runner{QueryID: a.queryID, Emit: a}
}

// HandleQuery runs the deploy pipeline: resolve → values → validate → review → preflight → apply → verify.
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
	a.emit(events.AgentStartEvent{AgentName: "Deploy Agent", QueryID: a.queryID})
	startTime := time.Now()
	defer func() {
		a.logInfo("deploy pipeline finished", zap.Duration("elapsed", time.Since(startTime)))
		a.emit(events.AgentDoneEvent{QueryID: a.queryID, Duration: time.Since(startTime)})
	}()

	return a.runDeployPipeline(ctx, query, entities)
}

func (a *Agent) runRecovery(
	ctx context.Context,
	deployErr error,
	query string,
	correctionHistory []string,
	chart *catalog.ChartInfo,
	defaultValues, finalValues, namespace, appName string,
) (string, error) {
	a.logInfo("entering recovery loop", zap.String("app", appName))
	runner := &recovery.Runner{
		QueryID:      a.queryID,
		LLM:          a.llmClient,
		Helm:         a.helmClient,
		Tools:        a.toolRegistry,
		Workflow:     a.workflowRunner(),
		K8s:          a.k8sClient,
		Confirm:      a.confirmDeploy,
		BuildReport:  a.buildReport,
		EmitPhase:    a.emit,
		EmitCritical: a.emitCritical,
		Log:          &recoveryLogger{agent: a},
	}
	result, recErr := runner.Run(ctx, recovery.RunInput{
		DeployErr:         deployErr,
		Query:             query,
		CorrectionHistory: correctionHistory,
		Chart:             chart,
		DefaultValues:     defaultValues,
		CurrentValues:     finalValues,
		TargetNS:          namespace,
		AppName:           appName,
	})
	if recErr != nil {
		a.logError("recovery loop error", zap.Error(recErr))
		return "", fmt.Errorf("诊断修复过程出错: %w", recErr)
	}
	a.logInfo("recovery loop finished",
		zap.Int("action", int(result.Action)),
		zap.String("reason", result.Reason),
	)
	return result.Details, nil
}

func (a *Agent) confirmDeploy(ctx context.Context, p events.DeployPlan) (events.DeployDecision, error) {
	if a.confirmHandler == nil {
		return events.DeployDecision{Action: "execute", Values: p.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, p)
}

// Emit implements workflow.Emitter.
func (a *Agent) Emit(e events.TUIEvent) {
	a.emit(e)
}

func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

func (a *Agent) emitCritical(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	a.eventCh <- e
}

type recoveryLogger struct {
	agent *Agent
}

func (l *recoveryLogger) Info(msg string, fields ...zap.Field)  { l.agent.logInfo(msg, fields...) }
func (l *recoveryLogger) Debug(msg string, fields ...zap.Field) { l.agent.logDebug(msg, fields...) }
func (l *recoveryLogger) Warn(msg string, fields ...zap.Field)  { l.agent.logWarn(msg, fields...) }
func (l *recoveryLogger) Error(msg string, fields ...zap.Field) { l.agent.logError(msg, fields...) }
