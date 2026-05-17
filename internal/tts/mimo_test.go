package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMimoClientSynthesizeBuildsRequestAndDecodesAudio(t *testing.T) {
	audio := []byte("RIFF fake wav")
	var gotKey string
	var gotRequest ChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("api-key")
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"audio": map[string]any{"data": base64.StdEncoding.EncodeToString(audio)},
				},
			}},
		})
	}))
	defer server.Close()

	result, err := (MimoClient{
		Endpoint:    server.URL,
		APIKey:      "secret",
		Model:       "mimo-v2.5-tts",
		Voice:       "Milo",
		Format:      "wav",
		Instruction: "Sci-fi story narrator voice.",
	}).Synthesize(context.Background(), "Turn prose.")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "secret" {
		t.Fatalf("api key header = %q", gotKey)
	}
	if gotRequest.Model != "mimo-v2.5-tts" || gotRequest.Audio.Voice != "Milo" || gotRequest.Audio.Format != "wav" {
		t.Fatalf("unexpected request: %#v", gotRequest)
	}
	if len(gotRequest.Messages) != 2 || gotRequest.Messages[0].Role != "user" || gotRequest.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected messages: %#v", gotRequest.Messages)
	}
	if string(result.Audio) != string(audio) {
		t.Fatalf("audio = %q", result.Audio)
	}
	if result.AudioBase64Chars == 0 {
		t.Fatal("missing base64 length")
	}
}

func TestMimoClientRequiresAPIKey(t *testing.T) {
	_, err := (MimoClient{}).Synthesize(context.Background(), "text")
	if err == nil {
		t.Fatal("expected api key error")
	}
}
