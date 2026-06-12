// Package appname extracts application names from router entities or free-text queries.
package appname

import (
	"strings"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
)

// FromEntities prefers explicit entity fields from the router.
func FromEntities(entities types.Entities, query string) string {
	if entities.AppName != "" {
		return entities.AppName
	}
	if entities.ResourceName != "" {
		return entities.ResourceName
	}
	return InferFromQuery(query)
}

// InferFromQuery parses common deploy/install prefixes in Chinese or English.
func InferFromQuery(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, prefix := range []string{"部署", "安装", "deploy", "install"} {
		if idx := strings.Index(q, prefix); idx >= 0 {
			rest := strings.TrimSpace(q[idx+len(prefix):])
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strings.Trim(fields[0], "，,. ")
			}
		}
	}
	return ""
}
