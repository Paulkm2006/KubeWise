package log

import (
	"os"
	"testing"
)

func TestResolveSink_defaultVerboseUsesFile(t *testing.T) {
	sink, desc, err := resolveSink("stderr", false)
	if err != nil {
		t.Fatal(err)
	}
	if sink == os.Stderr {
		t.Fatal("expected file sink, got stderr")
	}
	if desc != "./kubewise.log" {
		t.Fatalf("desc = %q, want ./kubewise.log", desc)
	}
	_ = sink.Close()
}

func TestResolveSink_explicitStderr(t *testing.T) {
	sink, desc, err := resolveSink("stderr", true)
	if err != nil {
		t.Fatal(err)
	}
	if sink != os.Stderr {
		t.Fatal("expected stderr")
	}
	if desc != "stderr" {
		t.Fatalf("desc = %q", desc)
	}
}

func TestResolveSink_explicitPath(t *testing.T) {
	sink, desc, err := resolveSink("/tmp/kw-test.log", true)
	if err != nil {
		t.Fatal(err)
	}
	if sink == os.Stderr {
		t.Fatal("expected file")
	}
	if desc != "/tmp/kw-test.log" {
		t.Fatalf("desc = %q", desc)
	}
	_ = sink.Close()
	_ = os.Remove("/tmp/kw-test.log")
}
