// Package localcli wires the "LocalCLI Gateway" channel type into one-hub.
//
// LocalCLI is a thin OpenAI-compatible provider that points at a user-managed
// cli2api sidecar (https://github.com/anoxis/CLI2API). The sidecar exposes a
// pure OpenAI `/v1/chat/completions` endpoint while internally driving local
// LLM CLIs such as `claude`, `gemini`, `codex`, `qwen`, or any generic
// command-line wrapper. Because the wire protocol is already OpenAI-shaped,
// this provider reuses openai.OpenAIProvider verbatim and only overrides:
//
//   - the default Base URL (so a freshly-created channel has a sane value)
//   - the default model dropdown (so users see CLI-style model names)
//
// The actual Base URL is taken from channel.GetBaseURL() at request time, so
// users can repoint to any cli2api instance (loopback, docker network, LAN).
package localcli

import (
	"one-api/common/requester"
	"one-api/model"
	"one-api/providers/base"
	"one-api/providers/openai"
)

// DefaultBaseURL is the value pre-filled in the "Base URL" field when the
// user creates a fresh LocalCLI channel in the Prism UI. cli2api's default
// listen address is :8000 on loopback; users typically remap this.
const DefaultBaseURL = "http://127.0.0.1:8000"

// ModelList is the default set of model names surfaced in the channel
// creation dropdown. cli2api treats the model string as a hint that gets
// passed through to the underlying CLI provider, so this list is purely
// cosmetic — users may type any model name in the "custom models" field.
var ModelList = []string{
	"sonnet",
	"opus",
	"haiku",
	"gemini-2.5-pro",
	"gemini-2.5-flash",
	"codex",
	"qwen3-coder",
}

// LocalCLIProviderFactory satisfies providers.ProviderFactory.
type LocalCLIProviderFactory struct{}

// Create builds a LocalCLI provider bound to the given channel.
//
// We piggy-back on openai.RequestErrorHandle because cli2api returns errors
// in the canonical OpenAI `{"error": {"message", "type"}}` envelope.
func (f LocalCLIProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	return &LocalCLIProvider{
		OpenAIProvider: openai.OpenAIProvider{
			BaseProvider: base.BaseProvider{
				Config:    getConfig(),
				Channel:   channel,
				Requester: requester.NewHTTPRequester(*channel.Proxy, openai.RequestErrorHandle),
			},
		},
	}
}

// LocalCLIProvider is an OpenAI-compatible provider with LocalCLI defaults.
type LocalCLIProvider struct {
	openai.OpenAIProvider
}

func getConfig() base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:         DefaultBaseURL,
		ChatCompletions: "/v1/chat/completions",
		Completions:     "/v1/completions",
		ModelList:       "/v1/models",
	}
}
