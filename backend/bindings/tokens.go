package bindings

import (
	"errors"
	"time"

	"one-api/common/config"
	"one-api/model"
)

// TokensAPI exposes token CRUD for the single-user desktop scenario.
// All operations act on the root user implicitly.
type TokensAPI struct{}

// NewTokensAPI builds the binding.
func NewTokensAPI() *TokensAPI { return &TokensAPI{} }

// TokenSummary is the list row shape.
type TokenSummary struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Key            string `json:"key"`
	Status         int    `json:"status"`
	Group          string `json:"group"`
	CreatedTime    int64  `json:"createdTime"`
	ExpiredTime    int64  `json:"expiredTime"`
	UnlimitedQuota bool   `json:"unlimitedQuota"`
	RemainQuota    int    `json:"remainQuota"`
	UsedQuota      int    `json:"usedQuota"`
}

// TokenListRequest is the frontend filter shape.
type TokenListRequest struct {
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Keyword  string `json:"keyword"`
}

// TokenListResponse is the paginated response.
type TokenListResponse struct {
	Items    []TokenSummary `json:"items"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

// List returns the root user's tokens.
func (a *TokensAPI) List(req TokenListRequest) (*TokenListResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	userID, err := rootUserID()
	if err != nil {
		return nil, err
	}
	params := &model.GenericParams{
		PaginationParams: model.PaginationParams{Page: req.Page, Size: req.PageSize},
		Keyword:          req.Keyword,
	}
	result, err := model.GetUserTokensList(userID, params)
	if err != nil {
		return nil, err
	}
	rows := []*model.Token{}
	if result.Data != nil {
		rows = *result.Data
	}
	items := make([]TokenSummary, 0, len(rows))
	for _, t := range rows {
		items = append(items, summariseToken(t))
	}
	return &TokenListResponse{
		Items:    items,
		Total:    result.TotalCount,
		Page:     req.Page,
		PageSize: req.PageSize,
	}, nil
}

// CreateTokenRequest is the payload for creating a new token.
type CreateTokenRequest struct {
	Name           string `json:"name"`
	Group          string `json:"group"`
	UnlimitedQuota bool   `json:"unlimitedQuota"`
	RemainQuota    int    `json:"remainQuota"`
	ExpiredTime    int64  `json:"expiredTime"` // -1 never
}

// Create inserts a new token.
func (a *TokensAPI) Create(req CreateTokenRequest) (*TokenSummary, error) {
	if req.Name == "" {
		return nil, errors.New("token name is required")
	}
	userID, err := rootUserID()
	if err != nil {
		return nil, err
	}
	if req.ExpiredTime == 0 {
		req.ExpiredTime = -1
	}
	if req.Group == "" {
		req.Group = "default"
	}
	tok := &model.Token{
		UserId:         userID,
		Name:           req.Name,
		Group:          req.Group,
		Status:         config.TokenStatusEnabled,
		UnlimitedQuota: req.UnlimitedQuota,
		RemainQuota:    req.RemainQuota,
		ExpiredTime:    req.ExpiredTime,
		CreatedTime:    time.Now().Unix(),
		AccessedTime:   time.Now().Unix(),
	}
	if err := tok.Insert(); err != nil {
		return nil, err
	}
	// After insert, the AfterCreate hook populates Key on the DB row but not
	// on the in-memory struct, so refetch.
	fresh, err := model.GetTokenByIds(tok.Id, userID)
	if err != nil {
		return nil, err
	}
	s := summariseToken(fresh)
	return &s, nil
}

// Delete removes a token.
func (a *TokensAPI) Delete(id int) error {
	userID, err := rootUserID()
	if err != nil {
		return err
	}
	return model.DeleteTokenById(id, userID)
}

// Toggle flips token status (enabled/disabled).
func (a *TokensAPI) Toggle(id int) (int, error) {
	userID, err := rootUserID()
	if err != nil {
		return 0, err
	}
	tok, err := model.GetTokenByIds(id, userID)
	if err != nil {
		return 0, err
	}
	if tok.Status == config.TokenStatusEnabled {
		tok.Status = config.TokenStatusDisabled
	} else {
		tok.Status = config.TokenStatusEnabled
	}
	if err := tok.Update(); err != nil {
		return 0, err
	}
	return tok.Status, nil
}

// Rename changes the token's display name.
func (a *TokensAPI) Rename(id int, name string) error {
	userID, err := rootUserID()
	if err != nil {
		return err
	}
	tok, err := model.GetTokenByIds(id, userID)
	if err != nil {
		return err
	}
	tok.Name = name
	return tok.Update()
}

// ClientSnippet generates a copy-paste configuration snippet for the given
// client kind (cursor, cline, cherry-studio) using the supplied base URL.
type SnippetRequest struct {
	TokenID int    `json:"tokenId"`
	BaseURL string `json:"baseURL"`
	Client  string `json:"client"` // cursor | cline | cherry-studio | openai-sdk
}

type SnippetResponse struct {
	Client string `json:"client"`
	Format string `json:"format"` // "json" | "text"
	Body   string `json:"body"`
}

// ClientSnippet returns ready-to-paste configuration for popular OpenAI-compat clients.
func (a *TokensAPI) ClientSnippet(req SnippetRequest) (*SnippetResponse, error) {
	userID, err := rootUserID()
	if err != nil {
		return nil, err
	}
	tok, err := model.GetTokenByIds(req.TokenID, userID)
	if err != nil {
		return nil, err
	}
	key := "sk-" + tok.Key
	base := req.BaseURL
	if base == "" {
		base = "http://127.0.0.1:3002/v1"
	}
	return &SnippetResponse{
		Client: req.Client,
		Format: snippetFormat(req.Client),
		Body:   renderSnippet(req.Client, base, key),
	}, nil
}

func rootUserID() (int, error) {
	var uid int
	if err := model.DB.Model(&model.User{}).
		Where("role = ?", config.RoleRootUser).
		Select("id").
		Limit(1).
		Scan(&uid).Error; err != nil {
		return 0, err
	}
	if uid == 0 {
		return 0, errors.New("root user not initialised yet")
	}
	return uid, nil
}

func summariseToken(t *model.Token) TokenSummary {
	return TokenSummary{
		ID:             t.Id,
		Name:           t.Name,
		Key:            t.Key,
		Status:         t.Status,
		Group:          t.Group,
		CreatedTime:    t.CreatedTime,
		ExpiredTime:    t.ExpiredTime,
		UnlimitedQuota: t.UnlimitedQuota,
		RemainQuota:    t.RemainQuota,
		UsedQuota:      t.UsedQuota,
	}
}

func snippetFormat(client string) string {
	switch client {
	case "openai-sdk":
		return "text"
	default:
		return "json"
	}
}

func renderSnippet(client, base, key string) string {
	switch client {
	case "cline":
		return `{
  "provider": "openai",
  "openaiBaseUrl": "` + base + `",
  "openaiApiKey": "` + key + `",
  "model": "gpt-4o-mini"
}`
	case "cherry-studio":
		return `{
  "name": "Prism",
  "type": "openai",
  "apiHost": "` + base + `",
  "apiKey": "` + key + `"
}`
	case "openai-sdk":
		return "OPENAI_BASE_URL=" + base + "\nOPENAI_API_KEY=" + key
	case "cursor":
		fallthrough
	default:
		return `{
  "baseUrl": "` + base + `",
  "apiKey": "` + key + `",
  "notes": "Paste into Cursor Settings -> Models -> OpenAI API Key (Custom OpenAI API Key)."
}`
	}
}
