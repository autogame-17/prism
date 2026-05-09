package subscriptioncli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"one-api/common"
	"one-api/common/config"
	"one-api/common/requester"
	"one-api/model"
	"one-api/providers/base"
	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	defaultTimeout = 10 * time.Minute
)

type ProviderFactory struct{}

type Provider struct {
	base.BaseProvider
	kind string
}

func (f ProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	kind := "codex"
	if channel.Type == config.ChannelTypeClaudeSubscription {
		kind = "claude"
	}

	return &Provider{
		BaseProvider: base.BaseProvider{
			Channel: channel,
		},
		kind: kind,
	}
}

func (p *Provider) GetRequestHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

func (p *Provider) GetRequester() *requester.HTTPRequester {
	return nil
}

func (p *Provider) CreateChatCompletion(request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *types.OpenAIErrorWithStatusCode) {
	modelName := request.Model
	if modelName == "" {
		modelName = p.defaultModel()
	}

	prompt := buildPrompt(request)
	if strings.TrimSpace(prompt) == "" {
		return nil, common.StringErrorWrapperLocal("messages must contain text content", "invalid_request_error", http.StatusBadRequest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	content, err := p.runCLI(ctx, modelName, prompt)
	if err != nil {
		return nil, common.ErrorWrapperLocal(err, "subscription_cli_error", http.StatusBadGateway)
	}

	promptTokens := common.CountTokenText(prompt, modelName)
	completionTokens := common.CountTokenText(content, modelName)
	usage := &types.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	if p.Usage != nil {
		*p.Usage = *usage
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []types.ChatCompletionChoice{
			{
				Index: 0,
				Message: types.ChatCompletionMessage{
					Role:    types.ChatMessageRoleAssistant,
					Content: content,
				},
				FinishReason: types.FinishReasonStop,
			},
		},
		Usage: usage,
	}, nil
}

func (p *Provider) CreateChatCompletionStream(request *types.ChatCompletionRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode) {
	response, errWithCode := p.CreateChatCompletion(request)
	if errWithCode != nil {
		return nil, errWithCode
	}

	content := response.GetContent()
	roleChunk, _ := json.Marshal(types.ChatCompletionStreamResponse{
		ID:      response.ID,
		Object:  "chat.completion.chunk",
		Created: response.Created,
		Model:   response.Model,
		Choices: []types.ChatCompletionStreamChoice{
			{
				Index: 0,
				Delta: types.ChatCompletionStreamChoiceDelta{Role: types.ChatMessageRoleAssistant},
			},
		},
	})
	contentChunk, _ := json.Marshal(types.ChatCompletionStreamResponse{
		ID:      response.ID,
		Object:  "chat.completion.chunk",
		Created: response.Created,
		Model:   response.Model,
		Choices: []types.ChatCompletionStreamChoice{
			{
				Index:        0,
				Delta:        types.ChatCompletionStreamChoiceDelta{Content: content},
				FinishReason: nil,
			},
		},
	})
	stopChunk, _ := json.Marshal(types.ChatCompletionStreamResponse{
		ID:      response.ID,
		Object:  "chat.completion.chunk",
		Created: response.Created,
		Model:   response.Model,
		Choices: []types.ChatCompletionStreamChoice{
			{
				Index:        0,
				Delta:        types.ChatCompletionStreamChoiceDelta{},
				FinishReason: types.FinishReasonStop,
			},
		},
	})

	return newStaticStream([]string{string(roleChunk), string(contentChunk), string(stopChunk)}), nil
}

func (p *Provider) defaultModel() string {
	if p.kind == "claude" {
		return "sonnet"
	}
	return "gpt-5.5"
}

func (p *Provider) runCLI(ctx context.Context, modelName string, prompt string) (string, error) {
	switch p.kind {
	case "claude":
		return runClaude(ctx, modelName, prompt)
	default:
		return runCodex(ctx, modelName, prompt)
	}
}

func runCodex(ctx context.Context, modelName string, prompt string) (string, error) {
	codex, err := lookPath("codex", codexCandidates())
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp("", "prism-codex-last-*.txt")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	args := []string{
		"exec",
		"--model", modelName,
		"--skip-git-repo-check",
		"--ephemeral",
		"--sandbox", "read-only",
		"--output-last-message", tmpPath,
		"-",
	}
	cmd := exec.CommandContext(ctx, codex, args...)
	cmd.Stdin = strings.NewReader(prompt)
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("codex timed out after %s", defaultTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("codex failed: %w: %s", err, trimOutput(out))
	}
	body, readErr := os.ReadFile(tmpPath)
	if readErr != nil {
		return "", readErr
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = strings.TrimSpace(string(out))
	}
	if text == "" {
		return "", errors.New("codex returned an empty response")
	}
	return text, nil
}

func runClaude(ctx context.Context, modelName string, prompt string) (string, error) {
	claude, err := lookPath("claude", claudeCandidates())
	if err != nil {
		return "", err
	}

	args := []string{
		"-p",
		"--model", modelName,
		"--output-format", "text",
		"--tools", "",
		"--permission-mode", "dontAsk",
		"--no-session-persistence",
		prompt,
	}
	cmd := exec.CommandContext(ctx, claude, args...)
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("claude timed out after %s", defaultTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("claude failed: %w: %s", err, trimOutput(out))
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "", errors.New("claude returned an empty response")
	}
	return text, nil
}

func buildPrompt(request *types.ChatCompletionRequest) string {
	var b strings.Builder
	hasContent := false

	if request.System != nil {
		if raw, err := json.Marshal(request.System); err == nil {
			b.WriteString("System:\n")
			b.Write(raw)
			b.WriteString("\n\n")
			hasContent = true
		}
	}

	for _, msg := range request.Messages {
		content := strings.TrimSpace(msg.StringContent())
		if content == "" {
			continue
		}
		role := msg.Role
		if role == "" {
			role = "user"
		}
		b.WriteString(strings.ToUpper(role[:1]))
		if len(role) > 1 {
			b.WriteString(role[1:])
		}
		b.WriteString(":\n")
		b.WriteString(content)
		b.WriteString("\n\n")
		hasContent = true
	}

	if !hasContent {
		return ""
	}

	if request.ResponseFormat != nil && request.ResponseFormat.Type != "" {
		b.WriteString("Response format requirement: ")
		b.WriteString(request.ResponseFormat.Type)
		b.WriteString("\n\n")
	}
	if len(request.Tools) > 0 || len(request.Functions) > 0 {
		b.WriteString("Note: tool calls are not available through this local subscription CLI bridge. Answer directly in text.\n\n")
	}

	b.WriteString("Reply as the assistant. Return only the assistant message content.")
	return b.String()
}

func lookPath(name string, candidates []string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s executable not found", name)
}

func codexCandidates() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/Applications/Codex.app/Contents/Resources/codex",
		filepath.Join(home, ".openai", "bin", "codex"),
		filepath.Join(home, ".local", "bin", "codex"),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(home, "AppData", "Local", "Programs", "Codex", "codex.exe"))
	}
	return candidates
}

func claudeCandidates() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
		filepath.Join(home, ".local", "bin", "claude"),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(home, "AppData", "Roaming", "npm", "claude.cmd"))
	}
	return candidates
}

func trimOutput(out []byte) string {
	text := strings.TrimSpace(string(out))
	if len(text) > 4000 {
		return text[len(text)-4000:]
	}
	return text
}

type staticStream struct {
	once     sync.Once
	chunks   []string
	dataChan chan string
	errChan  chan error
}

func newStaticStream(chunks []string) *staticStream {
	return &staticStream{
		chunks:   chunks,
		dataChan: make(chan string, len(chunks)),
		errChan:  make(chan error, 1),
	}
}

func (s *staticStream) Recv() (<-chan string, <-chan error) {
	s.once.Do(func() {
		gopool.Go(func() {
			for _, chunk := range s.chunks {
				s.dataChan <- chunk
			}
			s.errChan <- io.EOF
		})
	})
	return s.dataChan, s.errChan
}

func (s *staticStream) Close() {}
