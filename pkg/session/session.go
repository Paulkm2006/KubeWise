// Package session provides the dependency container for a KubeWise user session.
package session

import (
	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"go.uber.org/zap"
)

// Config holds all configuration needed to create a Session.
type Config struct {
	LLM           llm.Config
	KubeConfig    string
	MaxSteps      int
	SupervisorCfg supervisor.Config
}

// Session is the dependency container for one KubeWise instance.
// All shared resources are created once and injected into the component tree.
type Session struct {
	LLM    *llm.Client
	K8s    *k8s.Client
	Helm   *helm.Client
	Router *router.Agent
}

// New creates a Session, wiring all dependencies.
func New(cfg Config, log *zap.Logger) (*Session, error) {
	k8sClient, err := k8s.NewClient(cfg.KubeConfig)
	if err != nil {
		return nil, err
	}
	k8sClient.SetLogger(log)

	llmClient, err := llm.NewClient(cfg.LLM)
	if err != nil {
		return nil, err
	}
	llmClient.SetLogger(log)

	helmClient := helm.New("")
	helmClient.SetLogger(log)

	routerAgent, err := router.New(k8sClient, llmClient, cfg.MaxSteps, cfg.SupervisorCfg)
	if err != nil {
		return nil, err
	}
	routerAgent.SetLogger(log)

	return &Session{
		LLM:    llmClient,
		K8s:    k8sClient,
		Helm:   helmClient,
		Router: routerAgent,
	}, nil
}