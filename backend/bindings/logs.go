package bindings

import (
	"context"
	"sync"

	"one-api/common/logger"
	"one-api/model"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// LogsAPI exposes request logs (from the DB) and system logs (from the
// in-memory logger ring buffer + fan-out subscription) to the frontend.
type LogsAPI struct {
	ctx   context.Context
	once  sync.Once
	unsub func()
}

// NewLogsAPI creates the binding. Call SetContext during Wails startup.
func NewLogsAPI() *LogsAPI { return &LogsAPI{} }

// SetContext stashes the Wails runtime context so EventsEmit works. Guards
// against a nil ctx from accidental frontend invocation.
func (a *LogsAPI) SetContext(ctx context.Context) {
	if ctx == nil {
		return
	}
	a.ctx = ctx
}

// Shutdown tears down the subscription.
func (a *LogsAPI) Shutdown() {
	if a.unsub != nil {
		a.unsub()
	}
}

// LogEntry is the JSON-friendly entry shape.
type LogEntry struct {
	Timestamp int64  `json:"timestamp"` // unix millis
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// SystemLogs returns the latest N entries from the in-memory ring buffer.
func (a *LogsAPI) SystemLogs(n int) []LogEntry {
	if n <= 0 {
		n = 200
	}
	entries := logger.GetLatestEntries(n)
	out := make([]LogEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, LogEntry{
			Timestamp: e.Timestamp.UnixMilli(),
			Level:     e.Level,
			Message:   e.Message,
		})
	}
	return out
}

// StartSystemLogStream subscribes to the logger fan-out and emits `log.system`
// events for each new entry. Safe to call multiple times; only the first call
// wins.
func (a *LogsAPI) StartSystemLogStream() {
	a.once.Do(func() {
		_, ch, unsub := logger.Subscribe(512)
		a.unsub = unsub
		go func() {
			for e := range ch {
				if a.ctx == nil {
					continue
				}
				runtime.EventsEmit(a.ctx, "log.system", LogEntry{
					Timestamp: e.Timestamp.UnixMilli(),
					Level:     e.Level,
					Message:   e.Message,
				})
			}
		}()
	})
}

// RequestLogsRequest filters the request log query.
type RequestLogsRequest struct {
	Page           int    `json:"page"`
	PageSize       int    `json:"pageSize"`
	LogType        int    `json:"logType"`
	StartTimestamp int64  `json:"startTimestamp"`
	EndTimestamp   int64  `json:"endTimestamp"`
	ModelName      string `json:"modelName"`
	Username       string `json:"username"`
	TokenName      string `json:"tokenName"`
	ChannelID      int    `json:"channelId"`
	SourceIP       string `json:"sourceIp"`
}

// RequestLogRow is the flattened log row for the UI.
type RequestLogRow struct {
	ID               int    `json:"id"`
	CreatedAt        int64  `json:"createdAt"`
	Type             int    `json:"type"`
	Username         string `json:"username"`
	ModelName        string `json:"modelName"`
	TokenName        string `json:"tokenName"`
	ChannelID        int    `json:"channelId"`
	ChannelName      string `json:"channelName"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	RequestTime      int    `json:"requestTime"`
	IsStream         bool   `json:"isStream"`
	SourceIP         string `json:"sourceIp"`
	Content          string `json:"content"`
}

// RequestLogsResponse is the paginated response.
type RequestLogsResponse struct {
	Items    []RequestLogRow `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

// RequestLogs returns a paginated query of the request log table.
func (a *LogsAPI) RequestLogs(req RequestLogsRequest) (*RequestLogsResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	params := &model.LogsListParams{
		PaginationParams: model.PaginationParams{
			Page: req.Page,
			Size: req.PageSize,
		},
		LogType:        req.LogType,
		StartTimestamp: req.StartTimestamp,
		EndTimestamp:   req.EndTimestamp,
		ModelName:      req.ModelName,
		Username:       req.Username,
		TokenName:      req.TokenName,
		ChannelId:      req.ChannelID,
		SourceIp:       req.SourceIP,
	}
	result, err := model.GetLogsList(params)
	if err != nil {
		return nil, err
	}
	rows := []*model.Log{}
	if result.Data != nil {
		rows = *result.Data
	}
	out := make([]RequestLogRow, 0, len(rows))
	for _, r := range rows {
		chName := ""
		if r.Channel != nil {
			chName = r.Channel.Name
		}
		out = append(out, RequestLogRow{
			ID:               r.Id,
			CreatedAt:        r.CreatedAt,
			Type:             r.Type,
			Username:         r.Username,
			ModelName:        r.ModelName,
			TokenName:        r.TokenName,
			ChannelID:        r.ChannelId,
			ChannelName:      chName,
			Quota:            r.Quota,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			RequestTime:      r.RequestTime,
			IsStream:         r.IsStream,
			SourceIP:         r.SourceIp,
			Content:          r.Content,
		})
	}
	return &RequestLogsResponse{
		Items:    out,
		Total:    result.TotalCount,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}
