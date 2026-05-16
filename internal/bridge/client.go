package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/scene"
)

type SceneContextRequest struct {
	WorkspaceID      string   `json:"workspace_id"`
	SessionID        string   `json:"session_id"`
	PlayerPeerID     string   `json:"player_peer_id"`
	RecentSceneCount int      `json:"recent_scene_count"`
	Representations  []string `json:"representations,omitempty"`
	SearchQuery      string   `json:"search_query,omitempty"`
}

type SceneContextResponse struct {
	WorkspaceID     string              `json:"workspace_id"`
	SessionID       string              `json:"session_id"`
	RecentScenes    []string            `json:"recent_scenes"`
	Representations map[string]string   `json:"representations"`
	PeerCards       map[string][]string `json:"peer_cards,omitempty"`
}

type PeerCardPatch struct {
	Facts   []string `json:"facts"`
	Replace bool     `json:"replace"`
	Target  string   `json:"target,omitempty"`
}

type RepresentationRequest struct {
	Observer    string `json:"observer"`
	Observed    string `json:"observed"`
	SearchQuery string `json:"search_query,omitempty"`
	TopK        int    `json:"top_k,omitempty"`
}

type RepresentationResponse struct {
	Observer       string `json:"observer"`
	Observed       string `json:"observed"`
	Representation string `json:"representation"`
}

type LoreSearchRequest struct {
	Query       string   `json:"query"`
	K           int      `json:"k,omitempty"`
	Kind        []string `json:"kind,omitempty"`
	PeerID      []string `json:"peer_id,omitempty"`
	CharacterID []string `json:"character_id,omitempty"`
	Volume      []int    `json:"volume,omitempty"`
}

type LoreSnippet struct {
	DocID       string         `json:"doc_id"`
	Text        string         `json:"text"`
	SourcePath  string         `json:"source_path"`
	Score       float64        `json:"score"`
	Kind        string         `json:"kind"`
	PageID      string         `json:"page_id,omitempty"`
	PeerID      string         `json:"peer_id,omitempty"`
	CharacterID string         `json:"character_id,omitempty"`
	Volume      *int           `json:"volume,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

type LoreSearchResponse struct {
	Snippets []LoreSnippet `json:"snippets"`
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func NewClient(baseURL, token string) Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8787"
	}
	return Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c Client) Health(ctx context.Context) error {
	var out map[string]string
	return c.do(ctx, http.MethodGet, "/v1/health", nil, &out)
}

func (c Client) SceneContext(ctx context.Context, req SceneContextRequest) (SceneContextResponse, error) {
	var out SceneContextResponse
	err := c.do(ctx, http.MethodPost, "/v1/scene/context", req, &out)
	return out, err
}

func (c Client) CommitScene(ctx context.Context, commit scene.Commit) error {
	if err := commit.ValidateCanonOnly(); err != nil {
		return err
	}
	var out map[string]string
	return c.do(ctx, http.MethodPost, "/v1/scene/commit", commit, &out)
}

func (c Client) LoreSearch(ctx context.Context, req LoreSearchRequest) (LoreSearchResponse, error) {
	var out LoreSearchResponse
	err := c.do(ctx, http.MethodPost, "/v1/lore/search", req, &out)
	return out, err
}

func (c Client) do(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(in); err != nil {
			return err
		}
		body = &buf
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("sidecar %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
