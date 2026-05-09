package bindings

import (
	"errors"
	"fmt"
	"time"

	"one-api/common/config"
	"one-api/controller"
	"one-api/model"
)

// ChannelsAPI exposes channel CRUD + test flow to the Wails frontend.
type ChannelsAPI struct{}

// NewChannelsAPI constructs the binding.
func NewChannelsAPI() *ChannelsAPI { return &ChannelsAPI{} }

// ProviderMeta describes a supported channel provider type.
type ProviderMeta struct {
	Type int    `json:"type"`
	Name string `json:"name"`
}

// ListProviderTypes returns the set of channel provider types Prism knows about.
func (a *ChannelsAPI) ListProviderTypes() []ProviderMeta {
	return []ProviderMeta{
		{config.ChannelTypeOpenAI, "OpenAI"},
		{config.ChannelTypeChatGPTSubscription, "ChatGPT Subscription (Codex CLI)"},
		{config.ChannelTypeAzure, "Azure OpenAI"},
		{config.ChannelTypeCustom, "Custom (OpenAI-compatible)"},
		{config.ChannelTypeAnthropic, "Anthropic"},
		{config.ChannelTypeClaudeSubscription, "Claude Subscription (Claude CLI)"},
		{config.ChannelTypeGemini, "Google Gemini"},
		{config.ChannelTypeBedrock, "Amazon Bedrock"},
		{config.ChannelTypeVertexAI, "Google Vertex AI"},
		{config.ChannelTypeBaidu, "Baidu Wenxin"},
		{config.ChannelTypeZhipu, "Zhipu GLM"},
		{config.ChannelTypeAli, "Ali Qwen"},
		{config.ChannelTypeXunfei, "Xunfei Spark"},
		{config.ChannelType360, "360"},
		{config.ChannelTypeOpenRouter, "OpenRouter"},
		{config.ChannelTypeTencent, "Tencent Hunyuan"},
		{config.ChannelTypeBaichuan, "Baichuan"},
		{config.ChannelTypeMiniMax, "MiniMax"},
		{config.ChannelTypeDeepseek, "DeepSeek"},
		{config.ChannelTypeMoonshot, "Moonshot"},
		{config.ChannelTypeMistral, "Mistral"},
		{config.ChannelTypeGroq, "Groq"},
		{config.ChannelTypeLingyi, "01.ai"},
		{config.ChannelTypeCloudflareAI, "Cloudflare Workers AI"},
		{config.ChannelTypeCohere, "Cohere"},
		{config.ChannelTypeStabilityAI, "Stability AI"},
		{config.ChannelTypeCoze, "Coze"},
		{config.ChannelTypeOllama, "Ollama"},
		{config.ChannelTypeHunyuan, "Hunyuan"},
		{config.ChannelTypeLLAMA, "Llama"},
		{config.ChannelTypeIdeogram, "Ideogram"},
		{config.ChannelTypeSiliconflow, "Silicon Flow"},
		{config.ChannelTypeFlux, "Flux"},
		{config.ChannelTypeJina, "Jina"},
		{config.ChannelTypeMidjourney, "Midjourney"},
	}
}

// ChannelListRequest is the shape the frontend posts.
type ChannelListRequest struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Status   int    `json:"status"`
	OrderBy  string `json:"orderBy"`
	SortBy   string `json:"sortBy"`
}

// ChannelSummary is the list row shape (key omitted for security).
type ChannelSummary struct {
	ID           int     `json:"id"`
	Type         int     `json:"type"`
	Name         string  `json:"name"`
	Status       int     `json:"status"`
	Priority     int64   `json:"priority"`
	Weight       uint    `json:"weight"`
	Models       string  `json:"models"`
	Group        string  `json:"group"`
	BaseURL      string  `json:"baseURL"`
	TestModel    string  `json:"testModel"`
	Proxy        string  `json:"proxy"`
	ResponseTime int     `json:"responseTime"`
	Balance      float64 `json:"balance"`
	UsedQuota    int64   `json:"usedQuota"`
	CreatedTime  int64   `json:"createdTime"`
}

// ChannelListResponse wraps the paginated result.
type ChannelListResponse struct {
	Items    []ChannelSummary `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}

// List returns a paginated list of channels.
func (a *ChannelsAPI) List(req ChannelListRequest) (*ChannelListResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	params := &model.SearchChannelsParams{
		PaginationParams: model.PaginationParams{
			Page:  req.Page,
			Size:  req.PageSize,
			Order: req.OrderBy,
		},
	}
	params.Name = req.Name
	params.Type = req.Type
	params.Status = req.Status

	result, err := model.GetChannelsList(params)
	if err != nil {
		return nil, err
	}
	rows := []*model.Channel{}
	if result.Data != nil {
		rows = *result.Data
	}
	items := make([]ChannelSummary, 0, len(rows))
	for _, c := range rows {
		items = append(items, summariseChannel(c))
	}
	return &ChannelListResponse{
		Items:    items,
		Total:    result.TotalCount,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

func summariseChannel(c *model.Channel) ChannelSummary {
	base := ""
	if c.BaseURL != nil {
		base = *c.BaseURL
	}
	var priority int64
	if c.Priority != nil {
		priority = *c.Priority
	}
	var weight uint
	if c.Weight != nil {
		weight = *c.Weight
	}
	proxy := ""
	if c.Proxy != nil {
		proxy = *c.Proxy
	}
	return ChannelSummary{
		ID:           c.Id,
		Type:         c.Type,
		Name:         c.Name,
		Status:       c.Status,
		Priority:     priority,
		Weight:       weight,
		Models:       c.Models,
		Group:        c.Group,
		BaseURL:      base,
		TestModel:    c.TestModel,
		Proxy:        proxy,
		ResponseTime: c.ResponseTime,
		Balance:      c.Balance,
		UsedQuota:    c.UsedQuota,
		CreatedTime:  c.CreatedTime,
	}
}

// ChannelPayload is the full shape required to create or update a channel.
type ChannelPayload struct {
	ID        int    `json:"id"`
	Type      int    `json:"type"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	BaseURL   string `json:"baseURL"`
	Other     string `json:"other"`
	Models    string `json:"models"`
	Group     string `json:"group"`
	TestModel string `json:"testModel"`
	Proxy     string `json:"proxy"`
	Priority  int64  `json:"priority"`
	Weight    uint   `json:"weight"`
	Status    int    `json:"status"`
}

// Get returns a single channel by ID (including the key) as a summary-plus
// shape that's safe to serialise to JSON (model.Channel contains
// datatypes.JSONType which the Wails binding generator chokes on).
type ChannelDetail struct {
	ChannelSummary
	Key   string `json:"key"`
	Other string `json:"other"`
}

func detailFromChannel(c *model.Channel) ChannelDetail {
	return ChannelDetail{
		ChannelSummary: summariseChannel(c),
		Key:            c.Key,
		Other:          c.Other,
	}
}

// Get returns a single channel by ID.
func (a *ChannelsAPI) Get(id int) (*ChannelDetail, error) {
	ch, err := model.GetChannelById(id)
	if err != nil {
		return nil, err
	}
	d := detailFromChannel(ch)
	return &d, nil
}

// Create inserts a new channel.
func (a *ChannelsAPI) Create(p ChannelPayload) (*ChannelDetail, error) {
	if p.Name == "" {
		return nil, errors.New("channel name is required")
	}
	if p.Type == 0 {
		return nil, errors.New("channel type is required")
	}
	ch := channelFromPayload(p)
	ch.CreatedTime = time.Now().Unix()
	if ch.Status == 0 {
		ch.Status = config.ChannelStatusEnabled
	}
	if err := ch.Insert(); err != nil {
		return nil, err
	}
	d := detailFromChannel(ch)
	return &d, nil
}

// Update modifies an existing channel.
func (a *ChannelsAPI) Update(p ChannelPayload) (*ChannelDetail, error) {
	if p.ID == 0 {
		return nil, errors.New("channel id is required")
	}
	existing, err := model.GetChannelById(p.ID)
	if err != nil {
		return nil, err
	}
	applyPayload(existing, p)
	if err := existing.Update(true); err != nil {
		return nil, err
	}
	d := detailFromChannel(existing)
	return &d, nil
}

// Delete removes a channel permanently.
func (a *ChannelsAPI) Delete(id int) error {
	ch, err := model.GetChannelById(id)
	if err != nil {
		return err
	}
	return ch.Delete()
}

// Toggle flips a channel between enabled / manually disabled.
func (a *ChannelsAPI) Toggle(id int) (int, error) {
	ch, err := model.GetChannelById(id)
	if err != nil {
		return 0, err
	}
	if ch.Status == config.ChannelStatusEnabled {
		ch.Status = config.ChannelStatusManuallyDisabled
	} else {
		ch.Status = config.ChannelStatusEnabled
	}
	if err := ch.Update(true); err != nil {
		return 0, err
	}
	return ch.Status, nil
}

// TestResult summarises a channel test invocation.
type TestResult struct {
	Success      bool   `json:"success"`
	ResponseTime int    `json:"responseTime"`
	Message      string `json:"message"`
}

// Test runs a live provider ping for the given channel and optional model.
// Falls back to the channel's TestModel when modelName is empty.
func (a *ChannelsAPI) Test(id int, modelName string) (*TestResult, error) {
	ch, err := model.GetChannelById(id)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	openaiErr, testErr := controller.TestChannelOnce(ch, modelName)
	elapsed := int(time.Since(start).Milliseconds())
	ch.UpdateResponseTime(int64(elapsed))

	if testErr != nil {
		msg := testErr.Error()
		if openaiErr != nil && openaiErr.Message != "" {
			msg = openaiErr.Message
		}
		return &TestResult{
			Success:      false,
			ResponseTime: elapsed,
			Message:      fmt.Sprintf("channel %d (%s) failed: %s", ch.Id, ch.Name, msg),
		}, nil
	}
	return &TestResult{
		Success:      true,
		ResponseTime: elapsed,
		Message:      fmt.Sprintf("channel %d (%s) ok in %dms", ch.Id, ch.Name, elapsed),
	}, nil
}

func channelFromPayload(p ChannelPayload) *model.Channel {
	base := p.BaseURL
	proxy := p.Proxy
	ch := &model.Channel{
		Id:        p.ID,
		Type:      p.Type,
		Name:      p.Name,
		Key:       p.Key,
		BaseURL:   &base,
		Proxy:     &proxy,
		Other:     p.Other,
		Models:    p.Models,
		Group:     p.Group,
		TestModel: p.TestModel,
		Status:    p.Status,
	}
	priority := p.Priority
	ch.Priority = &priority
	weight := p.Weight
	ch.Weight = &weight
	return ch
}

func applyPayload(existing *model.Channel, p ChannelPayload) {
	existing.Type = p.Type
	existing.Name = p.Name
	if p.Key != "" {
		existing.Key = p.Key
	}
	base := p.BaseURL
	existing.BaseURL = &base
	proxy := p.Proxy
	existing.Proxy = &proxy
	existing.Other = p.Other
	existing.Models = p.Models
	existing.Group = p.Group
	existing.TestModel = p.TestModel
	priority := p.Priority
	existing.Priority = &priority
	weight := p.Weight
	existing.Weight = &weight
	if p.Status != 0 {
		existing.Status = p.Status
	}
}
