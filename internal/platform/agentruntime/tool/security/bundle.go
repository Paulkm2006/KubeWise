package security

import (
	"fmt"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

const (
	AuditImageSecurityToolName   = "audit_image_security"
	AuditNetworkPoliciesToolName = "audit_network_policies"
	AuditPodSecurityToolName     = "audit_pod_security"
	AuditRBACToolName            = "audit_rbac"
)

var auditToolNames = []string{
	AuditImageSecurityToolName,
	AuditNetworkPoliciesToolName,
	AuditPodSecurityToolName,
	AuditRBACToolName,
}

// NewAuditBundle returns the explicit toolv2 bundle for Kubernetes security audits.
func NewAuditBundle() toolv2.Bundle {
	return toolv2.Bundle{
		Name:  toolv2.BundleSecurityAudit,
		Tools: append([]string(nil), auditToolNames...),
	}
}

// RegisterAuditTools registers all native security audit tools into a toolv2 registry.
func RegisterAuditTools(registry *toolv2.Registry, k8sClient *cluster.Client) error {
	if registry == nil {
		return fmt.Errorf("register security audit tools: registry is nil")
	}
	for _, auditTool := range []toolv2.Tool{
		NewAuditImageSecurityTool(k8sClient),
		NewAuditNetworkPoliciesTool(k8sClient),
		NewAuditPodSecurityTool(k8sClient),
		NewAuditRBACTool(k8sClient),
	} {
		if err := registry.Register(auditTool); err != nil {
			return err
		}
	}
	return nil
}
