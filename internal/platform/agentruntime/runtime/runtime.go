// Package runtime bootstraps the agent runtime (LLM, K8s, router).
package runtime

import (
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/helm"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

type Config struct {
	LLM            llm.Config
	KubeConfig     string
	ClusterManager *cluster.ClusterClientManager
	MaxSteps       int
	SupervisorCfg  supervisor.Config
}

// Runtime holds shared agent-runtime dependencies for CLI, TUI, and HTTP API.
type Runtime struct {
	LLM      *llm.Client
	K8s      *cluster.Client
	Clusters *cluster.ClusterClientManager
	Helm     *helm.Client
	Router   *router.Agent
}

func New(cfg Config) (*Runtime, error) {
	k8sClient, err := cluster.NewClient(cfg.KubeConfig)
	if err != nil {
		return nil, err
	}

	llmClient, err := llm.NewClient(cfg.LLM)
	if err != nil {
		return nil, err
	}

	routerAgent, err := router.New(router.Config{
		K8sClient:      k8sClient,
		ClusterManager: cfg.ClusterManager,
		LLMClient:      llmClient,
		MaxSteps:       cfg.MaxSteps,
		SupervisorCfg:  cfg.SupervisorCfg,
	})
	if err != nil {
		return nil, err
	}

	return &Runtime{
		LLM:      llmClient,
		K8s:      k8sClient,
		Clusters: cfg.ClusterManager,
		Helm:     helm.New(""),
		Router:   routerAgent,
	}, nil
}
