package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	WorkspaceID string `json:"workspace_id"`
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
	Collection  string   `json:"collection,omitempty"`
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

type StoryIndexDocument struct {
	DocID        string         `json:"doc_id"`
	PointID      string         `json:"point_id"`
	Kind         string         `json:"kind"`
	SourcePath   string         `json:"source_path"`
	SourceSHA256 string         `json:"source_sha256"`
	TextSHA256   string         `json:"text_sha256"`
	Text         string         `json:"text"`
	StoryID      string         `json:"story_id,omitempty"`
	PacketID     string         `json:"packet_id,omitempty"`
	NodeID       string         `json:"node_id,omitempty"`
	Turn         int            `json:"turn,omitempty"`
	Heading      string         `json:"heading,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

type StoryIndexIngestRequest struct {
	Collection     string               `json:"collection"`
	Documents      []StoryIndexDocument `json:"documents"`
	Replace        bool                 `json:"replace,omitempty"`
	FakeEmbeddings bool                 `json:"fake_embeddings,omitempty"`
}

type StoryIndexIngestResponse struct {
	Collection string `json:"collection"`
	Count      int    `json:"count"`
	Replaced   bool   `json:"replaced"`
}

type PeerCardResponse struct {
	PeerID string   `json:"peer_id"`
	Target string   `json:"target,omitempty"`
	Facts  []string `json:"facts"`
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
	var out map[string]any
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

func (c Client) Representation(ctx context.Context, req RepresentationRequest) (RepresentationResponse, error) {
	var out RepresentationResponse
	err := c.do(ctx, http.MethodPost, "/v1/repr", req, &out)
	return out, err
}

func (c Client) GetCard(ctx context.Context, workspaceID, peerID, target string) ([]string, error) {
	values := url.Values{}
	values.Set("workspace_id", workspaceID)
	if target != "" {
		values.Set("target", target)
	}
	path := "/v1/peer/" + url.PathEscape(peerID) + "/card?" + values.Encode()
	var out PeerCardResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Facts, nil
}

func (c Client) PatchCard(ctx context.Context, workspaceID, peerID string, patch PeerCardPatch) ([]string, error) {
	values := url.Values{}
	values.Set("workspace_id", workspaceID)
	path := "/v1/peer/" + url.PathEscape(peerID) + "/card?" + values.Encode()
	var out PeerCardResponse
	if err := c.do(ctx, http.MethodPatch, path, patch, &out); err != nil {
		return nil, err
	}
	return out.Facts, nil
}

func (c Client) IngestStoryIndex(ctx context.Context, req StoryIndexIngestRequest) (StoryIndexIngestResponse, error) {
	var out StoryIndexIngestResponse
	err := c.do(ctx, http.MethodPost, "/v1/story/index/ingest", req, &out)
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
