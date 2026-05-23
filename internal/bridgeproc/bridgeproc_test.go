package bridgeproc

import (
	"context"
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
