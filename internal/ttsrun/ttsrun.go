package ttsrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rawpixel-vincent/sans-nom/internal/gamelog"
	"github.com/rawpixel-vincent/sans-nom/internal/modelmeta"
	"github.com/rawpixel-vincent/sans-nom/internal/narrator"
	"github.com/rawpixel-vincent/sans-nom/internal/runutil"
	"github.com/rawpixel-vincent/sans-nom/internal/story"
	ttsclient "github.com/rawpixel-vincent/sans-nom/internal/tts"
)

type Config struct {
	Enabled     bool
	BaseURL     string
	Model       string
	Voice       string
	Format      string
	Instruction string
	OutputDir   string
	APIKey      string
	Timeout     time.Duration
	Status      func(string)
}

func SynthesizeTurnAudio(ctx context.Context, cfg Config, workspace *story.Workspace, state *story.StoryState, logger *gamelog.Writer, loopTurn int, rollout narrator.Rollout) (string, error) {
	if !cfg.Enabled {
		return "", nil
	}
	text := strings.TrimSpace(rollout.Draft.Prose)
	if text == "" {
		return "", nil
	}
	format := NormalizeFormat(cfg.Format)
	if format == "" {
		return "", fmt.Errorf("unsupported tts format %q; expected wav or pcm16", cfg.Format)
	}
	turnNumber := currentStoryTurnNumber(state, loopTurn)
	outputPath := OutputPath(cfg, workspace, state, loopTurn)
	runDir := RunDir(cfg, workspace, state, loopTurn)
	client := ttsclient.Client{
		BaseURL:     cfg.BaseURL,
		APIKey:      runutil.FirstNonEmpty(cfg.APIKey, os.Getenv("SANS_NOM_TTS_API_KEY")),
		Model:       cfg.Model,
		Voice:       cfg.Voice,
		Format:      format,
		Instruction: cfg.Instruction,
		Timeout:     cfg.Timeout,
	}
	chunks := splitTTSChunks(text, runutil.EnvInt("SANS_NOM_TTS_CHUNK_CHARS", defaultTTSChunkChars))
	requestFormat := format
	if len(chunks) > 1 && format == "wav" {
		requestFormat = "pcm16"
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	requestSummary := ttsRequestSummary(client, text, chunks, format, requestFormat, outputPath)
	ttsMetadata := ModelMetadata(cfg, runutil.StringFromAny(requestSummary["model"]))
	if err := runutil.WriteJSONFileAtomic(filepath.Join(runDir, "request.json"), map[string]any{
		"request":         requestSummary,
		"model_metadata":  ttsMetadata,
		"base_url":        runutil.FirstNonEmpty(cfg.BaseURL, ttsclient.DefaultBaseURL),
		"output_path":     filepath.ToSlash(outputPath),
		"instruction":     cfg.Instruction,
		"text_sha256":     textSHA256(text),
		"text_chars":      len([]rune(text)),
		"chunk_count":     len(chunks),
		"chunk_max_chars": runutil.EnvInt("SANS_NOM_TTS_CHUNK_CHARS", defaultTTSChunkChars),
	}); err != nil {
		return "", err
	}
	runutil.LogEvent(logger, gamelog.Event{
		Category: "model",
		Event:    "tts.mimo.request",
		Message:  "mimo turn tts request prepared",
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"model":          requestSummary["model"],
			"voice":          requestSummary["voice"],
			"format":         format,
			"request_format": requestFormat,
			"text_chars":     len([]rune(text)),
			"chunk_count":    len(chunks),
			"output_path":    filepath.ToSlash(outputPath),
			"run_dir":        filepath.ToSlash(runDir),
		}, ttsMetadata),
		Detail: map[string]any{
			"request":        requestSummary,
			"base_url":       runutil.FirstNonEmpty(cfg.BaseURL, ttsclient.DefaultBaseURL),
			"output_path":    filepath.ToSlash(outputPath),
			"run_dir":        filepath.ToSlash(runDir),
			"text_sha256":    textSHA256(text),
			"model_metadata": ttsMetadata,
		},
	})
	ttsCtx, cancel := context.WithTimeout(ctx, client.TimeoutOrDefault())
	defer cancel()
	playStatusf(cfg, "\nTTS MiMo: synthesizing turn %d in %d chunk(s)...", turnNumber, len(chunks))
	var chunkAudios [][]byte
	var responses []map[string]any
	totalBase64Chars := 0
	for index, chunk := range chunks {
		chunkClient := client
		chunkClient.Format = requestFormat
		chunkRequest := chunkClient.RequestPayload(chunk)
		chunkDir := filepath.Join(runDir, fmt.Sprintf("chunk-%03d", index+1))
		if err := os.MkdirAll(chunkDir, 0o755); err != nil {
			return "", err
		}
		if err := runutil.WriteJSONFileAtomic(filepath.Join(chunkDir, "request.json"), chunkRequest); err != nil {
			return "", err
		}
		playStatusf(cfg, "TTS MiMo chunk %d/%d...", index+1, len(chunks))
		result, err := chunkClient.Synthesize(ttsCtx, chunk)
		responsePath := filepath.Join(chunkDir, "response.json")
		if writeErr := runutil.WriteJSONFileAtomic(responsePath, result.Response); writeErr != nil {
			return "", writeErr
		}
		if err != nil {
			failed := map[string]any{
				"chunk":              index + 1,
				"chars":              len([]rune(chunk)),
				"text_sha256":        textSHA256(chunk),
				"request_path":       filepath.ToSlash(filepath.Join(chunkDir, "request.json")),
				"response_path":      filepath.ToSlash(responsePath),
				"audio_path":         "",
				"audio_bytes":        0,
				"audio_base64_chars": result.AudioBase64Chars,
				"error":              err.Error(),
			}
			responses = append(responses, failed)
			_ = runutil.WriteJSONFileAtomic(filepath.Join(runDir, "response.json"), map[string]any{
				"chunk_count":  len(chunks),
				"failed_chunk": index + 1,
				"chunks":       responses,
			})
			logTTSFailure(cfg, logger, rollout, turnNumber, runDir, outputPath, err, map[string]any{
				"request":       chunkRequest,
				"chunk":         index + 1,
				"chunk_count":   len(chunks),
				"response_path": filepath.ToSlash(responsePath),
			})
			return "", err
		}
		chunkAudioPath := filepath.Join(chunkDir, "audio."+requestFormat)
		if err := runutil.WriteBinaryAtomic(chunkAudioPath, result.Audio); err != nil {
			logTTSFailure(cfg, logger, rollout, turnNumber, runDir, outputPath, err, map[string]any{
				"request":       chunkRequest,
				"chunk":         index + 1,
				"chunk_count":   len(chunks),
				"response_path": filepath.ToSlash(responsePath),
				"phase":         "write_chunk_audio",
			})
			return "", err
		}
		chunkAudios = append(chunkAudios, result.Audio)
		totalBase64Chars += result.AudioBase64Chars
		responses = append(responses, map[string]any{
			"chunk":              index + 1,
			"chars":              len([]rune(chunk)),
			"text_sha256":        textSHA256(chunk),
			"request_path":       filepath.ToSlash(filepath.Join(chunkDir, "request.json")),
			"response_path":      filepath.ToSlash(responsePath),
			"audio_path":         filepath.ToSlash(chunkAudioPath),
			"audio_bytes":        len(result.Audio),
			"audio_base64_chars": result.AudioBase64Chars,
		})
		chunkDetail := responses[len(responses)-1]
		chunkDetail["model_metadata"] = ttsMetadata
		runutil.LogEvent(logger, gamelog.Event{
			Category: "model",
			Event:    "tts.mimo.chunk.completed",
			Message:  "mimo turn tts chunk written",
			Turn:     turnNumber,
			PacketID: rollout.Commit.PacketID,
			NodeID:   rollout.Commit.NodeID,
			Fields: modelmeta.MergeFields(map[string]any{
				"chunk":       index + 1,
				"chunk_count": len(chunks),
				"chars":       len([]rune(chunk)),
				"audio_bytes": len(result.Audio),
			}, ttsMetadata),
			Detail: chunkDetail,
		})
	}
	if err := runutil.WriteJSONFileAtomic(filepath.Join(runDir, "response.json"), map[string]any{
		"chunk_count": len(chunks),
		"chunks":      responses,
	}); err != nil {
		return "", err
	}
	audio := bytes.Join(chunkAudios, nil)
	if format == "wav" && requestFormat == "pcm16" {
		audio = wavFromPCM16(audio, 24000, 1)
	}
	if err := runutil.WriteBinaryAtomic(outputPath, audio); err != nil {
		logTTSFailure(cfg, logger, rollout, turnNumber, runDir, outputPath, err, map[string]any{"request": requestSummary, "response_path": filepath.ToSlash(filepath.Join(runDir, "response.json")), "phase": "write_audio_file"})
		return "", err
	}
	runMetadata := map[string]any{
		"model":              requestSummary["model"],
		"voice":              requestSummary["voice"],
		"format":             format,
		"request_format":     requestFormat,
		"chunk_count":        len(chunks),
		"text_chars":         len([]rune(text)),
		"text_sha256":        textSHA256(text),
		"output_path":        filepath.ToSlash(outputPath),
		"audio_bytes":        len(audio),
		"audio_base64_chars": totalBase64Chars,
		"response_path":      filepath.ToSlash(filepath.Join(runDir, "response.json")),
		"created_at":         story.NowString(),
	}
	runMetadata["model_metadata"] = ttsMetadata
	if err := runutil.WriteJSONFileAtomic(filepath.Join(runDir, "metadata.json"), runMetadata); err != nil {
		logTTSFailure(cfg, logger, rollout, turnNumber, runDir, outputPath, err, map[string]any{"request": requestSummary, "metadata": runMetadata, "phase": "write_metadata"})
		return "", err
	}
	runutil.LogEvent(logger, gamelog.Event{
		Category: "model",
		Event:    "tts.mimo.completed",
		Message:  "mimo turn tts audio written",
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"model":       requestSummary["model"],
			"voice":       requestSummary["voice"],
			"format":      format,
			"chunk_count": len(chunks),
			"audio_bytes": len(audio),
			"output_path": filepath.ToSlash(outputPath),
			"run_dir":     filepath.ToSlash(runDir),
		}, ttsMetadata),
		Detail: map[string]any{
			"request":        requestSummary,
			"response_path":  filepath.ToSlash(filepath.Join(runDir, "response.json")),
			"metadata":       runMetadata,
			"model_metadata": ttsMetadata,
		},
	})
	return outputPath, nil
}

func playStatusf(cfg Config, format string, args ...any) {
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	if message == "" {
		return
	}
	if cfg.Status != nil {
		cfg.Status(message)
		return
	}
	fmt.Println(message)
}

func OutputPath(cfg Config, workspace *story.Workspace, state *story.StoryState, loopTurn int) string {
	outputDir := strings.TrimSpace(cfg.OutputDir)
	if outputDir == "" && workspace != nil {
		outputDir = filepath.Join(workspace.Dir, "audio")
	}
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "tts")
	}
	return filepath.Join(outputDir, fmt.Sprintf("turn-%06d.%s", currentStoryTurnNumber(state, loopTurn), NormalizeFormat(cfg.Format)))
}

func RunDir(cfg Config, workspace *story.Workspace, state *story.StoryState, loopTurn int) string {
	segment := fmt.Sprintf("turn-%06d", currentStoryTurnNumber(state, loopTurn))
	if workspace != nil {
		return filepath.Join(workspace.ModelRunsDir(), "tts", segment)
	}
	outputDir := strings.TrimSpace(cfg.OutputDir)
	if outputDir == "" {
		outputDir = filepath.Join("tmp", "tts")
	}
	return filepath.Join(outputDir, "model-runs", "tts", segment)
}

func logTTSFailure(cfg Config, logger *gamelog.Writer, rollout narrator.Rollout, turnNumber int, runDir string, outputPath string, err error, detail map[string]any) {
	_ = os.MkdirAll(runDir, 0o755)
	_ = runutil.WriteTextAtomic(filepath.Join(runDir, "error.txt"), err.Error()+"\n")
	if detail == nil {
		detail = map[string]any{}
	}
	detail["error"] = err.Error()
	detail["output_path"] = filepath.ToSlash(outputPath)
	detail["run_dir"] = filepath.ToSlash(runDir)
	metadata := ModelMetadata(cfg, "")
	detail["model_metadata"] = metadata
	runutil.LogEvent(logger, gamelog.Event{
		Level:    gamelog.LevelError,
		Category: "model",
		Event:    "tts.mimo.failed",
		Message:  err.Error(),
		Turn:     turnNumber,
		PacketID: rollout.Commit.PacketID,
		NodeID:   rollout.Commit.NodeID,
		Fields: modelmeta.MergeFields(map[string]any{
			"output_path": filepath.ToSlash(outputPath),
			"run_dir":     filepath.ToSlash(runDir),
		}, metadata),
		Detail: detail,
	})
}

func NormalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "wav":
		return "wav"
	case "pcm16":
		return "pcm16"
	default:
		return ""
	}
}

const defaultTTSChunkChars = 4500

func textSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func ttsRequestSummary(client ttsclient.Client, text string, chunks []string, outputFormat string, requestFormat string, outputPath string) map[string]any {
	template := client
	template.Format = requestFormat
	request := template.RequestPayload("")
	chunkSummaries := make([]map[string]any, 0, len(chunks))
	for index, chunk := range chunks {
		chunkSummaries = append(chunkSummaries, map[string]any{
			"chunk":       index + 1,
			"chars":       len([]rune(chunk)),
			"text_sha256": textSHA256(chunk),
		})
	}
	return map[string]any{
		"model":          request.Model,
		"voice":          request.Audio.Voice,
		"output_format":  outputFormat,
		"request_format": requestFormat,
		"instruction":    client.Instruction,
		"text_sha256":    textSHA256(text),
		"text_chars":     len([]rune(text)),
		"chunk_count":    len(chunks),
		"chunks":         chunkSummaries,
		"output_path":    filepath.ToSlash(outputPath),
		"endpoint":       strings.TrimRight(runutil.FirstNonEmpty(client.BaseURL, ttsclient.DefaultBaseURL), "/") + "/chat/completions",
	}
}

func ModelMetadata(cfg Config, model string) modelmeta.Metadata {
	if strings.TrimSpace(model) == "" {
		model = runutil.FirstNonEmpty(cfg.Model, "mimo-v2.5-tts")
	}
	return modelmeta.Metadata{
		ProviderType:  "openai_compatible_tts",
		ModelProvider: "xiaomi_mimo",
		Model:         model,
	}
}

func splitTTSChunks(text string, maxRunes int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxRunes <= 0 {
		maxRunes = defaultTTSChunkChars
	}
	if len([]rune(text)) <= maxRunes {
		return []string{text}
	}
	var chunks []string
	for _, paragraph := range strings.Split(text, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		if len([]rune(paragraph)) <= maxRunes {
			chunks = appendTTSChunk(chunks, paragraph, maxRunes)
			continue
		}
		for _, sentence := range splitSentences(paragraph) {
			chunks = appendTTSChunk(chunks, sentence, maxRunes)
		}
	}
	return chunks
}

func appendTTSChunk(chunks []string, part string, maxRunes int) []string {
	part = strings.TrimSpace(part)
	if part == "" {
		return chunks
	}
	if len([]rune(part)) > maxRunes {
		var current []rune
		for _, word := range strings.Fields(part) {
			wordRunes := []rune(word)
			if len(current) > 0 && len(current)+1+len(wordRunes) > maxRunes {
				chunks = appendTTSChunk(chunks, string(current), maxRunes)
				current = nil
			}
			if len(current) > 0 {
				current = append(current, ' ')
			}
			current = append(current, wordRunes...)
		}
		if len(current) > 0 {
			chunks = append(chunks, strings.TrimSpace(string(current)))
		}
		return chunks
	}
	if len(chunks) == 0 {
		return append(chunks, part)
	}
	last := chunks[len(chunks)-1]
	sep := "\n\n"
	if !strings.Contains(part, "\n") && !strings.Contains(last, "\n") {
		sep = " "
	}
	if len([]rune(last))+len([]rune(sep))+len([]rune(part)) <= maxRunes {
		chunks[len(chunks)-1] = last + sep + part
		return chunks
	}
	return append(chunks, part)
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	runes := []rune(text)
	for i, r := range runes {
		if r != '.' && r != '!' && r != '?' && r != '\n' {
			continue
		}
		if i+1 < len(runes) && runes[i+1] != ' ' && runes[i+1] != '\n' {
			continue
		}
		part := strings.TrimSpace(string(runes[start : i+1]))
		if part != "" {
			out = append(out, part)
		}
		start = i + 1
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

func wavFromPCM16(pcm []byte, sampleRate int, channels int) []byte {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	if channels <= 0 {
		channels = 1
	}
	byteRate := sampleRate * channels * 2
	blockAlign := channels * 2
	buf := bytes.NewBuffer(make([]byte, 0, 44+len(pcm)))
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(36+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(16))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

func FromFlags(enabled bool, baseURL string, model string, voice string, format string, instruction string, outputDir string) Config {
	return Config{
		Enabled:     enabled,
		BaseURL:     runutil.FirstNonEmpty(baseURL, ttsclient.DefaultBaseURL),
		Model:       runutil.FirstNonEmpty(model, ttsclient.DefaultModel),
		Voice:       runutil.FirstNonEmpty(voice, ttsclient.DefaultVoice),
		Format:      runutil.FirstNonEmpty(format, ttsclient.DefaultAudioFormat),
		Instruction: runutil.FirstNonEmpty(instruction, ttsclient.DefaultNarratorInstruction),
		OutputDir:   outputDir,
		Timeout:     time.Duration(runutil.EnvInt("SANS_NOM_TTS_TIMEOUT_MS", int(ttsclient.DefaultTimeout/time.Millisecond))) * time.Millisecond,
	}
}

func currentStoryTurnNumber(state *story.StoryState, loopTurn int) int {
	if state != nil {
		return state.LatestTurn + 1
	}
	if loopTurn <= 0 {
		return 1
	}
	return loopTurn
}
