package telemetry

import (
	"context"
	"log/slog"
	"time"
)

type Event struct {
	Name   string
	Fields map[string]any
}

type Sink interface {
	Emit(ctx context.Context, event Event)
}

type LoggerSink struct {
	Logger *slog.Logger
}

func (s LoggerSink) Emit(_ context.Context, event Event) {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	args := make([]any, 0, len(event.Fields)*2)
	for k, v := range event.Fields {
		args = append(args, k, v)
	}
	logger.Info(event.Name, args...)
}

type TurnMetrics struct {
	TurnID           string
	WorkspaceID      string
	SessionID        string
	ActorsInvoked    int
	TokenUsageTotal  int
	LatencyTotal     time.Duration
	LoreHitCount     int
	CardUpdateEvents int
}

type LoreSearchMetrics struct {
	QuerySHA256 string
	K           int
	Latency     time.Duration
	HitCount    int
	TopScore    float64
	Miss        bool
	Filters     map[string]any
	Error       string
}

func EmitTurn(ctx context.Context, sink Sink, m TurnMetrics) {
	if sink == nil {
		return
	}
	sink.Emit(ctx, Event{
		Name: "turn.completed",
		Fields: map[string]any{
			"turn_id":            m.TurnID,
			"workspace_id":       m.WorkspaceID,
			"session_id":         m.SessionID,
			"actors_invoked":     m.ActorsInvoked,
			"tokens_total":       m.TokenUsageTotal,
			"latency_ms_total":   m.LatencyTotal.Milliseconds(),
			"lore_hit_count":     m.LoreHitCount,
			"card_update_events": m.CardUpdateEvents,
		},
	})
}

func EmitLoreSearch(ctx context.Context, sink Sink, m LoreSearchMetrics) {
	if sink == nil {
		return
	}
	fields := map[string]any{
		"query_sha256": m.QuerySHA256,
		"k":            m.K,
		"latency_ms":   m.Latency.Milliseconds(),
		"hit_count":    m.HitCount,
		"top_score":    m.TopScore,
		"miss":         m.Miss,
	}
	for key, value := range m.Filters {
		fields["filter_"+key] = value
	}
	if m.Error != "" {
		fields["error"] = m.Error
	}
	sink.Emit(ctx, Event{Name: "lore.search", Fields: fields})
}
