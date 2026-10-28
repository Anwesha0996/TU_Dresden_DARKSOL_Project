package imagetransfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultTarget(t *testing.T) {
	tests := map[string]string{
		"registry.k8s.io/pause:3.10":         "pause:3.10",
		"docker.io/library/nginx:alpine":     "library/nginx:alpine",
		"quay.io/example/project/image:v1.2": "example/project/image:v1.2",
	}
	for source, want := range tests {
		got, err := DefaultTarget(source)
		if err != nil {
			t.Fatalf("DefaultTarget(%q): %v", source, err)
		}
		if got != want {
			t.Errorf("DefaultTarget(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestValidateTarget(t *testing.T) {
	valid := []string{"kubernetes/pause:3.10", "coredns/coredns:v1.12.1"}
	for _, target := range valid {
		if err := ValidateTarget(target); err != nil {
			t.Errorf("ValidateTarget(%q): %v", target, err)
		}
	}
	invalid := []string{"", "/absolute:1", "https://evil/image:1", "../escape:1", "repo/../escape:1", "repo//image:1"}
	for _, target := range invalid {
		if err := ValidateTarget(target); err == nil {
			t.Errorf("ValidateTarget(%q) unexpectedly succeeded", target)
		}
	}
}

func TestPayloadRoundTripAndCorruption(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "image.oci.tar")
	payload := filepath.Join(dir, "payload")
	decoded := filepath.Join(dir, "decoded.oci.tar")
	content := []byte("small deterministic OCI test placeholder")
	if err := os.WriteFile(archive, content, 0600); err != nil {
		t.Fatal(err)
	}
	sum, size, err := hashFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{TransferID: "test-id", SourceImageRef: "registry.k8s.io/pause:3.10", TargetImageRef: "kubernetes/pause:3.10", SourceDigest: "sha256:source", ArchiveSHA256: sum, ArchiveSizeBytes: size}
	if _, err := CreatePayload(header, archive, payload); err != nil {
		t.Fatal(err)
	}
	got, err := DecodePayload(payload, decoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.TransferID != header.TransferID {
		t.Fatalf("transfer ID = %q, want %q", got.TransferID, header.TransferID)
	}
	data, err := os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	corrupt := filepath.Join(dir, "corrupt.payload")
	if err := os.WriteFile(corrupt, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = DecodePayload(corrupt, filepath.Join(dir, "corrupt.oci.tar"))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestDefaultTargetRequiresTargetForDigest(t *testing.T) {
	_, err := DefaultTarget("registry.k8s.io/pause@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Fatal("expected digest reference error")
	}
}
