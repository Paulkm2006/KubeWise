package router

import (
	"fmt"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/operation"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/query"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/security"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/troubleshooting"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

type routedAgents struct {
	query           *query.Agent
	troubleshooting *troubleshooting.Agent
	security        *security.Agent
	operation       *operation.Agent
	deploy          *deploy.Agent
}

func (a *Agent) agentsForRequest(k8s *cluster.Client) (*routedAgents, error) {
	if k8s == a.k8sClient {
		return &routedAgents{
			query:           a.queryAgent,
			troubleshooting: a.troubleshootingAgent,
			security:        a.securityAgent,
			operation:       a.operationAgent,
			deploy:          a.deployAgent,
		}, nil
	}
	return a.buildRoutedAgents(k8s)
}

func (a *Agent) buildRoutedAgents(k8s *cluster.Client) (*routedAgents, error) {
	queryAgent, err := query.New(k8s, a.llmClient, query.WithMaxSteps(a.maxSteps), query.WithSupervisorConfig(a.supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("init query agent: %w", err)
	}
	troubleshootingAgent, err := troubleshooting.New(k8s, a.llmClient, troubleshooting.WithMaxSteps(a.maxSteps), troubleshooting.WithSupervisorConfig(a.supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("init troubleshooting agent: %w", err)
	}
	securityAgent, err := security.New(k8s, a.llmClient, security.WithMaxSteps(a.maxSteps), security.WithSupervisorConfig(a.supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("init security agent: %w", err)
	}
	operationAgent, err := operation.New(k8s, a.llmClient, operation.WithMaxSteps(a.maxSteps), operation.WithSupervisorConfig(a.supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("init operation agent: %w", err)
	}
	deployAgent := deploy.New(a.llmClient, a.helmClient, k8s)
	return &routedAgents{
		query:           queryAgent,
		troubleshooting: troubleshootingAgent,
		security:        securityAgent,
		operation:       operationAgent,
		deploy:          deployAgent,
	}, nil
}
