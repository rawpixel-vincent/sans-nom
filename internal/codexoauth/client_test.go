package codexoauth

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClientUsesDedicatedNoReuseHTTPTransport(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")
	client, err := NewClient(Options{AuthPath: authPath})
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient == http.DefaultClient {
		t.Fatal("default Codex client must not share http.DefaultClient")
	}
	streamingTransport, ok := client.httpClient.Transport.(streamingRoundTripper)
	if !ok {
		t.Fatalf("unexpected transport: %T", client.httpClient.Transport)
	}
	transport := streamingTransport.base
	if transport == nil {
		t.Fatal("streaming transport base is required")
	}
	if !transport.DisableKeepAlives {
		t.Fatal("streaming Codex transport should disable keep-alive reuse")
	}
	if !transport.ForceAttemptHTTP2 {
		t.Fatal("streaming Codex transport should preserve HTTP/2 negotiation")
	}
	if transport.TLSNextProto != nil {
		t.Fatalf("streaming Codex transport should not override TLS protocols: %#v", transport.TLSNextProto)
	}
}

func TestNewClientPreservesInjectedHTTPClient(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")
	injected := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}}
	client, err := NewClient(Options{AuthPath: authPath, HTTPClient: injected})
	if err != nil {
		t.Fatal(err)
	}
	if client.httpClient != injected {
		t.Fatal("expected injected HTTP client to be preserved")
	}
}

func TestPostRequestsConnectionClose(t *testing.T) {
	root := t.TempDir()
	authPath := filepath.Join(root, "auth.json")
	closeSeen := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeSeen <- r.Close
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(Options{BaseURL: server.URL, AuthPath: authPath, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.post(t.Context(), map[string]any{"model": "gpt-5.5"}, &authState{
		accessToken: "token",
		accountID:   "account",
	})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if !<-closeSeen {
		t.Fatal("expected Codex streaming request to ask the server to close the connection")
	}
}

func TestCloseIdleBodyClosesIdleConnections(t *testing.T) {
	closedIdle := false
	body := closeIdleBody{
		ReadCloser: io.NopCloser(strings.NewReader("")),
		closeIdle:  func() { closedIdle = true },
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if !closedIdle {
		t.Fatal("expected streaming body close to close idle connections")
	}
}
