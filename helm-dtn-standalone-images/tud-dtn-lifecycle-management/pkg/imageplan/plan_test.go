package imageplan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRepeatedFlags(t *testing.T) {
	requests, err := Build([]string{"registry.k8s.io/pause:3.10", "docker.io/library/nginx:alpine"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0].Target != "pause:3.10" || requests[1].Target != "library/nginx:alpine" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestBuildFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "images.txt")
	if err := os.WriteFile(path, []byte("# provisioning images\nregistry.k8s.io/pause:3.10=kubernetes/pause:3.10\n"), 0600); err != nil {
		t.Fatal(err)
	}
	requests, err := Build(nil, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].Source != "registry.k8s.io/pause:3.10" || requests[0].Target != "kubernetes/pause:3.10" {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestBuildRejectsMismatchedTargets(t *testing.T) {
	if _, err := Build([]string{"a:1", "b:2"}, []string{"a:1"}, ""); err == nil {
		t.Fatal("expected target count error")
	}
}
