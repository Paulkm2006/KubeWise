package nodes

import (
	"github.com/kubewise/kubewise/pkg/agent/deploy/core/appname"
	"github.com/kubewise/kubewise/pkg/types"
)

// AppName derives the deployment target name from routed entities or the raw query text.
func AppName(entities types.Entities, query string) string {
	return appname.FromEntities(entities, query)
}
