package audit

import securitytools "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/security"

type PhaseInfo struct {
	ID       string
	Label    string
	Category string
}

var toolPhases = map[string]PhaseInfo{
	securitytools.AuditRBACToolName: {
		ID: "rbac", Label: "RBAC", Category: "RBAC",
	},
	securitytools.AuditPodSecurityToolName: {
		ID: "pod_security", Label: "Pod Security", Category: "Pod Security",
	},
	securitytools.AuditNetworkPoliciesToolName: {
		ID: "network_policies", Label: "Network Policies", Category: "Network Policy",
	},
	securitytools.AuditImageSecurityToolName: {
		ID: "image_security", Label: "Image Security", Category: "Image Security",
	},
}

func PhaseForTool(toolName string) (PhaseInfo, bool) {
	p, ok := toolPhases[toolName]
	return p, ok
}
