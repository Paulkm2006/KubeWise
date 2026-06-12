package audit

import "strings"

func impactFor(body, category string) string {
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "cluster-admin") || strings.Contains(lower, "cluster-admin"):
		return "Subject can perform any action cluster-wide"
	case strings.Contains(lower, "privileged"):
		return "Container can access host kernel capabilities"
	case strings.Contains(lower, "wildcard") || strings.Contains(lower, "动词(*)") || strings.Contains(lower, "资源(*)"):
		return "Overly broad permissions increase blast radius"
	case strings.Contains(lower, "hostnetwork") || strings.Contains(lower, "hostnetwork"):
		return "Pod shares the host network namespace"
	case strings.Contains(lower, "hostpath"):
		return "Pod can read or write host filesystem paths"
	case strings.Contains(lower, "networkpolicy") || strings.Contains(lower, "network policy"):
		return "Workload traffic is not restricted by policy"
	case strings.Contains(lower, "latest"):
		return "Image tag is mutable and may change unexpectedly"
	case strings.Contains(lower, "exec") || strings.Contains(lower, "portforward"):
		return "Subject can access running workloads interactively"
	case strings.Contains(lower, "root"):
		return "Container may run with elevated UID 0"
	default:
		return "Security posture deviation in " + category
	}
}

func suggestFor(body, category string) string {
	lower := strings.ToLower(body)
	switch {
	case strings.Contains(lower, "cluster-admin"):
		return "Replace with scoped ClusterRoles using least-privilege verbs"
	case strings.Contains(lower, "wildcard") || strings.Contains(lower, "动词(*)") || strings.Contains(lower, "资源(*)"):
		return "Replace wildcard verbs/resources with explicit least-privilege rules"
	case strings.Contains(lower, "privileged"):
		return "Set securityContext.privileged=false and use required capabilities only"
	case strings.Contains(lower, "hostnetwork") || strings.Contains(lower, "hostnetwork"):
		return "Disable hostNetwork unless the workload truly requires it"
	case strings.Contains(lower, "hostpid") || strings.Contains(lower, "hostipc"):
		return "Disable hostPID/hostIPC and isolate the workload"
	case strings.Contains(lower, "hostpath"):
		return "Avoid hostPath volumes; use PVCs or emptyDir where possible"
	case strings.Contains(lower, "allowprivilegeescalation"):
		return "Set allowPrivilegeEscalation=false on containers"
	case strings.Contains(lower, "root") || strings.Contains(lower, "runasnonroot"):
		return "Set runAsNonRoot=true and an explicit non-zero runAsUser"
	case strings.Contains(lower, "networkpolicy") || strings.Contains(lower, "未配置"):
		return "Add default-deny NetworkPolicies and allow only required traffic"
	case strings.Contains(lower, "latest"):
		return "Pin images to immutable tags or digests"
	case strings.Contains(lower, "imagepullpolicy: never") || strings.Contains(lower, "never"):
		return "Use IfNotPresent or Always instead of Never for production images"
	case strings.Contains(lower, "imagepullsecrets"):
		return "Configure imagePullSecrets for private registry access"
	case strings.Contains(lower, "exec") || strings.Contains(lower, "portforward"):
		return "Restrict exec/portforward to narrowly scoped Roles"
	case strings.Contains(lower, "serviceaccount") && strings.Contains(lower, "未绑定"):
		return "Delete unused ServiceAccounts or bind them to explicit Roles"
	default:
		return "Review and remediate according to least-privilege best practices"
	}
}
