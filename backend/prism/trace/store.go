// Package trace stores every upstream LLM request/response Prism proxies.
// It lives in the same sqlite as one-hub's data but in a dedicated table so
// we never collide with upstream schema migrations.
package trace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	onehubmodel "one-api/model"
)

// Entry is a single captured proxy round-trip.
//
// Diagnostic fields (RequestID/DeviceID/ChunkCount/FirstChunkAt/LastChunkAt/
// Finished/FinishReason) exist so that when a streaming SSE response gets
// truncated mid-flight the row still carries enough breadcrumbs to tell
// "stream broke after N chunks at Tms" apart from "upstream returned empty
// from the start". Without them the only visible symptom is
// `response_body=""` and there is no way to attribute the loss.
type Entry struct {
	ID         int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt  int64  `json:"createdAt" gorm:"index"`
	RequestID  string `json:"requestId" gorm:"type:varchar(96);index"`
	DeviceID   string `json:"deviceId" gorm:"type:varchar(32);index"`
	Method     string `json:"method" gorm:"type:varchar(16)"`
	Path       string `json:"path" gorm:"type:varchar(256);index"`
	Status     int    `json:"status" gorm:"index"`
	DurationMs int64  `json:"durationMs"`
	IsStream   bool   `json:"isStream"`
	// Finished is true once the upstream handler returned cleanly. False
	// rows mean the stream was still in flight when the row was last
	// flushed (process exit, client disconnect, panic, ...). Combined with
	// LastChunkAt, downstream tools can confidently mark a record as
	// "truncated" instead of "empty response".
	Finished bool `json:"finished" gorm:"index"`
	// FinishReason mirrors the OpenAI-style finish_reason of the LAST SSE
	// chunk we observed before the stream ended (stop / tool_calls /
	// length / "" if unknown).
	FinishReason  string `json:"finishReason" gorm:"type:varchar(32)"`
	ChunkCount    int    `json:"chunkCount"`
	FirstChunkAt  int64  `json:"firstChunkAt"`
	LastChunkAt   int64  `json:"lastChunkAt"`
	ChannelID     int    `json:"channelId" gorm:"index"`
	ChannelType   int    `json:"channelType"`
	ChannelName   string `json:"channelName" gorm:"type:varchar(128)"`
	TokenName     string `json:"tokenName" gorm:"type:varchar(128);index"`
	Model         string `json:"model" gorm:"type:varchar(128);index"`
	ClientIP      string `json:"clientIp" gorm:"type:varchar(64)"`
	ErrorMessage  string `json:"errorMessage" gorm:"type:text"`
	RequestBody   string `json:"requestBody" gorm:"type:longtext"`
	ResponseBody  string `json:"responseBody" gorm:"type:longtext"`
	ContentType   string `json:"contentType" gorm:"type:varchar(128)"`
	RequestBytes  int64  `json:"requestBytes"`
	ResponseBytes int64  `json:"responseBytes"`
}

// TableName overrides gorm's default pluralisation so we get a clearly
// prism-owned table instead of blending into upstream naming.
func (Entry) TableName() string { return "prism_traces" }

var (
	initOnce sync.Once
	initErr  error
)

// Init ensures the prism_traces table exists. Safe to call repeatedly.
// Only the first call runs AutoMigrate; subsequent calls are no-ops.
func Init() error {
	initOnce.Do(func() {
		if onehubmodel.DB == nil {
			initErr = errors.New("onehub DB is not ready")
			return
		}
		initErr = onehubmodel.DB.AutoMigrate(&Entry{})
	})
	return initErr
}

// Save writes an entry. Errors are logged by the caller; the trace layer
// itself never panics so a DB hiccup cannot take down the proxy.
func Save(ctx context.Context, e *Entry) error {
	if onehubmodel.DB == nil {
		return errors.New("onehub DB is not ready")
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	return onehubmodel.DB.WithContext(ctx).Create(e).Error
}

// OpenStream inserts a placeholder row at the moment a streaming response
// starts. We persist as much routing context as we already know (method,
// path, channel, model, request body) so even if the process is killed
// mid-stream we still have a row to investigate.
//
// The returned ID is the autoincrement primary key — callers pass it back
// to AppendChunk/Finalize so we never have to re-query by request_id.
func OpenStream(ctx context.Context, e *Entry) (int64, error) {
	if onehubmodel.DB == nil {
		return 0, errors.New("onehub DB is not ready")
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().Unix()
	}
	if err := onehubmodel.DB.WithContext(ctx).Create(e).Error; err != nil {
		return 0, err
	}
	return e.ID, nil
}

// StreamChunkUpdate carries an incremental flush. Callers pass the full
// accumulated body (already capped to maxBodyBytes by the middleware) so
// readers always see a self-consistent prefix. We deliberately overwrite
// rather than append at the SQL layer because gorm's column-level
// concatenation is dialect-specific and SQLite's `||` semantics differ
// from MySQL's `CONCAT`. One write of N bytes per chunk is acceptable in
// the desktop single-user workload (sub-MB rows, low QPS).
type StreamChunkUpdate struct {
	ResponseBody  string
	ResponseBytes int64
	ChunkCount    int
	FirstChunkAt  int64
	LastChunkAt   int64
}

// AppendChunk applies an incremental update for a streaming row. id == 0
// is treated as a no-op so callers in non-streaming paths can use the
// same code shape without a nil check.
func AppendChunk(ctx context.Context, id int64, u StreamChunkUpdate) error {
	if onehubmodel.DB == nil {
		return errors.New("onehub DB is not ready")
	}
	if id == 0 {
		return nil
	}
	updates := map[string]any{
		"response_body":  u.ResponseBody,
		"response_bytes": u.ResponseBytes,
		"chunk_count":    u.ChunkCount,
		"last_chunk_at":  u.LastChunkAt,
	}
	if u.FirstChunkAt > 0 {
		updates["first_chunk_at"] = u.FirstChunkAt
	}
	return onehubmodel.DB.WithContext(ctx).
		Model(&Entry{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// FinalizeStreamUpdate is the terminal patch applied once the upstream
// handler returns (cleanly or with an error). It is also safe to call on
// a non-streaming row: the streaming-only fields stay at their zero values.
type FinalizeStreamUpdate struct {
	Status        int
	DurationMs    int64
	ResponseBody  string
	ResponseBytes int64
	ContentType   string
	IsStream      bool
	ChannelID     int
	ChannelType   int
	ChannelName   string
	Model         string
	ChunkCount    int
	FirstChunkAt  int64
	LastChunkAt   int64
	Finished      bool
	FinishReason  string
	ErrorMessage  string
}

// Finalize writes the terminal state of a row created by OpenStream.
func Finalize(ctx context.Context, id int64, u FinalizeStreamUpdate) error {
	if onehubmodel.DB == nil {
		return errors.New("onehub DB is not ready")
	}
	if id == 0 {
		return nil
	}
	updates := map[string]any{
		"status":         u.Status,
		"duration_ms":    u.DurationMs,
		"response_body":  u.ResponseBody,
		"response_bytes": u.ResponseBytes,
		"content_type":   u.ContentType,
		"is_stream":      u.IsStream,
		"channel_id":     u.ChannelID,
		"channel_type":   u.ChannelType,
		"channel_name":   u.ChannelName,
		"model":          u.Model,
		"chunk_count":    u.ChunkCount,
		"last_chunk_at":  u.LastChunkAt,
		"finished":       u.Finished,
		"finish_reason":  u.FinishReason,
		"error_message":  u.ErrorMessage,
	}
	if u.FirstChunkAt > 0 {
		updates["first_chunk_at"] = u.FirstChunkAt
	}
	return onehubmodel.DB.WithContext(ctx).
		Model(&Entry{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// ListQuery is the paginated filter shape used by the bindings.
type ListQuery struct {
	Page       int
	PageSize   int
	Keyword    string
	ChannelID  int
	Model      string
	TokenName  string
	OnlyErrors bool
	// OnlyTruncated narrows results to streaming rows that never reached a
	// clean finish. Useful when investigating "no response" reports.
	OnlyTruncated bool
	RequestID     string
	StartUnix     int64
	EndUnix       int64
}

// ListResult is the paginated response shape. RequestBody/ResponseBody are
// stripped on list results; call Get(id) for the full payload.
type ListResult struct {
	Items    []Entry
	Total    int64
	Page     int
	PageSize int
}

// List returns a paginated slice of traces ordered by newest first.
func List(ctx context.Context, q ListQuery) (*ListResult, error) {
	if onehubmodel.DB == nil {
		return nil, errors.New("onehub DB is not ready")
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 || q.PageSize > 200 {
		q.PageSize = 50
	}

	db := onehubmodel.DB.WithContext(ctx).Model(&Entry{})
	if q.ChannelID > 0 {
		db = db.Where("channel_id = ?", q.ChannelID)
	}
	if q.Model != "" {
		db = db.Where("model LIKE ?", "%"+q.Model+"%")
	}
	if q.TokenName != "" {
		db = db.Where("token_name = ?", q.TokenName)
	}
	if q.OnlyErrors {
		db = db.Where("status >= 400 OR error_message <> ''")
	}
	if q.OnlyTruncated {
		// is_stream=true AND finished=false captures the exact "stream
		// started but never closed cleanly" case; the request_body is
		// already there for context.
		db = db.Where("is_stream = ? AND finished = ?", true, false)
	}
	if q.RequestID != "" {
		db = db.Where("request_id = ?", q.RequestID)
	}
	if q.StartUnix > 0 {
		db = db.Where("created_at >= ?", q.StartUnix)
	}
	if q.EndUnix > 0 {
		db = db.Where("created_at <= ?", q.EndUnix)
	}
	if q.Keyword != "" {
		like := "%" + strings.ReplaceAll(q.Keyword, "%", `\%`) + "%"
		db = db.Where("request_body LIKE ? OR response_body LIKE ? OR path LIKE ?", like, like, like)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	var rows []Entry
	err := db.Select(
		"id, created_at, request_id, device_id, method, path, status, duration_ms, "+
			"is_stream, finished, finish_reason, chunk_count, first_chunk_at, last_chunk_at, "+
			"channel_id, channel_type, channel_name, token_name, model, "+
			"client_ip, error_message, content_type, request_bytes, response_bytes",
	).Order("id DESC").
		Offset((q.Page - 1) * q.PageSize).
		Limit(q.PageSize).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return &ListResult{Items: rows, Total: total, Page: q.Page, PageSize: q.PageSize}, nil
}

// Get fetches a full trace (including bodies) by id.
func Get(ctx context.Context, id int64) (*Entry, error) {
	if onehubmodel.DB == nil {
		return nil, errors.New("onehub DB is not ready")
	}
	var e Entry
	err := onehubmodel.DB.WithContext(ctx).First(&e, id).Error
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("trace %d not found", id)
		}
		return nil, err
	}
	return &e, nil
}

// Purge deletes entries older than the given unix timestamp. Returns the
// number of rows deleted. Use 0 to wipe everything (explicit).
//
// GORM v2 refuses any Delete that has no WHERE clause (ErrMissingWhereClause)
// to guard against accidental whole-table wipes. The "purge everything" path
// is intentional, so we pass a tautological WHERE to satisfy that check
// instead of flipping AllowGlobalUpdate (which would unsafely apply session-
// wide and let unrelated future bugs nuke the table).
func Purge(ctx context.Context, olderThanUnix int64) (int64, error) {
	if onehubmodel.DB == nil {
		return 0, errors.New("onehub DB is not ready")
	}
	q := onehubmodel.DB.WithContext(ctx)
	if olderThanUnix > 0 {
		q = q.Where("created_at < ?", olderThanUnix)
	} else {
		q = q.Where("1 = 1")
	}
	res := q.Delete(&Entry{})
	return res.RowsAffected, res.Error
}

// Count returns how many traces are stored.
func Count(ctx context.Context) (int64, error) {
	if onehubmodel.DB == nil {
		return 0, errors.New("onehub DB is not ready")
	}
	var n int64
	err := onehubmodel.DB.WithContext(ctx).Model(&Entry{}).Count(&n).Error
	return n, err
}
