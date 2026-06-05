// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/report"
	"github.com/kubewise/kubewise/pkg/agent/deploy/types"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

// DeployConfirmationHandler presents a deploy plan and waits for user decision.
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan deploytypes.DeployPlan) (deploytypes.DeployDecision, error)
}

// ChartSelectionHandler presents chart candidates for user selection.
type ChartSelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// Agent orchestrates the deploy pipeline.
type Agent struct {
	llmClient        *llm.Client
	helmClient       *helm.Client
	confirmHandler   DeployConfirmationHandler
	selectionHandler ChartSelectionHandler
	eventCh          chan<- stream.Event
	queryID          string
	log              *zap.Logger
	toolRegistry     *tool.Registry
	k8sClient        *k8s.Client
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

// SetSelectionHandler sets the chart selection handler.
func (a *Agent) SetSelectionHandler(h ChartSelectionHandler) {
	a.selectionHandler = h
}

// SetConfirmHandler sets the deploy confirmation handler.
func (a *Agent) SetConfirmHandler(h DeployConfirmationHandler) {
	a.confirmHandler = h
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

// WithEventChannel sets the stream event channel.
func WithEventChannel(ch chan<- stream.Event, queryID string) Option {
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

// HandleQuery runs the deploy pipeline.
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (result string, err error) {
	a.emit(stream.AgentStart{AgentName: "Deploy Agent", QueryID: a.queryID})
	startTime := time.Now()
	defer func() {
		a.logger().Info("deploy pipeline finished",
			zap.String("component", "deploy"),
			zap.String("query_id", a.queryID),
			zap.Duration("elapsed", time.Since(startTime)),
		)
		a.emit(stream.AgentDone{QueryID: a.queryID, Result: result, Duration: time.Since(startTime)})
	}()

	return a.runDeployPipeline(ctx, query, entities)
}

// ConfirmDeploy implements state.ConfirmHandler.
func (a *Agent) ConfirmDeploy(ctx context.Context, p deploytypes.DeployPlan) (deploytypes.DeployDecision, error) {
	if a.confirmHandler == nil {
		return deploytypes.DeployDecision{Action: "execute", Values: p.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, p)
}

// SelectChart implements state.SelectionHandler.
func (a *Agent) SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error) {
	if a.selectionHandler == nil {
		return nil, nil
	}
	return a.selectionHandler.SelectChart(ctx, appName, candidates)
}

func (a *Agent) buildReport(ctx context.Context, rel *helm.Release, chartInfo *catalog.ChartInfo, namespace, releaseName string) string {
	return report.SuccessMessage(ctx, rel, chartInfo, namespace, releaseName, a.k8sClient, a.logger())
}

func (a *Agent) emit(e stream.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// Emit sends a non-blocking stream event.
func (a *Agent) Emit(e stream.Event) {
	a.emit(e)
}

type recoveryLogger struct {
	agent *Agent
}

func (l *recoveryLogger) Info(msg string, fields ...zap.Field) {
	l.agent.logger().Info(msg, append([]zap.Field{zap.String("component", "deploy")}, fields...)...)
}
func (l *recoveryLogger) Debug(msg string, fields ...zap.Field) {
	l.agent.logger().Debug(msg, append([]zap.Field{zap.String("component", "deploy")}, fields...)...)
}
func (l *recoveryLogger) Warn(msg string, fields ...zap.Field) {
	l.agent.logger().Warn(msg, append([]zap.Field{zap.String("component", "deploy")}, fields...)...)
}
func (l *recoveryLogger) Error(msg string, fields ...zap.Field) {
	l.agent.logger().Error(msg, append([]zap.Field{zap.String("component", "deploy")}, fields...)...)
}
