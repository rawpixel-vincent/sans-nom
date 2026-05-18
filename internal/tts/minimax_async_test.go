package tts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAsyncClientCreateTaskBuildsRequest(t *testing.T) {
	var gotAuth string
	var gotRequest AsyncTaskRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != "/v1/t2a_async_v2" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(CreateTaskResponse{
			TaskID:          123,
			FileID:          456,
			UsageCharacters: 11,
			BaseResponse:    BaseResponse{StatusCode: 0, StatusMsg: "success"},
		})
	}))
	defer server.Close()

	resp, err := (AsyncClient{
		BaseURL: server.URL,
		APIKey:  "secret",
		Model:   "speech-2.8-turbo",
		VoiceID: "English_expressive_narrator",
		Format:  "wav",
	}).CreateTask(context.Background(), strings.Repeat("full text ", 20))
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotRequest.Model != "speech-2.8-turbo" || gotRequest.Text == "" {
		t.Fatalf("bad request: %#v", gotRequest)
	}
	if gotRequest.AudioSetting.Format != "wav" || gotRequest.AudioSetting.AudioSampleRate != 32000 || gotRequest.AudioSetting.Bitrate != 128000 || gotRequest.AudioSetting.Channel != 2 {
		t.Fatalf("bad audio settings: %#v", gotRequest.AudioSetting)
	}
	if gotRequest.VoiceSetting.VoiceID != "English_expressive_narrator" {
		t.Fatalf("bad voice settings: %#v", gotRequest.VoiceSetting)
	}
	if resp.TaskID != 123 || resp.FileID != 456 || resp.UsageCharacters != 11 {
		t.Fatalf("bad response: %#v", resp)
	}
}

func TestAsyncClientQueryParsesStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/query/t2a_async_query_v2" || r.URL.Query().Get("task_id") != "123" {
			t.Fatalf("unexpected query path: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(QueryTaskResponse{
			TaskID:       123,
			Status:       "Processing",
			FileID:       456,
			BaseResponse: BaseResponse{StatusCode: 0, StatusMsg: "success"},
		})
	}))
	defer server.Close()

	resp, err := (AsyncClient{BaseURL: server.URL, APIKey: "secret"}).QueryTask(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if resp.NormalizedStatus() != "processing" || resp.FileID != 456 {
		t.Fatalf("bad query response: %#v", resp)
	}
}

func TestAsyncClientDownloadReturnsRawAudio(t *testing.T) {
	audio := []byte("RIFF fake wav")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/files/retrieve_content" || r.URL.Query().Get("file_id") != "456" {
			t.Fatalf("unexpected download path: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(audio)
	}))
	defer server.Close()

	result, err := (AsyncClient{BaseURL: server.URL, APIKey: "secret"}).DownloadFile(context.Background(), 456)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Audio) != string(audio) || result.ContentType != "audio/wav" {
		t.Fatalf("bad download result: %#v", result)
	}
}

func TestAsyncClientSurfacesBaseResponseErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(CreateTaskResponse{
			BaseResponse: BaseResponse{StatusCode: 1001, StatusMsg: "bad request"},
		})
	}))
	defer server.Close()

	_, err := (AsyncClient{BaseURL: server.URL, APIKey: "secret"}).CreateTask(context.Background(), "text")
	if err == nil || !strings.Contains(err.Error(), "status_code=1001") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAsyncClientRequiresAPIKey(t *testing.T) {
	_, err := (AsyncClient{}).CreateTask(context.Background(), "text")
	if err == nil {
		t.Fatal("expected api key error")
	}
}
