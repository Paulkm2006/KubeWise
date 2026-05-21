package appname

import (
	"testing"

	"github.com/kubewise/kubewise/pkg/types"
)

func TestFromEntities_prefersAppName(t *testing.T) {
	got := FromEntities(types.Entities{AppName: "nginx", ResourceName: "other"}, "")
	if got != "nginx" {
		t.Fatalf("got %q", got)
	}
}

func TestInferFromQuery_deployPrefix(t *testing.T) {
	if got := InferFromQuery("部署 myapp 到生产"); got != "myapp" {
		t.Fatalf("got %q", got)
	}
}
