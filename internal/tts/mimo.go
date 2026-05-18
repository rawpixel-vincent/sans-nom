package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultBaseURL             = "https://token-plan-sgp.xiaomimimo.com/v1"
	DefaultModel               = "mimo-v2.5-tts"
	DefaultVoice               = "Milo"
	DefaultAudioFormat         = "wav"
	DefaultNarratorInstruction = "Sci-fi story narrator voice."
	DefaultTimeout             = 15 * time.Minute
	maxErrorResponseSize       = 4096
)

type Client struct {
	BaseURL     string
	APIKey      string
	Model       string
	Voice       string
	Format      string
	Instruction string
	HTTPClient  *http.Client
	Timeout     time.Duration
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AudioRequest struct {
	Format string `json:"format"`
	Voice  string `json:"voice,omitempty"`
}

type ChatCompletionRequest struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Audio    AudioRequest `json:"audio"`
}

type ChatCompletionResponse struct {
	ID      string        `json:"id,omitempty"`
	Object  string        `json:"object,omitempty"`
	Choices []Choice      `json:"choices"`
	Usage   any           `json:"usage,omitempty"`
	Error   *APIErrorBody `json:"error,omitempty"`
}

type Choice struct {
	Index        int             `json:"index,omitempty"`
	Message      ResponseMessage `json:"message"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

type ResponseMessage struct {
	Role    string         `json:"role,omitempty"`
	Content string         `json:"content,omitempty"`
	Audio   *AudioResponse `json:"audio,omitempty"`
}

type AudioResponse struct {
	Data      string `json:"data"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	ID        string `json:"id,omitempty"`
}

type APIErrorBody struct {
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

type Result struct {
	Audio            []byte
	AudioBase64Chars int
	Response         ChatCompletionResponse
}

type APIError struct {
	Operation string
	Status    string
	Body      string
	API       *APIErrorBody
}

func (e APIError) Error() string {
	if e.API != nil && e.API.Message != "" {
		return fmt.Sprintf("mimo tts %s failed: %s", e.Operation, e.API.Message)
	}
	if e.Status != "" {
		return fmt.Sprintf("mimo tts %s failed: %s: %s", e.Operation, e.Status, e.Body)
	}
	return "mimo tts " + e.Operation + " failed"
}

func (c Client) RequestPayload(text string) ChatCompletionRequest {
	messages := make([]Message, 0, 2)
	if instruction := strings.TrimSpace(firstNonEmpty(c.Instruction, DefaultNarratorInstruction)); instruction != "" {
		messages = append(messages, Message{Role: "user", Content: instruction})
	}
	messages = append(messages, Message{Role: "assistant", Content: text})
	return ChatCompletionRequest{
		Model:    firstNonEmpty(c.Model, DefaultModel),
		Messages: messages,
		Audio: AudioRequest{
			Format: firstNonEmpty(c.Format, DefaultAudioFormat),
			Voice:  firstNonEmpty(c.Voice, DefaultVoice),
		},
	}
}

func (c Client) Synthesize(ctx context.Context, text string) (Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, errors.New("tts text is empty")
	}
	payload := c.RequestPayload(text)
	data, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/chat/completions"), bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.authorize(req); err != nil {
		return Result{}, err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	var parsed ChatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return Result{}, fmt.Errorf("parse mimo tts response: %w", err)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{Response: parsed}, APIError{Operation: "synthesize", Status: resp.Status, Body: trimForError(body), API: parsed.Error}
	}
	if parsed.Error != nil {
		return Result{Response: parsed}, APIError{Operation: "synthesize", API: parsed.Error}
	}
	audioBase64, err := audioData(parsed)
	if err != nil {
		return Result{Response: parsed}, err
	}
	audio, err := base64.StdEncoding.DecodeString(audioBase64)
	if err != nil {
		return Result{Response: parsed, AudioBase64Chars: len(audioBase64)}, fmt.Errorf("decode mimo tts audio: %w", err)
	}
	return Result{Audio: audio, AudioBase64Chars: len(audioBase64), Response: parsed}, nil
}

func (c Client) TimeoutOrDefault() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

func (c Client) endpoint(path string) string {
	base := strings.TrimRight(firstNonEmpty(c.BaseURL, DefaultBaseURL), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (c Client) authorize(req *http.Request) error {
	apiKey := strings.TrimSpace(c.APIKey)
	if apiKey == "" {
		return errors.New("mimo tts api key is required")
	}
	req.Header.Set("api-key", apiKey)
	return nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func audioData(resp ChatCompletionResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", errors.New("mimo tts response had no choices")
	}
	audio := resp.Choices[0].Message.Audio
	if audio == nil || strings.TrimSpace(audio.Data) == "" {
		return "", errors.New("mimo tts response had no audio data")
	}
	return strings.TrimSpace(audio.Data), nil
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
