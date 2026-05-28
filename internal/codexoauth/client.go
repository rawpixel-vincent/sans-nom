package codexoauth

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL                = "https://chatgpt.com/backend-api/codex"
	codexResponseHeaderTimeoutEnv = "SANS_NOM_CODEX_RESPONSE_HEADER_TIMEOUT_MS"
	oauthClientID                 = "app_EMoamEEZ73f0CkXaXp7hrann"
	oauthRefreshURL               = "https://auth.openai.com/oauth/token"
	defaultOriginator             = "codex_cli_rs"
	defaultSystemPrompt           = "You are Codex, based on GPT-5. You are running as a coding agent in the Codex CLI on a user's computer."
	defaultResponseHeaderTimeout  = 30 * time.Second
)

var supportedModels = map[string]bool{
	"gpt-5.2":             true,
	"gpt-5.3-codex":       true,
	"gpt-5.3-codex-spark": true,
	"gpt-5.4":             true,
	"gpt-5.4-mini":        true,
	"gpt-5.5":             true,
}

// Options configures a Codex OAuth-backed Responses client.
type Options struct {
	BaseURL       string
	AuthPath      string
	Originator    string
	SessionID     string
	SystemPrompt  string
	ModelOverride string
	HTTPClient    *http.Client
}

// Client is a small Responses API client for the ChatGPT Codex backend using
// the local Codex OAuth bundle in ~/.codex/auth.json.
type Client struct {
	baseURL               string
	authPath              string
	originator            string
	sessionID             string
	systemPrompt          string
	modelOverride         string
	httpClient            *http.Client
	responseHeaderTimeout time.Duration
}

type authState struct {
	bundle       map[string]any
	accessToken  string
	refreshToken string
	accountID    string
}

// NewClient creates a Codex OAuth client.
func NewClient(opts Options) (*Client, error) {
	authPath := opts.AuthPath
	if authPath == "" {
		var err error
		authPath, err = DefaultAuthPath()
		if err != nil {
			return nil, err
		}
	}

	sessionID := opts.SessionID
	if sessionID == "" {
		sessionID = randomSessionID()
	}

	responseHeaderTimeout := codexResponseHeaderTimeout()
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = defaultStreamingHTTPClient(responseHeaderTimeout)
	}

	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	originator := opts.Originator
	if originator == "" {
		originator = defaultOriginator
	}

	systemPrompt := opts.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	return &Client{
		baseURL:               baseURL,
		authPath:              authPath,
		originator:            originator,
		sessionID:             sessionID,
		systemPrompt:          systemPrompt,
		modelOverride:         opts.ModelOverride,
		httpClient:            httpClient,
		responseHeaderTimeout: responseHeaderTimeout,
	}, nil
}

func defaultStreamingHTTPClient(responseHeaderTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Codex Responses calls are long-lived SSE streams. Local tool rounds may
	// intentionally stop reading as soon as a function call is available, so do
	// not reuse those half-consumed streams through the shared idle pool. Keep
	// Go's HTTP/2 negotiation intact: chatgpt.com may select h2 during TLS.
	transport.DisableKeepAlives = true
	transport.MaxIdleConns = 0
	transport.MaxIdleConnsPerHost = -1
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{Transport: streamingRoundTripper{base: transport}}
}

func codexResponseHeaderTimeout() time.Duration {
	raw, ok := os.LookupEnv(codexResponseHeaderTimeoutEnv)
	if !ok {
		return defaultResponseHeaderTimeout
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultResponseHeaderTimeout
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms < 0 {
		return defaultResponseHeaderTimeout
	}
	if ms == 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

type streamingRoundTripper struct {
	base *http.Transport
}

func (t streamingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := t.base.RoundTrip(req)
	if err != nil || res == nil || res.Body == nil {
		return res, err
	}
	res.Body = closeIdleBody{ReadCloser: res.Body, closeIdle: t.base.CloseIdleConnections}
	return res, nil
}

type closeIdleBody struct {
	io.ReadCloser
	closeIdle func()
}

func (b closeIdleBody) Close() error {
	err := b.ReadCloser.Close()
	if b.closeIdle != nil {
		b.closeIdle()
	}
	return err
}

// DefaultAuthPath resolves CODEX_HOME/auth.json or ~/.codex/auth.json.
func DefaultAuthPath() (string, error) {
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(codexHome, "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

// ResolveModel maps caller-facing model names to models accepted by the Codex backend.
func ResolveModel(model string) string {
	switch {
	case model == "":
		return "gpt-5.5"
	case supportedModels[model]:
		return model
	case model == "gpt-4o":
		return "gpt-5.4-mini"
	default:
		return "gpt-5.5"
	}
}

// Create sends a Responses API body and returns the aggregated final response.
func (c *Client) Create(ctx context.Context, body map[string]any) (map[string]any, error) {
	codexBody := c.coerceBody(body)
	res, err := c.fetch(ctx, codexBody)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return aggregateStream(res.Body)
}

func (c *Client) createLocalToolRound(ctx context.Context, body map[string]any) (map[string]any, error) {
	return c.createLocalToolRoundWithTrace(ctx, body, localToolRoundTrace{})
}

type localToolRoundTrace struct {
	OnResponseHeaders func()
	OnFirstEvent      func()
	OnEvent           func(codexStreamEvent)
}

func (c *Client) createLocalToolRoundWithTrace(ctx context.Context, body map[string]any, trace localToolRoundTrace) (map[string]any, error) {
	codexBody := c.coerceBody(body)
	res, err := c.fetch(ctx, codexBody)
	if err != nil {
		return nil, err
	}
	if trace.OnResponseHeaders != nil {
		trace.OnResponseHeaders()
	}
	defer res.Body.Close()
	return aggregateStreamUntilFunctionCallWithTrace(res.Body, trace)
}

func (c *Client) closeIdleConnections() {
	if c == nil || c.httpClient == nil {
		return
	}
	c.httpClient.CloseIdleConnections()
}

func (c *Client) coerceBody(params map[string]any) map[string]any {
	body := make(map[string]any, len(params)+4)
	for key, value := range params {
		body[key] = value
	}
	body["stream"] = true
	body["store"] = false
	delete(body, "max_output_tokens")
	delete(body, "max_completion_tokens")
	delete(body, "top_p")
	delete(body, "presence_penalty")
	delete(body, "frequency_penalty")
	delete(body, "temperature")

	if c.modelOverride != "" {
		body["model"] = c.modelOverride
	} else if model, _ := body["model"].(string); model != "" {
		body["model"] = ResolveModel(model)
	} else {
		body["model"] = ResolveModel("")
	}

	existing, _ := body["instructions"].(string)
	if existing != "" {
		body["instructions"] = c.systemPrompt + "\n\n" + existing
	} else {
		body["instructions"] = c.systemPrompt
	}
	return body
}

func (c *Client) fetch(ctx context.Context, body map[string]any) (*http.Response, error) {
	auth, err := c.loadAuth(ctx)
	if err != nil {
		return nil, err
	}
	res, err := c.post(ctx, body, auth)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusUnauthorized {
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			defer res.Body.Close()
			return nil, backendError(res)
		}
		return res, nil
	}

	res.Body.Close()
	if auth.refreshToken == "" {
		return nil, errors.New("codex backend returned 401 and ~/.codex/auth.json has no refresh token")
	}
	refreshed, refreshErr := c.refreshAuth(ctx, auth.bundle)
	if refreshErr != nil {
		return nil, fmt.Errorf("codex backend returned 401 and OAuth refresh failed: %w", refreshErr)
	}
	res, err = c.post(ctx, body, refreshed)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		return nil, backendError(res)
	}
	return res, nil
}

func (c *Client) post(ctx context.Context, body map[string]any, auth *authState) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.accessToken)
	req.Header.Set("chatgpt-account-id", auth.accountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", c.originator)
	req.Header.Set("session_id", c.sessionID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Close = true
	return c.httpClient.Do(req)
}

func (c *Client) loadAuth(ctx context.Context) (*authState, error) {
	raw, err := os.ReadFile(c.authPath)
	if err != nil {
		return nil, fmt.Errorf("read Codex auth file %s: %w", c.authPath, err)
	}
	auth, err := parseAuth(raw)
	if err != nil {
		return nil, err
	}
	if expiry, ok := jwtExpiry(auth.accessToken); ok && time.Until(expiry) < 2*time.Minute && auth.refreshToken != "" {
		return c.refreshAuth(ctx, auth.bundle)
	}
	return auth, nil
}

func parseAuth(raw []byte) (*authState, error) {
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, fmt.Errorf("parse Codex auth JSON: %w", err)
	}
	auth := &authState{
		bundle:       bundle,
		accessToken:  tokenString(bundle, "access_token"),
		refreshToken: tokenString(bundle, "refresh_token"),
		accountID:    tokenString(bundle, "account_id"),
	}
	if auth.accessToken == "" || auth.accountID == "" {
		return nil, errors.New("Codex auth file is missing tokens.access_token or tokens.account_id")
	}
	return auth, nil
}

func tokenString(bundle map[string]any, key string) string {
	tokens, _ := bundle["tokens"].(map[string]any)
	value, _ := tokens[key].(string)
	return value
}

func setToken(bundle map[string]any, key string, value string) {
	if value == "" {
		return
	}
	tokens, _ := bundle["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
		bundle["tokens"] = tokens
	}
	tokens[key] = value
}

func (c *Client) refreshAuth(ctx context.Context, bundle map[string]any) (*authState, error) {
	refreshToken := tokenString(bundle, "refresh_token")
	if refreshToken == "" {
		return nil, errors.New("Codex auth file has no tokens.refresh_token")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", oauthClientID)
	form.Set("scope", "openid profile email")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthRefreshURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Close = true

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("OAuth refresh returned %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var fresh map[string]any
	if err := json.NewDecoder(res.Body).Decode(&fresh); err != nil {
		return nil, fmt.Errorf("decode OAuth refresh response: %w", err)
	}
	for _, key := range []string{"access_token", "id_token", "refresh_token"} {
		if value, _ := fresh[key].(string); value != "" {
			setToken(bundle, key, value)
		}
	}
	bundle["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	if err := writeAuthBundle(c.authPath, bundle); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	return parseAuth(raw)
}

func writeAuthBundle(path string, bundle map[string]any) error {
	payload, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

func backendError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	return fmt.Errorf("codex backend returned %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
}

func aggregateStream(r io.Reader) (map[string]any, error) {
	return aggregateStreamWithOptions(r, streamAggregateOptions{})
}

func aggregateStreamUntilFunctionCall(r io.Reader) (map[string]any, error) {
	return aggregateStreamUntilFunctionCallWithTrace(r, localToolRoundTrace{})
}

func aggregateStreamUntilFunctionCallWithTrace(r io.Reader, trace localToolRoundTrace) (map[string]any, error) {
	return aggregateStreamWithOptions(r, streamAggregateOptions{ReturnOnFunctionCall: true, OnFirstEvent: trace.OnFirstEvent, OnEvent: trace.OnEvent})
}

type streamAggregateOptions struct {
	ReturnOnFunctionCall bool
	OnFirstEvent         func()
	OnEvent              func(codexStreamEvent)
}

type codexStreamEvent struct {
	Type        string
	OutputIndex int
	ItemType    string
	ToolName    string
	TextPreview string
	TextBytes   int
}

func aggregateStreamWithOptions(r io.Reader, opts streamAggregateOptions) (map[string]any, error) {
	reader := bufio.NewReader(r)
	outputItems := map[int]any{}
	var final map[string]any
	var frame strings.Builder
	firstEventSeen := false

	process := func(raw string) (bool, error) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return false, nil
		}
		data := ""
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "data:") {
				data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if data == "" || data == "[DONE]" {
			return false, nil
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return false, nil
		}
		if !firstEventSeen {
			firstEventSeen = true
			if opts.OnFirstEvent != nil {
				opts.OnFirstEvent()
			}
		}
		if opts.OnEvent != nil {
			opts.OnEvent(summarizeCodexStreamEvent(event))
		}
		eventType, _ := event["type"].(string)
		switch eventType {
		case "response.output_item.done":
			index, ok := numberAsInt(event["output_index"])
			if !ok {
				return false, nil
			}
			if item, ok := event["item"].(map[string]any); ok {
				outputItems[index] = item
				if opts.ReturnOnFunctionCall && item["type"] == "function_call" {
					if final == nil {
						final = map[string]any{}
					}
					return true, nil
				}
			}
		case "response.completed":
			if response, ok := event["response"].(map[string]any); ok {
				final = response
			}
		case "response.failed":
			return false, fmt.Errorf("codex response failed: %v", event["response"])
		}
		return false, nil
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				done, err := process(frame.String())
				if err != nil {
					return nil, err
				}
				frame.Reset()
				if done {
					break
				}
			} else {
				frame.WriteString(trimmed)
				frame.WriteByte('\n')
			}
		}
		if errors.Is(err, io.EOF) {
			if frame.Len() > 0 {
				done, err := process(frame.String())
				if err != nil {
					return nil, err
				}
				if done {
					break
				}
			}
			break
		}
		if err != nil {
			return nil, err
		}
	}

	if final == nil {
		return nil, errors.New("codex stream ended without response.completed")
	}
	if len(outputItems) > 0 {
		indexes := make([]int, 0, len(outputItems))
		for index := range outputItems {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		output := make([]any, 0, len(indexes))
		for _, index := range indexes {
			output = append(output, outputItems[index])
		}
		final["output"] = output
	}
	final["output_text"] = outputText(final["output"])
	return final, nil
}

func summarizeCodexStreamEvent(event map[string]any) codexStreamEvent {
	summary := codexStreamEvent{OutputIndex: -1}
	summary.Type, _ = event["type"].(string)
	if index, ok := numberAsInt(event["output_index"]); ok {
		summary.OutputIndex = index
	}
	if item, ok := event["item"].(map[string]any); ok {
		summary.ItemType, _ = item["type"].(string)
		summary.ToolName, _ = item["name"].(string)
		if summary.ItemType == "message" {
			text := outputText([]any{item})
			summary.TextBytes = len(text)
			summary.TextPreview = truncateStreamPreview(text)
		}
	}
	if delta, ok := event["delta"].(string); ok {
		summary.TextBytes = len(delta)
		summary.TextPreview = truncateStreamPreview(delta)
	}
	if summary.ItemType == "" {
		if itemType, ok := event["item_type"].(string); ok {
			summary.ItemType = itemType
		}
	}
	return summary
}

func truncateStreamPreview(text string) string {
	text = strings.TrimSpace(text)
	const limit = 240
	if len(text) <= limit {
		return text
	}
	return text[:limit-3] + "..."
}

func outputText(output any) string {
	items, ok := output.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok || item["type"] != "message" {
			continue
		}
		parts, _ := item["content"].([]any)
		for _, rawPart := range parts {
			part, ok := rawPart.(map[string]any)
			if !ok || part["type"] != "output_text" {
				continue
			}
			if text, _ := part["text"].(string); text != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

func numberAsInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	default:
		return 0, false
	}
}

func randomSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
