// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	routertypes "github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/core/catalog"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/core/report"
	deploytypes "github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/types"
	toolquery "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/query"
	tooltroubleshooting "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/troubleshooting"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/helm"
	"github.com/kubewise/kubewise/internal/utils/llm"
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
	eventCh          chan<- event.Event
	queryID          string
	log              *zap.Logger
	toolRegistry     *toolv2.Registry
	k8sClient        *cluster.Client
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- event.Event, queryID string) {
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

func (a *Agent) logger() *zap.Logger {
	return config.L()
}

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
func WithEventChannel(ch chan<- event.Event, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// New creates a Deploy Agent.
func New(llmClient *llm.Client, helmClient *helm.Client, k8sClient *cluster.Client, opts ...Option) *Agent {
	registry := toolv2.NewRegistry()
	_ = toolquery.RegisterQueryTools(registry, k8sClient)
	_ = tooltroubleshooting.RegisterTools(registry, k8sClient)

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
func (a *Agent) HandleQuery(ctx context.Context, query string, entities routertypes.Entities) (result string, err error) {
	a.emit(event.AgentStart{AgentName: "Deploy Agent", QueryID: a.queryID})
	startTime := time.Now()
	defer func() {
		a.logger().Info("deploy pipeline finished",
			zap.String("component", "deploy"),
			zap.String("query_id", a.queryID),
			zap.Duration("elapsed", time.Since(startTime)),
		)
		a.emit(event.AgentDone{QueryID: a.queryID, Result: result, Duration: time.Since(startTime)})
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

func (a *Agent) emit(e event.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// Emit sends a non-blocking stream event.
func (a *Agent) Emit(e event.Event) {
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
