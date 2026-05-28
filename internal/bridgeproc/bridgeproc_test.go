package bridgeproc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBridgeCommandUsesUvicornMainApp(t *testing.T) {
	cmd := bridgeCommand(context.Background(), "127.0.0.1", "8787")
	want := []string{
		"bridge/.venv/bin/python",
		"-m",
		"uvicorn",
		"bridge.main:app",
		"--host",
		"127.0.0.1",
		"--port",
		"8787",
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("bridge command args mismatch\nwant: %#v\n got: %#v", want, cmd.Args)
	}
}

func TestListenAddressRejectsRemoteAutoStart(t *testing.T) {
	host, port, err := ListenAddress("http://127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port != "8787" {
		t.Fatalf("unexpected local listen address: host=%q port=%q", host, port)
	}
	if _, _, err := ListenAddress("https://example.com:8787"); err == nil {
		t.Fatal("expected remote bridge URL to be rejected for auto-start")
	}
}

func TestEnsureQdrantReturnsWhenAlreadyHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"title":"qdrant"}`))
	}))
	defer server.Close()

	t.Setenv("QDRANT_URL", server.URL)
	if err := EnsureQdrant(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDockerPathFromCandidates(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := dockerPathFromCandidates([]string{filepath.Join(dir, "missing"), docker})
	if err != nil {
		t.Fatal(err)
	}
	if got != docker {
		t.Fatalf("docker path = %q, want %q", got, docker)
	}
}
