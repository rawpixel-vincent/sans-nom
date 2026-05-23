package codexoauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"mvdan.cc/sh/v3/syntax"
)

const (
	codexMinCallIntervalEnv     = "SANS_NOM_CODEX_MIN_CALL_INTERVAL_MS"
	defaultMaxToolRounds        = 24
	defaultMinCodexCallInterval = 10 * time.Second
)

var readOnlyCommands = map[string]bool{
	"cat":  true,
	"find": true,
	"grep": true,
	"head": true,
	"jq":   true,
	"ls":   true,
	"nl":   true,
	"pwd":  true,
	"rg":   true,
	"sed":  true,
	"sort": true,
	"tail": true,
	"uniq": true,
	"wc":   true,
}

// LocalToolOptions enables a minimal local shell-like tool loop for models
// called directly through the Codex backend.
type LocalToolOptions struct {
	Root            string
	MaxRounds       int
	MinCallInterval time.Duration
	ExtraTools      []LocalTool
	Logger          *gamelog.Writer
	VisiblePaths    []string

	ToolCategory          string
	CallerRole            string
	CallerSpeaker         string
	StoryID               string
	Turn                  int
	PacketID              string
	NodeID                string
	RunID                 string
	ModelMetadata         modelmeta.Metadata
	ContextBudget         map[string]any
	RequireToolUse        bool
	ToolUseRequiredReason string
}

type LocalTool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Handler     func(ctx context.Context, root string, arguments string) (any, error)
}

type functionCall struct {
	CallID    string
	Name      string
	Arguments string
}

// CreateWithLocalTools runs Responses calls until the model stops requesting
// local tools. The exposed tool is intentionally shaped like Codex's shell
// command surface, but enforced read-only and rooted to opts.Root.
func (c *Client) CreateWithLocalTools(ctx context.Context, body map[string]any, opts LocalToolOptions) (map[string]any, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, err
	}
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = defaultMaxToolRounds
	}
	minCallInterval := localToolMinCallInterval(opts)

	visiblePaths := existingVisiblePaths(root, opts.VisiblePaths)
	current := cloneMap(body)
	current["tools"] = appendTools(current["tools"], localToolDefinitions(opts.ExtraTools, visiblePaths))
	extraTools := extraToolHandlers(opts.ExtraTools)
	opts.VisiblePaths = visiblePaths
	usedAnyTool := false
	missingToolRetrySent := false

	for round := 0; round < maxRounds; round++ {
		if err := waitBeforeLocalToolProviderRound(ctx, opts, round+1, maxRounds, minCallInterval); err != nil {
			logLocalToolProviderFailure(opts, round+1, maxRounds, err)
			return nil, err
		}
		logLocalToolProviderRequest(opts, round+1, maxRounds)
		response, err := c.createLocalToolRound(ctx, current)
		if err != nil {
			logLocalToolProviderFailure(opts, round+1, maxRounds, err)
			return nil, err
		}
		calls := collectFunctionCalls(response["output"])
		logLocalToolProviderResponse(opts, round+1, maxRounds, calls)
		if len(calls) == 0 {
			if opts.RequireToolUse && !usedAnyTool {
				if !missingToolRetrySent && round+1 < maxRounds {
					logLocalToolRequiredRetry(opts, round+1, maxRounds)
					input := inputItems(current["input"])
					input = append(input, map[string]any{
						"role":    "user",
						"content": requiredToolUseReminder(opts),
					})
					current["input"] = input
					missingToolRetrySent = true
					continue
				}
				err := errors.New("model returned final answer without required local tool call")
				logLocalToolProviderFailure(opts, round+1, maxRounds, err)
				return nil, err
			}
			return response, nil
		}
		usedAnyTool = true

		input := inputItems(current["input"])
		input = append(input, functionCallInputItems(calls)...)
		for _, call := range calls {
			logLocalToolUse(opts, call)
			result := executeLocalTool(ctx, root, call, extraTools)
			logLocalToolResult(opts, call, result)
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": call.CallID,
				"output":  result,
			})
		}
		current["input"] = input
	}
	return nil, fmt.Errorf("model exceeded %d local tool rounds", maxRounds)
}

func requiredToolUseReminder(opts LocalToolOptions) string {
	reason := strings.TrimSpace(opts.ToolUseRequiredReason)
	if reason == "" {
		reason = "This run is configured to require at least one local grounding tool call before final output."
	}
	return reason + " Use one narrow read with exec_command, preferably an exact staged file such as `cat story/current-node.json` when it exists, then return the final JSON."
}

func localToolMinCallInterval(opts LocalToolOptions) time.Duration {
	if opts.MinCallInterval > 0 {
		return opts.MinCallInterval
	}
	raw, ok := os.LookupEnv(codexMinCallIntervalEnv)
	if !ok {
		return defaultMinCodexCallInterval
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return defaultMinCodexCallInterval
	}
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func waitBeforeLocalToolProviderRound(ctx context.Context, opts LocalToolOptions, round int, maxRounds int, interval time.Duration) error {
	if round <= 1 || interval <= 0 {
		return nil
	}
	logLocalToolProviderWait(opts, round, maxRounds, interval)
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func localToolDefinitions(extra []LocalTool, visiblePaths []string) []any {
	tools := []any{
		map[string]any{
			"type":        "function",
			"name":        "exec_command",
			"description": localToolDescription(visiblePaths),
			"parameters": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"cmd": map[string]any{
						"type":        "string",
						"description": "Shell command to execute. Only read-only commands are allowed; writes, redirects, command substitution, and command chaining are rejected.",
					},
					"workdir": map[string]any{
						"type":        "string",
						"description": "Working directory under the scoped workspace root. Defaults to the workspace root.",
					},
					"yield_time_ms": map[string]any{
						"type":        "integer",
						"description": "Compatibility field with Codex shell calls. The helper runs the command to completion within a short timeout.",
					},
					"background_task": map[string]any{
						"type":        "boolean",
						"description": "Set true when the command intentionally starts or manages work that may outlive this tool call, so the host can track cleanup responsibility.",
					},
					"cleanup_hint": map[string]any{
						"type":        "string",
						"description": "Short optional cleanup note for background_task commands, such as the process name, port, or stop command.",
					},
					"max_output_tokens": map[string]any{
						"type":        "integer",
						"description": "Approximate output cap. Defaults to 12000 tokens.",
					},
				},
				"required": []string{"cmd"},
			},
		},
	}
	for _, tool := range extra {
		if strings.TrimSpace(tool.Name) == "" || tool.Handler == nil {
			continue
		}
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Parameters,
		})
	}
	return tools
}

func localToolDescription(visiblePaths []string) string {
	var b strings.Builder
	b.WriteString("Run a read-only shell command inside the scoped agent workspace.")
	if len(visiblePaths) == 0 {
		b.WriteString(" No top-level material paths are currently advertised; use pwd, ls, or find . to inspect the workspace before reading files.")
	} else {
		b.WriteString(" Currently advertised material paths: ")
		for i, path := range visiblePaths {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(pathForDisplay(path))
		}
		b.WriteString(". Read known paths directly; avoid broad inventory commands unless you need exact file names.")
	}
	b.WriteString(" Prefer rg, grep, sed, jq, find, cat, ls, head, tail, wc, sort, uniq, nl, and pwd.")
	return b.String()
}

func pathForDisplay(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	if path == "." || path == "" {
		return "./"
	}
	return strings.TrimSuffix(path, "/") + "/"
}

func existingVisiblePaths(root string, visiblePaths []string) []string {
	out := make([]string, 0, len(visiblePaths))
	seen := map[string]struct{}{}
	for _, path := range visiblePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(path))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			continue
		}
		abs := filepath.Join(root, clean)
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		rel := filepath.ToSlash(clean)
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	return out
}

func extraToolHandlers(extra []LocalTool) map[string]LocalTool {
	out := map[string]LocalTool{}
	for _, tool := range extra {
		if strings.TrimSpace(tool.Name) == "" || tool.Handler == nil {
			continue
		}
		out[tool.Name] = tool
	}
	return out
}

func appendTools(existing any, added []any) []any {
	tools := outputItems(existing)
	tools = append(tools, added...)
	return tools
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func inputItems(input any) []any {
	switch typed := input.(type) {
	case []any:
		return append([]any(nil), typed...)
	case string:
		return []any{map[string]any{"role": "user", "content": typed}}
	case nil:
		return nil
	default:
		return []any{typed}
	}
}

func outputItems(output any) []any {
	items, ok := output.([]any)
	if !ok {
		return nil
	}
	return append([]any(nil), items...)
}

func collectFunctionCalls(output any) []functionCall {
	var calls []functionCall
	for _, rawItem := range outputItems(output) {
		item, ok := rawItem.(map[string]any)
		if !ok || item["type"] != "function_call" {
			continue
		}
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		arguments, _ := item["arguments"].(string)
		if callID == "" || name == "" {
			continue
		}
		calls = append(calls, functionCall{CallID: callID, Name: name, Arguments: arguments})
	}
	return calls
}

func functionCallInputItems(calls []functionCall) []any {
	items := make([]any, 0, len(calls))
	for _, call := range calls {
		items = append(items, map[string]any{
			"type":      "function_call",
			"call_id":   call.CallID,
			"name":      call.Name,
			"arguments": call.Arguments,
		})
	}
	return items
}

func executeLocalTool(ctx context.Context, root string, call functionCall, extraTools map[string]LocalTool) string {
	var output any
	var err error
	switch call.Name {
	case "exec_command":
		output, err = toolExecCommand(ctx, root, call.Arguments)
	default:
		tool, ok := extraTools[call.Name]
		if !ok {
			err = fmt.Errorf("unknown tool %q", call.Name)
			break
		}
		output, err = tool.Handler(ctx, root, call.Arguments)
	}
	if err != nil {
		output = map[string]any{"ok": false, "error": err.Error()}
	}
	payload, marshalErr := json.Marshal(output)
	if marshalErr != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, marshalErr.Error())
	}
	return string(payload)
}

func logLocalToolProviderRequest(opts LocalToolOptions, round int, maxRounds int) {
	logLocalToolProviderEvent(opts, "request", "pending", "Codex provider request started", map[string]any{
		"round":           round,
		"max_rounds":      maxRounds,
		"available_tools": localToolNames(opts.ExtraTools),
	})
}

func logLocalToolProviderWait(opts LocalToolOptions, round int, maxRounds int, interval time.Duration) {
	logLocalToolProviderEvent(opts, "wait", "pending", fmt.Sprintf("Waiting %s before next Codex provider request", interval), map[string]any{
		"round":                round,
		"max_rounds":           maxRounds,
		"min_call_interval_ms": interval.Milliseconds(),
	})
}

func logLocalToolProviderResponse(opts LocalToolOptions, round int, maxRounds int, calls []functionCall) {
	logLocalToolProviderEvent(opts, "response", "completed", fmt.Sprintf("Codex provider response received with %d local tool call(s)", len(calls)), map[string]any{
		"round":           round,
		"max_rounds":      maxRounds,
		"tool_call_count": len(calls),
		"tool_calls":      functionCallSummaries(opts, calls),
	})
}

func logLocalToolRequiredRetry(opts LocalToolOptions, round int, maxRounds int) {
	logLocalToolProviderEvent(opts, "tool_required_retry", "pending", "Required grounding tool call missing; retrying Codex provider request", map[string]any{
		"round":                    round,
		"max_rounds":               maxRounds,
		"tool_call_count":          0,
		"tool_use_required_reason": opts.ToolUseRequiredReason,
	})
}

func logLocalToolProviderFailure(opts LocalToolOptions, round int, maxRounds int, err error) {
	logLocalToolProviderEvent(opts, "failed", "failed", err.Error(), map[string]any{
		"round":      round,
		"max_rounds": maxRounds,
		"error":      err.Error(),
	})
}

func logLocalToolProviderEvent(opts LocalToolOptions, eventSuffix string, status string, message string, detail map[string]any) {
	if opts.Logger == nil {
		return
	}
	category := firstNonEmpty(opts.ToolCategory, "model")
	fields := modelmeta.MergeFields(map[string]any{
		"caller_role":          opts.CallerRole,
		"caller_speaker":       opts.CallerSpeaker,
		"status":               status,
		"round":                detail["round"],
		"max_rounds":           detail["max_rounds"],
		"min_call_interval_ms": detail["min_call_interval_ms"],
		"tool_call_count":      detail["tool_call_count"],
		"agent_workspace_root": filepath.ToSlash(opts.Root),
		"input_budget_tokens":  contextBudgetField(opts.ContextBudget, "input_budget_tokens"),
	}, opts.ModelMetadata)
	eventDetail := map[string]any{
		"caller_role":          opts.CallerRole,
		"caller_speaker":       opts.CallerSpeaker,
		"model_metadata":       opts.ModelMetadata,
		"agent_workspace_root": filepath.ToSlash(opts.Root),
		"visible_paths":        opts.VisiblePaths,
	}
	if len(opts.ContextBudget) > 0 {
		eventDetail["context_budget"] = opts.ContextBudget
	}
	for key, value := range detail {
		eventDetail[key] = value
	}
	_ = opts.Logger.Append(gamelog.Event{
		Category: category,
		Event:    category + ".provider." + eventSuffix,
		StoryID:  opts.StoryID,
		Turn:     opts.Turn,
		PacketID: opts.PacketID,
		NodeID:   opts.NodeID,
		RunID:    opts.RunID,
		Message:  message,
		Fields:   fields,
		Detail:   eventDetail,
	})
}

func contextBudgetField(budget map[string]any, key string) any {
	if len(budget) == 0 {
		return nil
	}
	return budget[key]
}

func localToolNames(extra []LocalTool) []string {
	names := []string{"exec_command"}
	for _, tool := range extra {
		name := strings.TrimSpace(tool.Name)
		if name != "" && tool.Handler != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func functionCallSummaries(opts LocalToolOptions, calls []functionCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		info := localToolLogInfo(opts, call)
		out = append(out, map[string]any{
			"tool_use_id":   call.CallID,
			"raw_tool_name": call.Name,
			"tool_name":     info.DisplayName,
			"target":        info.Target,
		})
	}
	return out
}

func logLocalToolUse(opts LocalToolOptions, call functionCall) {
	if !shouldLogLocalTool(opts, call) {
		return
	}
	info := localToolLogInfo(opts, call)
	logLocalToolEvent(opts, call, "tool_use", "pending", "@"+info.DisplayName+targetSuffix(info.Target), map[string]any{
		"tool_name":     info.DisplayName,
		"raw_tool_name": call.Name,
		"tool_use_id":   call.CallID,
		"target":        info.Target,
		"arguments":     call.Arguments,
		"input":         info.Input,
	})
}

func logLocalToolResult(opts LocalToolOptions, call functionCall, result string) {
	if !shouldLogLocalTool(opts, call) {
		return
	}
	info := localToolLogInfo(opts, call)
	resultValue := decodeToolResult(result)
	message := info.DisplayName + " result" + targetSuffix(info.Target)
	preview := toolResultPreview(resultValue)
	if preview.Text != "" {
		message += "\n\n" + preview.Text
	}
	logLocalToolEvent(opts, call, "tool_result", "completed", message, map[string]any{
		"tool_name":         info.DisplayName,
		"raw_tool_name":     call.Name,
		"tool_use_id":       call.CallID,
		"target":            info.Target,
		"arguments":         call.Arguments,
		"input":             info.Input,
		"preview_truncated": preview.Truncated,
		"result":            resultValue,
	})
}

func shouldLogLocalTool(opts LocalToolOptions, call functionCall) bool {
	if call.Name == "exec_command" {
		return true
	}
	for _, tool := range opts.ExtraTools {
		if tool.Name == call.Name {
			return true
		}
	}
	return false
}

type localToolInfo struct {
	DisplayName string
	Target      string
	Input       map[string]any
}

func localToolLogInfo(opts LocalToolOptions, call functionCall) localToolInfo {
	input := map[string]any{}
	_ = json.Unmarshal([]byte(call.Arguments), &input)
	if call.Name != "exec_command" {
		return localToolInfo{
			DisplayName: displayNameForExtraTool(opts.ExtraTools, call.Name),
			Target:      extraToolTarget(input),
			Input:       input,
		}
	}
	cmd, _ := input["cmd"].(string)
	return localToolInfo{
		DisplayName: displayNameForCommand(cmd),
		Target:      strings.TrimSpace(cmd),
		Input:       input,
	}
}

func displayNameForExtraTool(extra []LocalTool, name string) string {
	for _, tool := range extra {
		if tool.Name == name {
			return tool.Name
		}
	}
	return name
}

func extraToolTarget(input map[string]any) string {
	actorID, _ := input["actor_id"].(string)
	question, _ := input["question"].(string)
	query, _ := input["query"].(string)
	topic, _ := input["topic"].(string)
	nodeID, _ := input["node_id"].(string)
	return firstNonEmpty(
		compactToolTarget(actorID, question),
		compactToolTarget(actorID, query),
		compactToolTarget(actorID, topic),
		question,
		query,
		topic,
		actorID,
		nodeID,
	)
}

func compactToolTarget(prefix string, text string) string {
	prefix = strings.TrimSpace(prefix)
	text = strings.TrimSpace(text)
	switch {
	case prefix == "":
		return text
	case text == "":
		return prefix
	default:
		return prefix + ": " + text
	}
}

func displayNameForCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return "Read"
	}
	name := filepath.Base(fields[0])
	switch name {
	case "ls", "find", "pwd":
		return "LS"
	case "rg", "grep":
		return "Grep"
	default:
		return "Read"
	}
}

func logLocalToolEvent(opts LocalToolOptions, call functionCall, eventSuffix string, status string, message string, detail map[string]any) {
	if opts.Logger == nil {
		return
	}
	category := firstNonEmpty(opts.ToolCategory, "model")
	fields := modelmeta.MergeFields(map[string]any{
		"tool_name":            detail["tool_name"],
		"raw_tool_name":        call.Name,
		"tool_use_id":          call.CallID,
		"target":               detail["target"],
		"caller_role":          opts.CallerRole,
		"caller_speaker":       opts.CallerSpeaker,
		"status":               status,
		"agent_workspace_root": filepath.ToSlash(opts.Root),
	}, opts.ModelMetadata)
	eventDetail := map[string]any{
		"tool_name":            detail["tool_name"],
		"raw_tool_name":        call.Name,
		"tool_use_id":          call.CallID,
		"caller_role":          opts.CallerRole,
		"caller_speaker":       opts.CallerSpeaker,
		"model_metadata":       opts.ModelMetadata,
		"agent_workspace_root": filepath.ToSlash(opts.Root),
		"visible_paths":        opts.VisiblePaths,
	}
	for key, value := range detail {
		eventDetail[key] = value
	}
	_ = opts.Logger.Append(gamelog.Event{
		Category: category,
		Event:    category + ".provider." + eventSuffix,
		StoryID:  opts.StoryID,
		Turn:     opts.Turn,
		PacketID: opts.PacketID,
		NodeID:   opts.NodeID,
		RunID:    opts.RunID,
		Message:  message,
		Fields:   fields,
		Detail:   eventDetail,
	})
}

func decodeToolResult(result string) any {
	var value any
	if err := json.Unmarshal([]byte(result), &value); err != nil {
		return result
	}
	return value
}

type toolPreview struct {
	Text      string
	Truncated bool
}

func toolResultPreview(result any) toolPreview {
	mapped, ok := result.(map[string]any)
	if !ok {
		if text, ok := result.(string); ok {
			return trimToolPreview(text)
		}
		return toolPreview{}
	}
	for _, key := range []string{"stdout", "content", "error", "stderr"} {
		if value, ok := mapped[key].(string); ok && strings.TrimSpace(value) != "" {
			return trimToolPreview(value)
		}
	}
	payload, err := json.Marshal(mapped)
	if err != nil {
		return toolPreview{}
	}
	return trimToolPreview(string(payload))
}

func trimToolPreview(value string) toolPreview {
	value = strings.TrimSpace(value)
	const max = 4000
	if len(value) <= max {
		return toolPreview{Text: value}
	}
	return toolPreview{Text: value[:max] + "... [preview truncated]", Truncated: true}
}

func targetSuffix(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	return " " + target
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toolExecCommand(ctx context.Context, root string, rawArgs string) (map[string]any, error) {
	var args struct {
		Cmd             string `json:"cmd"`
		Workdir         string `json:"workdir"`
		YieldTimeMS     int    `json:"yield_time_ms"`
		BackgroundTask  bool   `json:"background_task"`
		CleanupHint     string `json:"cleanup_hint"`
		MaxOutputTokens int    `json:"max_output_tokens"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return nil, err
	}
	if args.Cmd == "" {
		return nil, errors.New("cmd is required")
	}
	if err := validateReadOnlyCommand(args.Cmd); err != nil {
		return nil, err
	}
	workdir := root
	if args.Workdir != "" {
		var err error
		workdir, err = safePath(root, args.Workdir)
		if err != nil {
			return nil, err
		}
	}
	if err := validateCommandPaths(root, args.Cmd); err != nil {
		return nil, err
	}

	maxBytes := outputByteLimit(args.MaxOutputTokens)
	toolCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	command := exec.Command("/bin/zsh", "-lc", args.Cmd)
	command.Dir = workdir
	configureProcessGroup(command)
	stdout := &tailBuffer{limit: maxBytes}
	stderr := &tailBuffer{limit: maxBytes / 2}
	command.Stdout = stdout
	command.Stderr = stderr

	exitCode, timedOut, durationMS := runLocalShellCommand(toolCtx, command)

	return map[string]any{
		"ok":               exitCode == 0,
		"cmd":              args.Cmd,
		"workdir":          workdir,
		"background_task":  args.BackgroundTask,
		"cleanup_hint":     strings.TrimSpace(args.CleanupHint),
		"exit_code":        exitCode,
		"timed_out":        timedOut,
		"duration_ms":      durationMS,
		"stdout":           stdout.String(),
		"stderr":           stderr.String(),
		"stdout_truncated": stdout.truncated,
		"stderr_truncated": stderr.truncated,
	}, nil
}

func runLocalShellCommand(ctx context.Context, command *exec.Cmd) (int, bool, int64) {
	start := time.Now()
	if err := command.Start(); err != nil {
		return 127, false, time.Since(start).Milliseconds()
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- command.Wait()
	}()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-waitCh:
	case <-ctx.Done():
		timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		killProcessGroup(command)
		waitErr = <-waitCh
	}

	return exitCodeFromWait(waitErr, timedOut), timedOut, time.Since(start).Milliseconds()
}

func exitCodeFromWait(waitErr error, timedOut bool) int {
	if timedOut {
		return 124
	}
	if waitErr == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return 127
}

func validateReadOnlyCommand(command string) error {
	_, err := parseReadOnlyCommandSegments(command)
	return err
}

func parseReadOnlyCommandSegments(command string) ([][]string, error) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil, fmt.Errorf("invalid shell command: %w", err)
	}
	if len(file.Stmts) != 1 {
		return nil, errors.New("command must contain exactly one statement")
	}
	segments, err := readOnlySegmentsFromStmt(file.Stmts[0])
	if err != nil {
		return nil, err
	}
	if len(segments) > 4 {
		return nil, errors.New("command pipeline is too long")
	}
	for _, segment := range segments {
		name := filepath.Base(segment[0])
		if !readOnlyCommands[name] {
			return nil, fmt.Errorf("command %q is not in the read-only allowlist", name)
		}
		if err := validateReadOnlyArgs(name, segment[1:]); err != nil {
			return nil, err
		}
	}
	return segments, nil
}

func readOnlySegmentsFromStmt(stmt *syntax.Stmt) ([][]string, error) {
	if stmt == nil {
		return nil, errors.New("empty command segment")
	}
	if stmt.Negated {
		return nil, errors.New("negated commands are not allowed")
	}
	if stmt.Background {
		return nil, errors.New("background commands are not allowed")
	}
	if stmt.Coprocess {
		return nil, errors.New("coprocess commands are not allowed")
	}
	if stmt.Semicolon.IsValid() {
		return nil, errors.New("command separators are not allowed")
	}
	if len(stmt.Redirs) > 0 {
		return nil, errors.New("shell redirects are not allowed")
	}
	switch cmd := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		return readOnlySegmentFromCall(cmd)
	case *syntax.BinaryCmd:
		if cmd.Op != syntax.Pipe {
			return nil, fmt.Errorf("shell operator %q is not allowed", cmd.Op.String())
		}
		left, err := readOnlySegmentsFromStmt(cmd.X)
		if err != nil {
			return nil, err
		}
		right, err := readOnlySegmentsFromStmt(cmd.Y)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	default:
		return nil, fmt.Errorf("shell command form %T is not allowed", cmd)
	}
}

func readOnlySegmentFromCall(call *syntax.CallExpr) ([][]string, error) {
	if len(call.Assigns) > 0 {
		return nil, errors.New("environment assignments are not allowed")
	}
	if len(call.Args) == 0 {
		return nil, errors.New("empty command segment")
	}
	fields := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		field, err := staticShellWord(word)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(field) == "" {
			return nil, errors.New("empty command argument")
		}
		fields = append(fields, field)
	}
	return [][]string{fields}, nil
}

func staticShellWord(word *syntax.Word) (string, error) {
	var b strings.Builder
	for _, part := range word.Parts {
		value, err := staticShellWordPart(part)
		if err != nil {
			return "", err
		}
		b.WriteString(value)
	}
	return b.String(), nil
}

func staticShellWordPart(part syntax.WordPart) (string, error) {
	switch p := part.(type) {
	case *syntax.Lit:
		return p.Value, nil
	case *syntax.SglQuoted:
		return p.Value, nil
	case *syntax.DblQuoted:
		var b strings.Builder
		for _, nested := range p.Parts {
			value, err := staticShellWordPart(nested)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
		}
		return b.String(), nil
	default:
		return "", fmt.Errorf("shell expansion %T is not allowed", part)
	}
}

func validateCommandPaths(root string, command string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	segments, err := parseReadOnlyCommandSegments(command)
	if err != nil {
		return err
	}
	for _, segment := range segments {
		for _, raw := range segment[1:] {
			arg := trimShellToken(raw)
			if arg == "" || strings.HasPrefix(arg, "-") {
				continue
			}
			if strings.Contains(arg, "..") {
				return fmt.Errorf("command path escapes agent workspace: %s", raw)
			}
			if agentWorkspaceSurface(arg) {
				return fmt.Errorf("command path is outside the curated agent workspace: %s", raw)
			}
			if strings.HasPrefix(arg, "~") {
				return fmt.Errorf("home-relative paths are not allowed in scoped tools: %s", raw)
			}
			if filepath.IsAbs(arg) {
				absArg, err := filepath.Abs(arg)
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(absRoot, absArg)
				if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
					return fmt.Errorf("absolute path escapes agent workspace: %s", raw)
				}
				if agentWorkspaceSurface(rel) {
					return fmt.Errorf("command path is outside the curated agent workspace: %s", raw)
				}
			}
		}
	}
	return nil
}

func trimShellToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSuffix(value, ":")
	return value
}

func agentWorkspaceSurface(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	clean = strings.TrimPrefix(clean, "./")
	forbidden := []string{
		".git",
		"cmd",
		"internal",
		"testdata",
		"lore",
		"lorepack/comics/peer-cards/runs",
		"lorepack/comics/peer-cards/provenance",
		"lorepack/index/documents.jsonl",
		"lorepack/testdata",
		"data/stories",
		"data/seed-runs",
	}
	for _, prefix := range forbidden {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	return false
}

func validateReadOnlyArgs(name string, args []string) error {
	for _, arg := range args {
		switch {
		case name == "sed" && (arg == "-i" || arg == "--in-place" || strings.HasPrefix(arg, "-i")):
			return errors.New("sed in-place editing is not allowed")
		case name == "find" && forbiddenFindArg(arg):
			return fmt.Errorf("find argument %q is not allowed", arg)
		case strings.HasPrefix(arg, "--output") || strings.HasPrefix(arg, "-o"):
			return fmt.Errorf("output-writing argument %q is not allowed", arg)
		}
	}
	return nil
}

func forbiddenFindArg(arg string) bool {
	forbidden := map[string]bool{
		"-delete":  true,
		"-exec":    true,
		"-execdir": true,
		"-ok":      true,
		"-okdir":   true,
		"-fls":     true,
		"-fprint":  true,
		"-fprintf": true,
	}
	return forbidden[arg]
}

func outputByteLimit(maxOutputTokens int) int {
	if maxOutputTokens <= 0 {
		maxOutputTokens = 12000
	}
	if maxOutputTokens < 1000 {
		maxOutputTokens = 1000
	}
	if maxOutputTokens > 50000 {
		maxOutputTokens = 50000
	}
	return maxOutputTokens * 4
}

func safePath(root string, requested string) (string, error) {
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes agent workspace: %s", requested)
	}
	return abs, nil
}

type tailBuffer struct {
	limit     int
	truncated bool
	data      []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	if b.limit <= 0 {
		b.truncated = true
		return originalLen, nil
	}
	if len(p) >= b.limit {
		b.truncated = true
		b.data = append(b.data[:0], p[len(p)-b.limit:]...)
		return originalLen, nil
	}
	overflow := len(b.data) + len(p) - b.limit
	if overflow > 0 {
		b.truncated = true
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, p...)
	return originalLen, nil
}

func (b *tailBuffer) String() string {
	return string(bytes.ToValidUTF8(b.data, []byte("\uFFFD")))
}

func AllowedReadOnlyCommands() []string {
	commands := make([]string, 0, len(readOnlyCommands))
	for command := range readOnlyCommands {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	return commands
}
