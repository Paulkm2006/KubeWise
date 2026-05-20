// Package nodes holds deploy pipeline steps (plain functions over shared dependencies).
package nodes

import (
	"context"

	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/llm"
)

// HelmClient is the Helm surface used by deploy pipeline steps.
type HelmClient interface {
	helmtools.Client
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
	ListReleases(ctx context.Context) ([]helm.Release, error)
}

// LLMClient matches values and recovery LLM usage.
type LLMClient interface {
	ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}
