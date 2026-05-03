// Package council implements the Council of the Castaka — a deliberation
// feature where Metabaron characters debate matters posed by the user.
//
// Each council member runs as a parallel `claude -p` CLI invocation with
// an assembled prompt: character card + directional relationship roster +
// invocation template + the user's matter.
package council

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CouncilRequest describes what the user wants the council to deliberate.
type CouncilRequest struct {
	Matter       string
	PeerPressure bool
	Scrutiny     bool // run an extra re-evaluation round where each member hears all others
}

// MemberStatement holds one council member's public statement.
type MemberStatement struct {
	Name      string // display name (e.g. "Honorata — La Trisaïeule")
	Key       string // short key (e.g. "honorata")
	Statement string
}

// PrivateExchange records a private message between two members (peer pressure mode).
type PrivateExchange struct {
	FromKey string // short key (e.g. "honorata")
	From    string // display name
	To      string // display name
	ToKey   string // short key (e.g. "honorata")
	Message string
	Reply   string // empty = silence
	Silent  bool
}

// CouncilResult holds the complete council output.
type CouncilResult struct {
	Members   []MemberStatement
	Exchanges []PrivateExchange // peer pressure only
	Scrutiny  []MemberStatement // re-evaluation round: each member hears all others first
}

// SaveMarkdown writes the council result to a markdown file in outDir.
// Returns the file path written.
func (r *CouncilResult) SaveMarkdown(outDir string, matter string) (string, error) {
	dir := outDir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create council dir: %w", err)
	}

	ts := time.Now().Format("2006-01-02-150405")
	filename := filepath.Join(dir, ts+".md")

	var b strings.Builder

	// Title page.
	b.WriteString("# Council of the Castaka\n\n")
	b.WriteString(fmt.Sprintf("*%s*\n\n", time.Now().Format("2 January 2006, 15:04")))
	b.WriteString("---\n\n")

	// The matter.
	b.WriteString("## The Matter\n\n")
	b.WriteString(fmt.Sprintf("> %s\n\n", matter))

	// Members summoned.
	b.WriteString("## Members Summoned\n\n")
	for _, m := range r.Members {
		b.WriteString(fmt.Sprintf("- %s\n", m.Name))
	}
	b.WriteString("\n---\n\n")

	// Each member's statement.
	// for _, m := range r.Members {
	// 	b.WriteString(fmt.Sprintf("## %s\n\n", m.Name))
	// 	b.WriteString(m.Statement)
	// 	b.WriteString("\n\n---\n\n")
	// }

	// Scrutiny round (re-evaluation).
	if len(r.Scrutiny) > 0 {
		b.WriteString("## The Final Word *(after hearing all others)*\n\n")
		for _, m := range r.Scrutiny {
			b.WriteString(fmt.Sprintf("## %s\n\n", m.Name))
			// Private exchanges (peer pressure).
			if len(r.Exchanges) > 0 {
				messageReceived := []PrivateExchange{}
				for _, ex := range r.Exchanges {
					if ex.ToKey == m.Key {
						messageReceived = append(messageReceived, ex)
					}
				}
				if len(messageReceived) > 0 {
					for _, ex := range messageReceived {
						b.WriteString("\n\n---\n")
						b.WriteString(fmt.Sprintf("**Message from %s:**\n\n", ex.From))
						b.WriteString(fmt.Sprintf("> %s\n\n", ex.Message))
						// b.WriteString("**Replied:**\n")
						// if messageReceived[0].Silent {
						// 	b.WriteString("*[silence]*\n\n")
						// } else if messageReceived[0].Reply != "" {
						// 	b.WriteString(fmt.Sprintf("%s\n\n", messageReceived[0].Reply))
						// } else {
						// 	b.WriteString("*[silence]*\n")
						// }
					}
					b.WriteString("---\n\n")
				}
			}
			b.WriteString(m.Statement)
			b.WriteString("\n\n---\n\n")
		}
	}

	b.WriteString("*The council does not reconvene on the same matter. What is decided here stands.*\n")

	if err := os.WriteFile(filename, []byte(b.String()), 0644); err != nil {
		return "", err
	}
	return filename, nil
}

// SaveMarkdownFinal writes a slim report containing only the summoning header and
// final answers (scrutiny round, or members if scrutiny is absent).
// Used by the standalone binary. The full log is written by SaveMarkdown.
func (r *CouncilResult) SaveMarkdownFinal(outDir string, matter string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("create council dir: %w", err)
	}

	ts := time.Now().Format("2006-01-02-150405")
	filename := filepath.Join(outDir, ts+".md")

	var b strings.Builder

	// Title page.
	b.WriteString("# Council of the Castaka\n\n")
	b.WriteString(fmt.Sprintf("*%s*\n\n", time.Now().Format("2 January 2006, 15:04")))
	b.WriteString("---\n\n")

	// The matter.
	b.WriteString("## The Matter\n\n")
	b.WriteString(fmt.Sprintf("> %s\n\n", matter))

	// Members summoned.
	b.WriteString("## Members Summoned\n\n")
	for _, m := range r.Members {
		b.WriteString(fmt.Sprintf("- %s\n", m.Name))
	}
	b.WriteString("\n---\n\n")

	// Final statements only: scrutiny if present, else members.
	final := r.Scrutiny
	if len(final) == 0 {
		final = r.Members
	}
	for _, m := range final {
		b.WriteString(fmt.Sprintf("## %s\n\n", m.Name))
		b.WriteString(m.Statement)
		b.WriteString("\n\n---\n\n")
	}

	b.WriteString("*The council does not reconvene on the same matter. What is decided here stands.*\n")

	if err := os.WriteFile(filename, []byte(b.String()), 0644); err != nil {
		return "", err
	}
	return filename, nil
}

// SelectMembers randomly picks 2-8 members from the full roster.
func SelectMembers() []MemberDef {
	all := AllMembers()

	n := cryptoRandInt(2, len(all)+1) // [2, 8]
	shuffled := make([]MemberDef, len(all))
	copy(shuffled, all)
	// Fisher-Yates shuffle with crypto/rand.
	for i := len(shuffled) - 1; i > 0; i-- {
		j := cryptoRandInt(0, i+1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled[:n]
}

// Invoke runs the council deliberation and returns the result.
func Invoke(ctx context.Context, req CouncilRequest, repoPath string, logger *slog.Logger) (*CouncilResult, error) {
	if !claudeAvailable() {
		return nil, fmt.Errorf("claude CLI not found in PATH — the council requires it")
	}

	selected := SelectMembers()
	logger.Info("council summoned", "members", len(selected), "peer_pressure", req.PeerPressure)
	for _, m := range selected {
		logger.Info("council member", "name", m.DisplayName)
	}

	// Pick Steelhead phase if he's in the council.
	phases := []string{"early", "Melmoth", "late"}
	steelheadPhase := phases[cryptoRandInt(0, len(phases))]

	var result *CouncilResult
	var err error
	if req.PeerPressure {
		result, err = invokePeerPressure(ctx, selected, req.Matter, steelheadPhase, repoPath, logger)
	} else {
		result, err = invokeOneShot(ctx, selected, req.Matter, steelheadPhase, repoPath, logger)
	}
	if err != nil || !req.Scrutiny {
		return result, err
	}

	logger.Info("council scrutiny round starting", "members", len(selected))
	scrutiny, err := invokeScrutiny(ctx, selected, req.Matter, steelheadPhase, result.Members, repoPath, logger)
	if err != nil {
		logger.Warn("council scrutiny round failed", "error", err)
		return result, nil // return partial result without scrutiny
	}
	result.Scrutiny = scrutiny
	return result, nil
}

func invokeOneShot(ctx context.Context, selected []MemberDef, matter string, steelheadPhase string, repoPath string, logger *slog.Logger) (*CouncilResult, error) {
	type indexedResult struct {
		idx       int
		statement string
		err       error
	}

	results := make([]indexedResult, len(selected))
	var wg sync.WaitGroup
	wg.Add(len(selected))

	for i, member := range selected {
		go func(idx int, m MemberDef) {
			defer wg.Done()
			prompt := assembleOneShot(m, selected, matter, steelheadPhase)
			model := modelForMember(m)
			logger.Info("council member speaking", "name", m.DisplayName, "model", model)
			out, err := runClaude(ctx, prompt, model, repoPath, "medium")
			results[idx] = indexedResult{idx: idx, statement: out, err: err}
		}(i, member)
	}

	wg.Wait()

	var statements []MemberStatement
	for i, r := range results {
		if r.err != nil {
			logger.Warn("council member failed", "name", selected[i].DisplayName, "error", r.err)
			statements = append(statements, MemberStatement{
				Name:      selected[i].DisplayName,
				Key:       selected[i].Key,
				Statement: fmt.Sprintf("[failed to respond: %v]", r.err),
			})
			continue
		}
		statements = append(statements, MemberStatement{
			Name:      selected[i].DisplayName,
			Key:       selected[i].Key,
			Statement: r.statement,
		})
	}

	return &CouncilResult{Members: statements}, nil
}

func invokePeerPressure(ctx context.Context, selected []MemberDef, matter string, steelheadPhase string, repoPath string, logger *slog.Logger) (*CouncilResult, error) {
	// Build a set of selected keys for silent message detection.
	selectedKeys := make(map[string]bool, len(selected))
	for _, m := range selected {
		selectedKeys[m.Key] = true
	}

	// ── Round 1: private messages + initial public statements ──
	type round1Result struct {
		statement  string
		targetKey  string
		privateMsg string
		err        error
	}

	r1 := make([]round1Result, len(selected))
	var wg1 sync.WaitGroup
	wg1.Add(len(selected))

	for i, member := range selected {
		go func(idx int, m MemberDef) {
			defer wg1.Done()
			prompt := assembleRound1(m, selected, matter, steelheadPhase)
			model := modelForMember(m)
			out, err := runClaude(ctx, prompt, model, repoPath, "low")
			if err != nil {
				r1[idx] = round1Result{err: err}
				return
			}
			targetKey, privateMsg, pubStatement := parseRound1Output(out)
			r1[idx] = round1Result{
				statement:  pubStatement,
				targetKey:  targetKey,
				privateMsg: privateMsg,
			}
		}(i, member)
	}
	wg1.Wait()

	// Collect private messages: who sent what to whom.
	var exchanges []PrivateExchange
	// Map: recipientKey → list of messages received.
	received := make(map[string][]PrivateExchange)

	for i, res := range r1 {
		if res.err != nil || res.targetKey == "" {
			continue
		}
		fromMember := selected[i]
		// Resolve target display name.
		toName := res.targetKey
		for _, m := range AllMembers() {
			if m.Key == res.targetKey {
				toName = m.DisplayName
				break
			}
		}

		ex := PrivateExchange{
			FromKey: fromMember.Key,
			From:    fromMember.DisplayName,
			ToKey:   res.targetKey,
			To:      toName,
			Message: res.privateMsg,
		}

		// Silent message: Sans-Nom → Aghora (not in council), Aghnar → Oda (not in council).
		if !selectedKeys[res.targetKey] {
			ex.Silent = true
			ex.Reply = ""
		}

		exchanges = append(exchanges, ex)
		if !ex.Silent || ex.Reply != "" {
			received[res.targetKey] = append(received[res.targetKey], ex)
		}
	}

	// ── Round 2: deliver private messages, get final public statements ──
	r2 := make([]string, len(selected))
	var wg2 sync.WaitGroup
	wg2.Add(len(selected))

	for i, member := range selected {
		go func(idx int, m MemberDef) {
			defer wg2.Done()
			if r1[idx].err != nil {
				return
			}

			// What this member sent.
			var sentTo, sentMsg, sentReply string
			if r1[idx].targetKey != "" {
				toName := r1[idx].targetKey
				for _, all := range AllMembers() {
					if all.Key == r1[idx].targetKey {
						toName = all.DisplayName
						break
					}
				}
				sentTo = toName
				sentMsg = r1[idx].privateMsg
				if selectedKeys[r1[idx].targetKey] {
					sentReply = "[they will address the council shortly]"
				} else {
					sentReply = "[silence]"
				}
			}

			rcvd := received[m.Key]
			prompt := assembleRound2(m, selected, matter, steelheadPhase, sentTo, sentMsg, sentReply, rcvd)
			model := modelForMember(m)
			out, err := runClaude(ctx, prompt, model, repoPath, "low")
			if err != nil {
				r2[idx] = r1[idx].statement // fall back to round 1
				return
			}
			r2[idx] = out
		}(i, member)
	}
	wg2.Wait()

	// Build final statements.
	var statements []MemberStatement
	for i, member := range selected {
		stmt := r2[i]
		if stmt == "" {
			stmt = r1[i].statement // fallback
		}
		if r1[i].err != nil {
			stmt = fmt.Sprintf("[failed to respond: %v]", r1[i].err)
		}
		statements = append(statements, MemberStatement{
			Name:      member.DisplayName,
			Key:       member.Key,
			Statement: stmt,
		})
	}

	return &CouncilResult{
		Members:   statements,
		Exchanges: exchanges,
	}, nil
}

func invokeScrutiny(ctx context.Context, selected []MemberDef, matter string, steelheadPhase string, statements []MemberStatement, repoPath string, logger *slog.Logger) ([]MemberStatement, error) {
	type indexedResult struct {
		idx       int
		statement string
		err       error
	}

	results := make([]indexedResult, len(selected))
	var wg sync.WaitGroup
	wg.Add(len(selected))

	for i, member := range selected {
		go func(idx int, m MemberDef) {
			defer wg.Done()
			// Collect all other members' statements.
			var others []MemberStatement
			for _, s := range statements {
				if s.Key != m.Key {
					others = append(others, s)
				}
			}
			prompt := assembleScrutiny(m, selected, matter, steelheadPhase, others)
			model := modelForMember(m)
			logger.Info("council scrutiny", "name", m.DisplayName, "model", model)
			out, err := runClaude(ctx, prompt, model, repoPath, "medium")
			results[idx] = indexedResult{idx: idx, statement: out, err: err}
		}(i, member)
	}

	wg.Wait()

	var scrutiny []MemberStatement
	for i, r := range results {
		if r.err != nil {
			logger.Warn("scrutiny member failed", "name", selected[i].DisplayName, "error", r.err)
			scrutiny = append(scrutiny, MemberStatement{
				Name:      selected[i].DisplayName,
				Key:       selected[i].Key,
				Statement: fmt.Sprintf("[failed to respond: %v]", r.err),
			})
			continue
		}
		scrutiny = append(scrutiny, MemberStatement{
			Name:      selected[i].DisplayName,
			Key:       selected[i].Key,
			Statement: r.statement,
		})
	}

	return scrutiny, nil
}

func modelForMember(m MemberDef) string {
	if m.Key == "sans-nom" {
		return "opus"
	}
	return "sonnet"
}

// cryptoRandInt returns a random int in [min, max).
func cryptoRandInt(min, max int) int {
	if max <= min {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	if err != nil {
		return min // fallback
	}
	return min + int(n.Int64())
}
