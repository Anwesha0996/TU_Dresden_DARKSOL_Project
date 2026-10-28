package aap2_client

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	aap2 "github.com/darksol-cloud/helm-dtn/pkg/proto"
)

func TestReceiveAduFileStreamsExactLength(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	a := &AAP2SyncClient{AAP2BaseClient: AAP2BaseClient{connection: client}}
	want := "streamed-image-payload"
	go func() {
		_, _ = io.WriteString(server, want+"not-part-of-this-adu")
	}()
	path := filepath.Join(t.TempDir(), "payload")
	if _, err := a.ReceiveAduFile(&aap2.BundleADU{PayloadLength: uint64(len(want))}, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

func TestSendFileRejectsLengthMismatch(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	a := &AAP2SyncClient{AAP2BaseClient: AAP2BaseClient{connection: client}}
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.SendFile(&aap2.BundleADU{PayloadLength: 999}, path)
	if err == nil || !strings.Contains(err.Error(), "does not match file size") {
		t.Fatalf("expected size mismatch, got %v", err)
	}
}
