package troubleshooting

import (
	"strings"

	"github.com/kubewise/kubewise/internal/agent/tool"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

// RecoveryToolDefinitions returns function definitions for the deploy recovery ReAct loop.
// It uses the same category-"" registry surface as this package's agent (query + troubleshooting
// read tools), but excludes security audit tools that are not appropriate for automated recovery.
func RecoveryToolDefinitions(reg *tool.Registry) []llm.FunctionDefinition {
	if reg == nil {
		return nil
	}
	defs := reg.GetAllFunctionDefinitions()
	var out []llm.FunctionDefinition
	for _, d := range defs {
		if strings.HasPrefix(d.Name, "audit_") {
			continue
		}
		out = append(out, d)
	}
	return out
}
