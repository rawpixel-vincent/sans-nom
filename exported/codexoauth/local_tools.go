package codexoauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultMaxToolRounds = 24

var readOnlyCommands = map[string]bool{
	"cat":  true,
	"find": true,
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
	Root      string
	MaxRounds int
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

	current := cloneMap(body)
	current["tools"] = appendTools(current["tools"], localToolDefinitions())

	for round := 0; round < maxRounds; round++ {
		response, err := c.Create(ctx, current)
		if err != nil {
			return nil, err
		}
		calls := collectFunctionCalls(response["output"])
		if len(calls) == 0 {
			return response, nil
		}

		input := inputItems(current["input"])
		input = append(input, functionCallInputItems(calls)...)
		for _, call := range calls {
			result := executeLocalTool(ctx, root, call)
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

func localToolDefinitions() []any {
	return []any{
		map[string]any{
			"type":        "function",
			"name":        "exec_command",
			"description": "Run a read-only shell command inside the repository. Prefer rg, sed, jq, find, cat, ls, head, tail, wc, sort, uniq, nl, and pwd.",
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
						"description": "Working directory under the repository root. Defaults to the repository root.",
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

func executeLocalTool(ctx context.Context, root string, call functionCall) string {
	var output any
	var err error
	switch call.Name {
	case "exec_command":
		output, err = toolExecCommand(ctx, root, call.Arguments)
	default:
		err = fmt.Errorf("unknown tool %q", call.Name)
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
	forbidden := []string{"\n", "\r", ";", "&&", "||", "`", "$(", ">", "<"}
	for _, token := range forbidden {
		if strings.Contains(command, token) {
			return fmt.Errorf("command contains forbidden shell token %q", token)
		}
	}
	segments := strings.Split(command, "|")
	if len(segments) > 4 {
		return errors.New("command pipeline is too long")
	}
	for _, segment := range segments {
		fields := strings.Fields(strings.TrimSpace(segment))
		if len(fields) == 0 {
			return errors.New("empty command segment")
		}
		name := filepath.Base(fields[0])
		if !readOnlyCommands[name] {
			return fmt.Errorf("command %q is not in the read-only allowlist", name)
		}
		if err := validateReadOnlyArgs(name, fields[1:]); err != nil {
			return err
		}
	}
	return nil
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
		return "", fmt.Errorf("path escapes repository root: %s", requested)
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
