package troubleshooting

import (
	"strings"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

// RecoveryToolDefinitions returns function definitions for the deploy recovery ReAct loop.
// It uses query + troubleshooting read tools, excluding security audit tools.
func RecoveryToolDefinitions(reg *toolv2.Registry) []llm.FunctionDefinition {
	if reg == nil {
		return nil
	}
	defs := reg.Definitions(reg.Names())
	var out []llm.FunctionDefinition
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "audit_") {
			continue
		}
		out = append(out, d)
	}
	return out
}
