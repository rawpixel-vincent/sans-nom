package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultBaseURL       = "https://api.minimax.io"
	DefaultAsyncModel    = "speech-2.8-turbo"
	DefaultVoiceID       = "English_expressive_narrator"
	DefaultAudioFormat   = "wav"
	DefaultSampleRate    = 32000
	DefaultBitrate       = 128000
	DefaultChannel       = 2
	DefaultSpeed         = 1
	DefaultVolume        = 1
	DefaultPitch         = 1
	DefaultPollInterval  = 2 * time.Second
	DefaultAsyncTimeout  = 15 * time.Minute
	DefaultLegacyPrompt  = "Sci-fi story narrator voice."
	maxErrorResponseSize = 4096
)

type AsyncClient struct {
	BaseURL      string
	APIKey       string
	Model        string
	VoiceID      string
	Format       string
	HTTPClient   *http.Client
	SampleRate   int
	Bitrate      int
	Channel      int
	Speed        float64
	Volume       float64
	Pitch        float64
	Timeout      time.Duration
	PollInterval time.Duration
}

type VoiceSetting struct {
	VoiceID string  `json:"voice_id"`
	Speed   float64 `json:"speed"`
	Volume  float64 `json:"vol"`
	Pitch   float64 `json:"pitch"`
}

type AudioSetting struct {
	AudioSampleRate int    `json:"audio_sample_rate"`
	Bitrate         int    `json:"bitrate"`
	Format          string `json:"format"`
	Channel         int    `json:"channel"`
}

type AsyncTaskRequest struct {
	Model         string       `json:"model"`
	Text          string       `json:"text"`
	LanguageBoost string       `json:"language_boost"`
	VoiceSetting  VoiceSetting `json:"voice_setting"`
	AudioSetting  AudioSetting `json:"audio_setting"`
}

type BaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type CreateTaskResponse struct {
	TaskID          int64        `json:"task_id"`
	TaskToken       string       `json:"task_token,omitempty"`
	FileID          int64        `json:"file_id,omitempty"`
	UsageCharacters int          `json:"usage_characters,omitempty"`
	BaseResponse    BaseResponse `json:"base_resp"`
}

type QueryTaskResponse struct {
	TaskID       int64        `json:"task_id"`
	Status       string       `json:"status"`
	FileID       int64        `json:"file_id,omitempty"`
	BaseResponse BaseResponse `json:"base_resp"`
}

type DownloadResult struct {
	Audio       []byte
	StatusCode  int
	ContentType string
}

type APIError struct {
	Operation string
	Status    string
	Body      string
	BaseResp  BaseResponse
}

func (e APIError) Error() string {
	if e.BaseResp.StatusCode != 0 || e.BaseResp.StatusMsg != "" {
		return fmt.Sprintf("minimax tts %s failed: base_resp status_code=%d status_msg=%q", e.Operation, e.BaseResp.StatusCode, e.BaseResp.StatusMsg)
	}
	if e.Status != "" {
		return fmt.Sprintf("minimax tts %s failed: %s: %s", e.Operation, e.Status, e.Body)
	}
	return "minimax tts " + e.Operation + " failed"
}

func (c AsyncClient) RequestPayload(text string) AsyncTaskRequest {
	return AsyncTaskRequest{
		Model:         firstNonEmpty(c.Model, DefaultAsyncModel),
		Text:          text,
		LanguageBoost: "auto",
		VoiceSetting: VoiceSetting{
			VoiceID: firstNonEmpty(c.VoiceID, DefaultVoiceID),
			Speed:   firstPositiveFloat(c.Speed, DefaultSpeed),
			Volume:  firstPositiveFloat(c.Volume, DefaultVolume),
			Pitch:   firstPositiveFloat(c.Pitch, DefaultPitch),
		},
		AudioSetting: AudioSetting{
			AudioSampleRate: firstPositiveInt(c.SampleRate, DefaultSampleRate),
			Bitrate:         firstPositiveInt(c.Bitrate, DefaultBitrate),
			Format:          firstNonEmpty(c.Format, DefaultAudioFormat),
			Channel:         firstPositiveInt(c.Channel, DefaultChannel),
		},
	}
}

func (c AsyncClient) CreateTask(ctx context.Context, text string) (CreateTaskResponse, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return CreateTaskResponse{}, errors.New("tts text is empty")
	}
	payload := c.RequestPayload(text)
	var out CreateTaskResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/t2a_async_v2", nil, payload, &out, "create")
	if err != nil {
		return out, err
	}
	if out.BaseResponse.StatusCode != 0 {
		return out, APIError{Operation: "create", BaseResp: out.BaseResponse}
	}
	if out.TaskID == 0 {
		return out, errors.New("minimax tts create response missing task_id")
	}
	return out, nil
}

func (c AsyncClient) QueryTask(ctx context.Context, taskID int64) (QueryTaskResponse, error) {
	if taskID == 0 {
		return QueryTaskResponse{}, errors.New("task_id is required")
	}
	query := url.Values{"task_id": []string{fmt.Sprintf("%d", taskID)}}
	var out QueryTaskResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/query/t2a_async_query_v2", query, nil, &out, "query")
	if err != nil {
		return out, err
	}
	if out.BaseResponse.StatusCode != 0 {
		return out, APIError{Operation: "query", BaseResp: out.BaseResponse}
	}
	return out, nil
}

func (c AsyncClient) DownloadFile(ctx context.Context, fileID int64) (DownloadResult, error) {
	if fileID == 0 {
		return DownloadResult{}, errors.New("file_id is required")
	}
	query := url.Values{"file_id": []string{fmt.Sprintf("%d", fileID)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/v1/files/retrieve_content", query), nil)
	if err != nil {
		return DownloadResult{}, err
	}
	if err := c.authorize(req); err != nil {
		return DownloadResult{}, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return DownloadResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return DownloadResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return DownloadResult{}, APIError{Operation: "download", Status: resp.Status, Body: trimForError(data)}
	}
	return DownloadResult{Audio: data, StatusCode: resp.StatusCode, ContentType: resp.Header.Get("Content-Type")}, nil
}

func (c AsyncClient) PollIntervalOrDefault() time.Duration {
	if c.PollInterval > 0 {
		return c.PollInterval
	}
	return DefaultPollInterval
}

func (c AsyncClient) TimeoutOrDefault() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultAsyncTimeout
}

func (r QueryTaskResponse) NormalizedStatus() string {
	return strings.ToLower(strings.TrimSpace(r.Status))
}

func (c AsyncClient) doJSON(ctx context.Context, method string, path string, query url.Values, payload any, out any, operation string) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.authorize(req); err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return APIError{Operation: operation, Status: resp.Status, Body: trimForError(data)}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse minimax tts %s response: %w", operation, err)
	}
	return nil
}

func (c AsyncClient) endpoint(path string, query url.Values) string {
	base := strings.TrimRight(firstNonEmpty(c.BaseURL, DefaultBaseURL), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	out := base + path
	if len(query) > 0 {
		out += "?" + query.Encode()
	}
	return out
}

func (c AsyncClient) authorize(req *http.Request) error {
	apiKey := strings.TrimSpace(c.APIKey)
	if apiKey == "" {
		return errors.New("minimax tts api key is required")
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	return nil
}

func (c AsyncClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func trimForError(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) <= maxErrorResponseSize {
		return text
	}
	return text[:maxErrorResponseSize] + "...(truncated)"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstPositiveFloat(value float64, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}
