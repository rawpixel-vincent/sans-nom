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
	DefaultMimoEndpoint    = "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions"
	DefaultMimoModel       = "mimo-v2.5-tts"
	DefaultMimoVoice       = "Milo"
	DefaultAudioFormat     = "wav"
	DefaultNarratorPrompt  = "Sci-fi story narrator voice."
	defaultRequestTimeout  = 5 * time.Minute
	maxErrorResponseLength = 4096
)

type MimoClient struct {
	Endpoint    string
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

type AudioOptions struct {
	Format string `json:"format"`
	Voice  string `json:"voice"`
}

type ChatRequest struct {
	Model    string       `json:"model"`
	Messages []Message    `json:"messages"`
	Audio    AudioOptions `json:"audio"`
}

type Result struct {
	Audio            []byte
	AudioBase64Chars int
}

func (c MimoClient) RequestPayload(text string) ChatRequest {
	model := firstNonEmpty(c.Model, DefaultMimoModel)
	voice := firstNonEmpty(c.Voice, DefaultMimoVoice)
	format := firstNonEmpty(c.Format, DefaultAudioFormat)
	instruction := firstNonEmpty(c.Instruction, DefaultNarratorPrompt)
	return ChatRequest{
		Model: model,
		Messages: []Message{
			{Role: "user", Content: instruction},
			{Role: "assistant", Content: text},
		},
		Audio: AudioOptions{Format: format, Voice: voice},
	}
}

func (c MimoClient) Synthesize(ctx context.Context, text string) (Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Result{}, errors.New("tts text is empty")
	}
	endpoint := firstNonEmpty(c.Endpoint, DefaultMimoEndpoint)
	apiKey := strings.TrimSpace(c.APIKey)
	if apiKey == "" {
		return Result{}, errors.New("mimo tts api key is required")
	}
	payload := c.RequestPayload(text)
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", apiKey)
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("mimo tts %s: %s", resp.Status, trimForError(data))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{}, fmt.Errorf("parse mimo tts response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Audio.Data) == "" {
		return Result{}, errors.New("mimo tts response missing choices[0].message.audio.data")
	}
	encoded := strings.TrimSpace(parsed.Choices[0].Message.Audio.Data)
	audio, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return Result{}, fmt.Errorf("decode mimo tts audio: %w", err)
	}
	return Result{Audio: audio, AudioBase64Chars: len(encoded)}, nil
}

func trimForError(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) <= maxErrorResponseLength {
		return text
	}
	return text[:maxErrorResponseLength] + "...(truncated)"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
