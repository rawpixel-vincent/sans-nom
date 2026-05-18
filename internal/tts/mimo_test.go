package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSynthesizeBuildsOpenAICompatibleRequest(t *testing.T) {
	audio := []byte("RIFF fake wav")
	var gotAPIKey string
	var gotRequest ChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAPIKey = r.Header.Get("api-key")
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(ChatCompletionResponse{
			Choices: []Choice{{
				Message: ResponseMessage{
					Role:  "assistant",
					Audio: &AudioResponse{Data: base64.StdEncoding.EncodeToString(audio)},
				},
			}},
		})
	}))
	defer server.Close()

	result, err := (Client{
		BaseURL:     server.URL + "/v1",
		APIKey:      "secret",
		Model:       "mimo-v2.5-tts",
		Voice:       "Milo",
		Format:      "wav",
		Instruction: "Sci-fi story narrator voice.",
	}).Synthesize(context.Background(), strings.Repeat("full text ", 20))
	if err != nil {
		t.Fatal(err)
	}
	if gotAPIKey != "secret" {
		t.Fatalf("api-key header = %q", gotAPIKey)
	}
	if gotRequest.Model != "mimo-v2.5-tts" {
		t.Fatalf("model = %q", gotRequest.Model)
	}
	if len(gotRequest.Messages) != 2 {
		t.Fatalf("messages = %#v", gotRequest.Messages)
	}
	if gotRequest.Messages[0].Role != "user" || gotRequest.Messages[0].Content != "Sci-fi story narrator voice." {
		t.Fatalf("bad instruction message: %#v", gotRequest.Messages[0])
	}
	if gotRequest.Messages[1].Role != "assistant" || gotRequest.Messages[1].Content == "" {
		t.Fatalf("bad spoken text message: %#v", gotRequest.Messages[1])
	}
	if gotRequest.Audio.Format != "wav" || gotRequest.Audio.Voice != "Milo" {
		t.Fatalf("bad audio request: %#v", gotRequest.Audio)
	}
	if string(result.Audio) != string(audio) || result.AudioBase64Chars == 0 {
		t.Fatalf("bad synth result: %#v", result)
	}
}

func TestClientSurfacesAPIErrorWithoutSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad text"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := (Client{BaseURL: server.URL, APIKey: "secret"}).Synthesize(context.Background(), "text")
	if err == nil || !strings.Contains(err.Error(), "bad text") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientRequiresAPIKey(t *testing.T) {
	_, err := (Client{}).Synthesize(context.Background(), "text")
	if err == nil {
		t.Fatal("expected api key error")
	}
}
