package query

import "testing"

func TestMapString_nilValue(t *testing.T) {
	m := map[string]interface{}{
		"name":      "foo",
		"namespace": nil,
	}
	if got := mapString(m, "namespace"); got != "" {
		t.Fatalf("namespace = %q, want empty", got)
	}
	if got := mapString(m, "name"); got != "foo" {
		t.Fatalf("name = %q", got)
	}
}
