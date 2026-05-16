package contextbundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rawpixel-vincent/sans-nom/internal/actors"
)

type Card struct {
	ActorID string `json:"actor_id"`
	Text    string `json:"text"`
	Source  string `json:"source"`
}

type LocalCardStore struct {
	cards map[string]Card
}

func LoadLocalCardStore(actorDir string, peerCardDir string) (*LocalCardStore, error) {
	store := &LocalCardStore{cards: map[string]Card{}}
	if err := store.loadPeerCards(peerCardDir); err != nil {
		return nil, err
	}
	if err := store.loadMarkdownCards(actorDir); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *LocalCardStore) LocalCard(actorID string) (Card, bool) {
	if s == nil {
		return Card{}, false
	}
	card, ok := s.cards[actorID]
	return card, ok
}

func (s *LocalCardStore) ActorIDs() []string {
	if s == nil {
		return nil
	}
	ids := make([]string, 0, len(s.cards))
	for id := range s.cards {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (s *LocalCardStore) Registry() (*actors.Registry, error) {
	var loaded []actors.Actor
	for _, id := range s.ActorIDs() {
		card, _ := s.LocalCard(id)
		loaded = append(loaded, actors.Actor{
			ID:          id,
			DisplayName: id,
			Card:        card.Text,
		})
	}
	return actors.NewRegistry(loaded)
}

func (s *LocalCardStore) loadPeerCards(dir string) error {
	if dir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var artifact struct {
			PeerCardID  string   `json:"peer_card_id"`
			DisplayName string   `json:"display_name"`
			Card        []string `json:"card"`
		}
		if err := json.Unmarshal(data, &artifact); err != nil {
			return fmt.Errorf("parse peer card %s: %w", file, err)
		}
		id := strings.TrimSpace(artifact.PeerCardID)
		if id == "" {
			id = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		facts := trimFacts(artifact.Card, 40)
		if len(facts) == 0 {
			continue
		}
		s.cards[id] = Card{
			ActorID: id,
			Text:    strings.Join(facts, "\n"),
			Source:  file,
		}
	}
	return nil
}

func (s *LocalCardStore) loadMarkdownCards(dir string) error {
	if dir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		id := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		if _, exists := s.cards[id]; exists {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		lines := trimFacts(strings.Split(string(data), "\n"), 40)
		if len(lines) == 0 {
			continue
		}
		s.cards[id] = Card{
			ActorID: id,
			Text:    strings.Join(lines, "\n"),
			Source:  file,
		}
	}
	return nil
}

func trimFacts(values []string, max int) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}
