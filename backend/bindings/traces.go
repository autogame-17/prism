package bindings

import (
	"context"

	"prism/backend/prism/trace"
)

// TracesAPI exposes the prism_traces capture store to the frontend. Each
// method proxies into prism/prism/trace and returns typed rows that the
// Wails binding generator can serialise cleanly.
type TracesAPI struct {
	ctx context.Context
}

func NewTracesAPI() *TracesAPI { return &TracesAPI{} }

func (a *TracesAPI) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	a.ctx = ctx
}

// TraceSummary is the list row. Bodies are omitted here; call Get(id) to
// read the full captured request + response.
type TraceSummary struct {
	ID            int64  `json:"id"`
	CreatedAt     int64  `json:"createdAt"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Status        int    `json:"status"`
	DurationMs    int64  `json:"durationMs"`
	IsStream      bool   `json:"isStream"`
	ChannelID     int    `json:"channelId"`
	ChannelName   string `json:"channelName"`
	TokenName     string `json:"tokenName"`
	Model         string `json:"model"`
	ClientIP      string `json:"clientIp"`
	ContentType   string `json:"contentType"`
	RequestBytes  int64  `json:"requestBytes"`
	ResponseBytes int64  `json:"responseBytes"`
	ErrorMessage  string `json:"errorMessage"`
}

// TraceDetail includes the captured bodies.
type TraceDetail struct {
	TraceSummary
	RequestBody  string `json:"requestBody"`
	ResponseBody string `json:"responseBody"`
}

// TracesListRequest is the paginated filter payload.
type TracesListRequest struct {
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	Keyword    string `json:"keyword"`
	ChannelID  int    `json:"channelId"`
	Model      string `json:"model"`
	TokenName  string `json:"tokenName"`
	OnlyErrors bool   `json:"onlyErrors"`
	StartUnix  int64  `json:"startUnix"`
	EndUnix    int64  `json:"endUnix"`
}

// TracesListResponse is the paginated response.
type TracesListResponse struct {
	Items    []TraceSummary `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

// List returns recent trace summaries. Bodies are stripped.
func (a *TracesAPI) List(req TracesListRequest) (*TracesListResponse, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := trace.List(ctx, trace.ListQuery{
		Page:       req.Page,
		PageSize:   req.PageSize,
		Keyword:    req.Keyword,
		ChannelID:  req.ChannelID,
		Model:      req.Model,
		TokenName:  req.TokenName,
		OnlyErrors: req.OnlyErrors,
		StartUnix:  req.StartUnix,
		EndUnix:    req.EndUnix,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TraceSummary, 0, len(res.Items))
	for _, e := range res.Items {
		items = append(items, TraceSummary{
			ID:            e.ID,
			CreatedAt:     e.CreatedAt,
			Method:        e.Method,
			Path:          e.Path,
			Status:        e.Status,
			DurationMs:    e.DurationMs,
			IsStream:      e.IsStream,
			ChannelID:     e.ChannelID,
			ChannelName:   e.ChannelName,
			TokenName:     e.TokenName,
			Model:         e.Model,
			ClientIP:      e.ClientIP,
			ContentType:   e.ContentType,
			RequestBytes:  e.RequestBytes,
			ResponseBytes: e.ResponseBytes,
			ErrorMessage:  e.ErrorMessage,
		})
	}
	return &TracesListResponse{
		Items:    items,
		Total:    res.Total,
		Page:     res.Page,
		PageSize: res.PageSize,
	}, nil
}

// Get returns a single trace with both bodies.
func (a *TracesAPI) Get(id int64) (*TraceDetail, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	e, err := trace.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &TraceDetail{
		TraceSummary: TraceSummary{
			ID:            e.ID,
			CreatedAt:     e.CreatedAt,
			Method:        e.Method,
			Path:          e.Path,
			Status:        e.Status,
			DurationMs:    e.DurationMs,
			IsStream:      e.IsStream,
			ChannelID:     e.ChannelID,
			ChannelName:   e.ChannelName,
			TokenName:     e.TokenName,
			Model:         e.Model,
			ClientIP:      e.ClientIP,
			ContentType:   e.ContentType,
			RequestBytes:  e.RequestBytes,
			ResponseBytes: e.ResponseBytes,
			ErrorMessage:  e.ErrorMessage,
		},
		RequestBody:  e.RequestBody,
		ResponseBody: e.ResponseBody,
	}, nil
}

// PurgeOlderThan deletes traces older than the given unix timestamp (0 = all).
func (a *TracesAPI) PurgeOlderThan(olderThanUnix int64) (int64, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.Purge(ctx, olderThanUnix)
}

// Count returns the total captured trace count.
func (a *TracesAPI) Count() (int64, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return trace.Count(ctx)
}
