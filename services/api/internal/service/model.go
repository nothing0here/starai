package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ModelService struct {
	db *pgxpool.Pool
}

func NewModelService(db *pgxpool.Pool) *ModelService {
	return &ModelService{db: db}
}

type ModelDTO struct {
	ID            int64                  `json:"id"`
	Code          string                 `json:"code"`
	DisplayName   string                 `json:"display_name"`
	Category      string                 `json:"category"`
	IconURL       *string                `json:"icon_url,omitempty"`
	Description   *string                `json:"description,omitempty"`
	Tags          []string               `json:"tags"`
	RuntimeRule   map[string]interface{} `json:"runtime_rule,omitempty"`
	InputSchema   map[string]interface{} `json:"input_schema"`
	DefaultParams map[string]interface{} `json:"default_params"`
	PriceRule     map[string]interface{} `json:"price_rule"`
	IsEnabled     bool                   `json:"is_enabled"`
	SortOrder     int                    `json:"sort_order"`
}

func (s *ModelService) ListPublic(ctx context.Context, category string) ([]ModelDTO, error) {
	q := `SELECT id, code, display_name, category, icon_url, description, tags, runtime_rule, input_schema, default_params, price_rule, is_enabled, sort_order
		FROM models WHERE is_enabled=true`
	args := []interface{}{}
	if category != "" && category != "all" {
		if category == "chat" {
			q += ` AND category IN ('chat','multi_collab')`
		} else {
			q += ` AND category=$1`
			args = append(args, category)
		}
	}
	q += ` ORDER BY sort_order ASC, id ASC`
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanModels(rows)
}

func (s *ModelService) GetByCode(ctx context.Context, code string, publicOnly bool) (*ModelDTO, error) {
	q := `SELECT id, code, display_name, category, icon_url, description, tags, runtime_rule, input_schema, default_params, price_rule, is_enabled, sort_order
		FROM models WHERE code=$1`
	if publicOnly {
		q += ` AND is_enabled=true`
	}
	var m ModelDTO
	var tags, runtime, schema, defaults, price []byte
	err := s.db.QueryRow(ctx, q, code).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.Category, &m.IconURL, &m.Description,
		&tags, &runtime, &schema, &defaults, &price, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("model not found")
		}
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(runtime, &m.RuntimeRule)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(price, &m.PriceRule)
	return &m, nil
}

func (s *ModelService) GetByID(ctx context.Context, id int64) (*ModelDTO, error) {
	var m ModelDTO
	var tags, runtime, schema, defaults, price []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, code, display_name, category, icon_url, description, tags, runtime_rule, input_schema, default_params, price_rule, is_enabled, sort_order
		FROM models WHERE id=$1`, id).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.Category, &m.IconURL, &m.Description,
		&tags, &runtime, &schema, &defaults, &price, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(runtime, &m.RuntimeRule)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(price, &m.PriceRule)
	return &m, nil
}

type ModelFull struct {
	ModelDTO
	NewAPIModel       string                 `json:"new_api_model"`
	NewAPIEndpoint    string                 `json:"new_api_endpoint"`
	RequestMode       string                 `json:"request_mode"`
	NewAPIExtraParams map[string]interface{} `json:"new_api_extra_params"`
	RuntimeRule       map[string]interface{} `json:"runtime_rule"`
	RetentionDays     int                    `json:"retention_days"`
}

func (s *ModelService) GetFullByCode(ctx context.Context, code string) (*ModelFull, error) {
	var m ModelFull
	var tags, schema, defaults, price, extra, runtime []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, code, display_name, new_api_model, new_api_endpoint, request_mode, category,
			icon_url, description, tags, input_schema, default_params, new_api_extra_params, price_rule, runtime_rule,
			retention_days, is_enabled, sort_order
		FROM models WHERE code=$1 AND is_enabled=true`, code).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.NewAPIModel, &m.NewAPIEndpoint, &m.RequestMode, &m.Category,
		&m.IconURL, &m.Description, &tags, &schema, &defaults, &extra, &price, &runtime,
		&m.RetentionDays, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("model not found")
		}
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(extra, &m.NewAPIExtraParams)
	json.Unmarshal(price, &m.PriceRule)
	json.Unmarshal(runtime, &m.RuntimeRule)
	return &m, nil
}

// GetFullByIDForAdmin returns the complete stored upstream configuration for
// admin-only operations such as connection testing. Unlike GetFullByCode it
// intentionally includes disabled models, so an administrator can verify a
// model before enabling it.
func (s *ModelService) GetFullByIDForAdmin(ctx context.Context, id int64) (*ModelFull, error) {
	var m ModelFull
	var tags, schema, defaults, price, extra, runtimeRule []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, code, display_name, new_api_model, new_api_endpoint, request_mode, category,
			icon_url, description, tags, input_schema, default_params, new_api_extra_params, price_rule, runtime_rule,
			retention_days, is_enabled, sort_order
		FROM models WHERE id=$1`, id).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.NewAPIModel, &m.NewAPIEndpoint, &m.RequestMode, &m.Category,
		&m.IconURL, &m.Description, &tags, &schema, &defaults, &extra, &price, &runtimeRule,
		&m.RetentionDays, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("模型不存在")
		}
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(extra, &m.NewAPIExtraParams)
	json.Unmarshal(price, &m.PriceRule)
	json.Unmarshal(runtimeRule, &m.RuntimeRule)
	return &m, nil
}

func (s *ModelService) ResolveChatModel(ctx context.Context, identifier string) (*ModelFull, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, errors.New("model not found")
	}
	if model, err := s.GetFullByCode(ctx, identifier); err == nil {
		return model, nil
	} else if !errors.Is(err, pgx.ErrNoRows) && err.Error() != "model not found" {
		return nil, err
	}

	var m ModelFull
	var tags, schema, defaults, price, extra, runtime []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, code, display_name, new_api_model, new_api_endpoint, request_mode, category,
			icon_url, description, tags, input_schema, default_params, new_api_extra_params, price_rule, runtime_rule,
			retention_days, is_enabled, sort_order
		FROM models
		WHERE is_enabled=true
		  AND new_api_model=$1
		  AND request_mode='chat_completions'
		ORDER BY sort_order ASC, id ASC
		LIMIT 1`, identifier).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.NewAPIModel, &m.NewAPIEndpoint, &m.RequestMode, &m.Category,
		&m.IconURL, &m.Description, &tags, &schema, &defaults, &extra, &price, &runtime,
		&m.RetentionDays, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("model not found")
		}
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(extra, &m.NewAPIExtraParams)
	json.Unmarshal(price, &m.PriceRule)
	json.Unmarshal(runtime, &m.RuntimeRule)
	if m.RetentionDays <= 0 {
		m.RetentionDays = 7
	}
	return &m, nil
}

func (s *ModelService) ResolveTaskModel(ctx context.Context, identifier string, requestModes ...string) (*ModelFull, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, errors.New("model not found")
	}
	allowed := map[string]bool{}
	for _, mode := range requestModes {
		mode = strings.TrimSpace(mode)
		if mode != "" {
			allowed[mode] = true
		}
	}
	matchesMode := func(model *ModelFull) bool {
		if len(allowed) == 0 {
			return model.RequestMode == "images" || model.RequestMode == "video" || model.RequestMode == "audio"
		}
		return allowed[model.RequestMode]
	}
	if model, err := s.GetFullByCode(ctx, identifier); err == nil {
		if matchesMode(model) {
			return model, nil
		}
		return nil, errors.New("model not found")
	} else if err.Error() != "model not found" {
		return nil, err
	}

	var m ModelFull
	var tags, schema, defaults, price, extra, runtime []byte
	args := []interface{}{identifier}
	modeSQL := ""
	if len(allowed) > 0 {
		i := 2
		placeholders := []string{}
		for mode := range allowed {
			args = append(args, mode)
			placeholders = append(placeholders, fmt.Sprintf("$%d", i))
			i++
		}
		modeSQL = " AND request_mode IN (" + strings.Join(placeholders, ",") + ")"
	} else {
		modeSQL = " AND request_mode IN ('images','video','audio')"
	}
	err := s.db.QueryRow(ctx, `
		SELECT id, code, display_name, new_api_model, new_api_endpoint, request_mode, category,
			icon_url, description, tags, input_schema, default_params, new_api_extra_params, price_rule, runtime_rule,
			retention_days, is_enabled, sort_order
		FROM models
		WHERE is_enabled=true
		  AND new_api_model=$1`+modeSQL+`
		ORDER BY sort_order ASC, id ASC
		LIMIT 1`, args...).Scan(
		&m.ID, &m.Code, &m.DisplayName, &m.NewAPIModel, &m.NewAPIEndpoint, &m.RequestMode, &m.Category,
		&m.IconURL, &m.Description, &tags, &schema, &defaults, &extra, &price, &runtime,
		&m.RetentionDays, &m.IsEnabled, &m.SortOrder)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("model not found")
		}
		return nil, err
	}
	json.Unmarshal(tags, &m.Tags)
	json.Unmarshal(schema, &m.InputSchema)
	json.Unmarshal(defaults, &m.DefaultParams)
	json.Unmarshal(extra, &m.NewAPIExtraParams)
	json.Unmarshal(price, &m.PriceRule)
	json.Unmarshal(runtime, &m.RuntimeRule)
	if m.RetentionDays <= 0 {
		m.RetentionDays = 7
	}
	return &m, nil
}

func (s *ModelService) EstimateCost(model *ModelFull, params map[string]interface{}, promptTokens, outputTokens int) float64 {
	billingType, _ := model.PriceRule["billing_type"].(string)
	switch billingType {
	case "per_image":
		unitPrice := floatValue(model.PriceRule["unit_price"])
		n := floatValue(params["n"])
		if n <= 0 {
			n = floatValue(params["count"])
		}
		if n <= 0 {
			n = 1
		}
		return unitPrice * n
	case "per_token":
		promptTokens, outputTokens = estimatedTokenCounts(model.PriceRule, params, promptTokens, outputTokens)
		return tokenCostFromRule(model.PriceRule, promptTokens, outputTokens, 0, 0) * modelTokenItemCount(model, params)
	case "per_request":
		return floatValue(model.PriceRule["unit_price"])
	case "per_second":
		unitPrice := floatValue(model.PriceRule["unit_price"])
		duration := parseDurationSeconds(params)
		return unitPrice * duration * billingItemCount(params)
	case "dynamic":
		return estimateDynamicCost(model.PriceRule, params)
	default:
		return 0
	}
}

func (s *ModelService) EstimateCostWithTokenDetails(model *ModelFull, params map[string]interface{}, promptTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	if stringValue(model.PriceRule["billing_type"]) != "per_token" {
		return s.EstimateCost(model, params, promptTokens, outputTokens)
	}
	if promptTokens <= 0 && outputTokens <= 0 {
		promptTokens, outputTokens = estimatedTokenCounts(model.PriceRule, params, promptTokens, outputTokens)
	} else if promptTokens <= 0 {
		promptTokens, _ = estimatedTokenCounts(model.PriceRule, params, promptTokens, 1)
	}
	return tokenCostFromRule(model.PriceRule, promptTokens, outputTokens, cacheReadTokens, cacheWriteTokens) * modelTokenItemCount(model, params)
}

func modelTokenItemCount(model *ModelFull, params map[string]interface{}) float64 {
	if model != nil && model.Category == "audio" {
		return billingItemCount(params)
	}
	return 1
}

func billingItemCount(params map[string]interface{}) float64 {
	count := floatValue(params["count"])
	if count <= 0 {
		count = floatValue(params["n"])
	}
	if count <= 0 {
		return 1
	}
	return count
}

func tokenCostFromRule(rule map[string]interface{}, promptTokens, outputTokens, cacheReadTokens, cacheWriteTokens int) float64 {
	if promptTokens < 0 {
		promptTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cacheReadTokens < 0 {
		cacheReadTokens = 0
	}
	if cacheWriteTokens < 0 {
		cacheWriteTokens = 0
	}
	if cacheReadTokens+cacheWriteTokens > promptTokens {
		overflow := cacheReadTokens + cacheWriteTokens - promptTokens
		if cacheWriteTokens >= overflow {
			cacheWriteTokens -= overflow
		} else {
			cacheReadTokens = promptTokens - cacheWriteTokens
		}
	}
	uncachedInput := promptTokens - cacheReadTokens - cacheWriteTokens
	inputPrice := perTokenPrice(rule, "input_price")
	outputPrice := perTokenPrice(rule, "output_price")
	cacheReadPrice := perTokenPrice(rule, "cache_read_price")
	if cacheReadPrice <= 0 {
		cacheReadPrice = inputPrice
	}
	cacheWritePrice := perTokenPrice(rule, "cache_write_price")
	if cacheWritePrice <= 0 {
		cacheWritePrice = inputPrice
	}
	cost := float64(uncachedInput)*inputPrice + float64(cacheReadTokens)*cacheReadPrice + float64(cacheWriteTokens)*cacheWritePrice + float64(outputTokens)*outputPrice
	if surcharge := floatValue(rule["surcharge_per_m"]); surcharge > 0 {
		cost += float64(promptTokens+outputTokens) / 1_000_000 * surcharge
	}
	return cost
}

func estimatedTokenCounts(rule, params map[string]interface{}, promptTokens, outputTokens int) (int, int) {
	if promptTokens <= 0 {
		promptTokens = firstPositiveIntValue(params, "_estimated_input_tokens", "estimated_input_tokens")
		if promptTokens <= 0 {
			promptTokens = firstPositiveIntValue(rule, "estimated_input_tokens")
		}
		if promptTokens <= 0 {
			promptTokens = estimateTextTokens(stringValue(params["prompt"]))
		}
		if promptTokens <= 0 {
			promptTokens = 500
		}
	}
	if outputTokens <= 0 {
		outputTokens = firstPositiveIntValue(params, "_estimated_output_tokens", "max_completion_tokens", "max_tokens")
		if outputTokens <= 0 {
			outputTokens = firstPositiveIntValue(rule, "estimated_output_tokens")
		}
		if outputTokens <= 0 {
			outputTokens = 1000
		}
	}
	return promptTokens, outputTokens
}

func firstPositiveIntValue(values map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if value := int(math.Ceil(floatValue(values[key]))); value > 0 {
			return value
		}
	}
	return 0
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := []rune(text)
	weighted := 0.0
	for _, r := range runes {
		if r <= 127 {
			weighted += 0.25
		} else {
			weighted += 0.75
		}
	}
	return int(math.Ceil(weighted)) + 8
}

func estimateDynamicCost(rule map[string]interface{}, params map[string]interface{}) float64 {
	switch strings.ToLower(strings.TrimSpace(stringValue(rule["strategy"]))) {
	case "seedance_2_tokens":
		return estimateSeedance2TokenCost(rule, params)
	case "minimax_h3_seconds":
		return estimateMiniMaxH3Cost(rule, params)
	default:
		return floatValue(rule["fallback_cost"])
	}
}

func estimateMiniMaxH3Cost(rule map[string]interface{}, params map[string]interface{}) float64 {
	resolution := strings.ToLower(strings.TrimSpace(stringValue(params["resolution"])))
	if resolution == "" {
		resolution = strings.ToLower(strings.TrimSpace(stringValue(rule["default_resolution"])))
	}
	if resolution == "" {
		resolution = "2k"
	}
	rate := mapFloatValue(rule["rates_per_second"], resolution)
	if rate <= 0 {
		rate = map[string]float64{"2k": 0.8, "768p": 0.5}[resolution]
	}
	if rate <= 0 {
		return floatValue(rule["fallback_cost"])
	}

	outputSeconds := parseDurationSeconds(params)
	videoCount := urlFieldCount(params["reference_videos"])
	inputSeconds := floatValue(params["reference_video_duration_seconds"])
	if videoCount > 0 && inputSeconds <= 0 {
		inputSeconds = float64(videoCount) * floatValue(rule["default_input_video_seconds"])
		if inputSeconds <= 0 {
			inputSeconds = float64(videoCount) * 4
		}
	}
	imageCount := urlFieldCount(params["reference_images"])
	imageCount += urlFieldCount(params["first_frame"])
	imageCount += urlFieldCount(params["last_frame"])
	freeImages := int(floatValue(rule["free_reference_images"]))
	if freeImages < 0 {
		freeImages = 0
	}
	excessImages := imageCount - freeImages
	if excessImages < 0 {
		excessImages = 0
	}
	imagePrice := floatValue(rule["excess_image_price"])
	if imagePrice <= 0 {
		imagePrice = 0.2
	}
	pointsPerCNY := floatValue(rule["points_per_cny"])
	if pointsPerCNY <= 0 {
		pointsPerCNY = 1
	}
	multiplier := floatValue(rule["platform_multiplier"])
	if multiplier <= 0 {
		multiplier = 1
	}
	return ((outputSeconds+inputSeconds)*rate + float64(excessImages)*imagePrice) * pointsPerCNY * multiplier
}

func estimateSeedance2TokenCost(rule map[string]interface{}, params map[string]interface{}) float64 {
	resolution := strings.ToLower(strings.TrimSpace(stringValue(params["resolution"])))
	if resolution == "" {
		resolution = strings.ToLower(strings.TrimSpace(stringValue(rule["default_resolution"])))
	}
	if resolution == "" {
		resolution = "720p"
	}

	tokensPerSecond := mapFloatValue(rule["tokens_per_second"], resolution)
	if tokensPerSecond <= 0 {
		tokensPerSecond = map[string]float64{
			"480p":  10044,
			"720p":  21600,
			"1080p": 48600,
			"4k":    194400,
		}[resolution]
	}
	if tokensPerSecond <= 0 {
		tokensPerSecond = 21600
	}

	mode := strings.ToLower(strings.TrimSpace(stringValue(params["generation_mode"])))
	hasVideoInput := strings.Contains(mode, "video") || urlFieldCount(params["reference_videos"]) > 0
	if portraitID := strings.TrimSpace(stringValue(params["portrait_asset_id"])); portraitID != "" &&
		strings.EqualFold(strings.TrimSpace(stringValue(params["portrait_asset_type"])), "video") {
		hasVideoInput = true
	}

	rateKey := "without_video"
	if hasVideoInput {
		rateKey = "with_video"
	}
	ratePerMillion := nestedMapFloatValue(rule["rates_per_m_tokens"], resolution, rateKey)
	if ratePerMillion <= 0 {
		defaultRates := map[string]map[string]float64{
			"480p":  {"without_video": 46, "with_video": 28},
			"720p":  {"without_video": 46, "with_video": 28},
			"1080p": {"without_video": 51, "with_video": 31},
			"4k":    {"without_video": 26, "with_video": 16},
		}
		ratePerMillion = defaultRates[resolution][rateKey]
	}
	if ratePerMillion <= 0 {
		return floatValue(rule["fallback_cost"])
	}

	outputDuration := parseDurationSeconds(params)
	tokenUsage := outputDuration * tokensPerSecond
	if hasVideoInput {
		inputDuration := floatValue(params["reference_video_duration_seconds"])
		if inputDuration <= 0 {
			inputDuration = floatValue(rule["default_input_video_seconds"])
		}
		if inputDuration <= 0 {
			inputDuration = 4
		}
		tokenUsage = (outputDuration + inputDuration) * tokensPerSecond
		minMultiplier := floatValue(rule["video_min_token_multiplier"])
		if minMultiplier <= 0 {
			minMultiplier = 1.8
		}
		minTokens := outputDuration * tokensPerSecond * minMultiplier
		if tokenUsage < minTokens {
			tokenUsage = minTokens
		}
	}

	cost := tokenUsage / 1_000_000 * ratePerMillion
	multiplier := floatValue(rule["platform_multiplier"])
	if multiplier <= 0 {
		multiplier = 1
	}
	pointsPerCurrency := floatValue(rule["points_per_cny"])
	if pointsPerCurrency <= 0 {
		pointsPerCurrency = 1
	}
	return cost * multiplier * pointsPerCurrency
}

func mapFloatValue(raw interface{}, key string) float64 {
	m, _ := raw.(map[string]interface{})
	if m == nil {
		return 0
	}
	return floatValue(m[key])
}

func nestedMapFloatValue(raw interface{}, firstKey, secondKey string) float64 {
	m, _ := raw.(map[string]interface{})
	if m == nil {
		return 0
	}
	return mapFloatValue(m[firstKey], secondKey)
}

// perTokenPrice resolves a per-token price from price_rule, supporting both
// per-token keys (input_price) and admin-friendly per-1M keys (input_price_per_m).
func perTokenPrice(rule map[string]interface{}, key string) float64 {
	if v, ok := rule[key].(float64); ok && v > 0 {
		return v
	}
	if v, ok := rule[key+"_per_m"].(float64); ok && v > 0 {
		return v / 1_000_000
	}
	return 0
}

func (s *ModelService) ListCategories(ctx context.Context) ([]map[string]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT category FROM models WHERE is_enabled=true ORDER BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []map[string]string
	labels := map[string]string{
		"chat": "聊天", "multi_collab": "多模型协作", "image": "图片", "video": "视频", "audio": "音频",
	}
	for rows.Next() {
		var cat string
		rows.Scan(&cat)
		label := labels[cat]
		if label == "" {
			label = cat
		}
		cats = append(cats, map[string]string{"code": cat, "label": label})
	}
	if len(cats) == 0 {
		cats = []map[string]string{{"code": "chat", "label": "聊天"}, {"code": "image", "label": "图片"}}
	}
	return cats, nil
}

type AdminModelDTO struct {
	ModelDTO
	NewAPIModel       string                 `json:"new_api_model"`
	NewAPIEndpoint    string                 `json:"new_api_endpoint"`
	RequestMode       string                 `json:"request_mode"`
	NewAPIExtraParams map[string]interface{} `json:"new_api_extra_params"`
}

type APIDocDTO struct {
	ID           int64                  `json:"id"`
	ModelID      int64                  `json:"model_id"`
	ModelCode    string                 `json:"model_code"`
	ModelName    string                 `json:"model_name"`
	ModelIconURL *string                `json:"model_icon_url,omitempty"`
	ModelDesc    string                 `json:"model_description"`
	Category     string                 `json:"category"`
	RequestMode  string                 `json:"request_mode"`
	NewAPIModel  string                 `json:"new_api_model"`
	Slug         string                 `json:"slug"`
	Title        string                 `json:"title"`
	Summary      string                 `json:"summary"`
	Protocol     string                 `json:"protocol"`
	BaseURL      string                 `json:"base_url"`
	Endpoint     string                 `json:"endpoint"`
	AuthHeader   string                 `json:"auth_header"`
	SDK          string                 `json:"sdk"`
	Content      map[string]interface{} `json:"content"`
	IsPublished  bool                   `json:"is_published"`
	SortOrder    int                    `json:"sort_order"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

type APIDocInput struct {
	ModelID     int64                  `json:"model_id"`
	Slug        string                 `json:"slug"`
	Title       string                 `json:"title"`
	Summary     string                 `json:"summary"`
	Protocol    string                 `json:"protocol"`
	BaseURL     string                 `json:"base_url"`
	Endpoint    string                 `json:"endpoint"`
	AuthHeader  string                 `json:"auth_header"`
	SDK         string                 `json:"sdk"`
	Content     map[string]interface{} `json:"content"`
	IsPublished bool                   `json:"is_published"`
	SortOrder   int                    `json:"sort_order"`
}

func (s *ModelService) ListAll(ctx context.Context) ([]AdminModelDTO, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, code, display_name, new_api_model, new_api_endpoint, request_mode, category, icon_url, description, tags, runtime_rule, input_schema, default_params, new_api_extra_params, price_rule, is_enabled, sort_order
		FROM models ORDER BY sort_order ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanAdminModels(rows)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].NewAPIExtraParams = maskModelSecrets(items[i].NewAPIExtraParams)
	}
	return items, nil
}

func scanAdminModels(rows pgx.Rows) ([]AdminModelDTO, error) {
	var models []AdminModelDTO
	for rows.Next() {
		var m AdminModelDTO
		var tags, runtime, schema, defaults, extra, price []byte
		if err := rows.Scan(&m.ID, &m.Code, &m.DisplayName, &m.NewAPIModel, &m.NewAPIEndpoint, &m.RequestMode, &m.Category,
			&m.IconURL, &m.Description, &tags, &runtime, &schema, &defaults, &extra, &price, &m.IsEnabled, &m.SortOrder); err != nil {
			return nil, err
		}
		json.Unmarshal(tags, &m.Tags)
		json.Unmarshal(runtime, &m.RuntimeRule)
		json.Unmarshal(schema, &m.InputSchema)
		json.Unmarshal(defaults, &m.DefaultParams)
		json.Unmarshal(extra, &m.NewAPIExtraParams)
		json.Unmarshal(price, &m.PriceRule)
		models = append(models, m)
	}
	return models, nil
}

func (s *ModelService) ListAPIDocs(ctx context.Context, includeUnpublished bool) ([]APIDocDTO, error) {
	q := `
		SELECT d.id, d.model_id, d.slug, d.title, COALESCE(d.summary,''), d.protocol, d.base_url, d.endpoint,
		       d.auth_header, COALESCE(d.sdk,''), d.content, d.is_published, d.sort_order, d.created_at, d.updated_at,
		       m.code, m.display_name, m.category, m.request_mode, m.new_api_model, m.icon_url, COALESCE(m.description,'')
		FROM api_docs d JOIN models m ON m.id=d.model_id`
	if !includeUnpublished {
		q += ` WHERE d.is_published=true AND m.is_enabled=true`
	}
	q += ` ORDER BY d.sort_order ASC, d.id ASC`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []APIDocDTO
	for rows.Next() {
		item, err := scanAPIDoc(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *ModelService) GetAPIDoc(ctx context.Context, slug string, publicOnly bool) (*APIDocDTO, error) {
	q := `
		SELECT d.id, d.model_id, d.slug, d.title, COALESCE(d.summary,''), d.protocol, d.base_url, d.endpoint,
		       d.auth_header, COALESCE(d.sdk,''), d.content, d.is_published, d.sort_order, d.created_at, d.updated_at,
		       m.code, m.display_name, m.category, m.request_mode, m.new_api_model, m.icon_url, COALESCE(m.description,'')
		FROM api_docs d JOIN models m ON m.id=d.model_id
		WHERE d.slug=$1`
	if publicOnly {
		q += ` AND d.is_published=true AND m.is_enabled=true`
	}
	return scanAPIDocRow(s.db.QueryRow(ctx, q, slug))
}

func (s *ModelService) CreateAPIDoc(ctx context.Context, input APIDocInput) (*APIDocDTO, error) {
	normalized, err := s.normalizeAPIDocInput(ctx, input)
	if err != nil {
		return nil, err
	}
	content, _ := json.Marshal(normalized.Content)
	var id int64
	err = s.db.QueryRow(ctx, `
		INSERT INTO api_docs (model_id, slug, title, summary, protocol, base_url, endpoint, auth_header, sdk, content, is_published, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		normalized.ModelID, normalized.Slug, normalized.Title, normalized.Summary, normalized.Protocol,
		normalized.BaseURL, normalized.Endpoint, normalized.AuthHeader, normalized.SDK, content, normalized.IsPublished, normalized.SortOrder).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetAPIDocByID(ctx, id)
}

func (s *ModelService) UpdateAPIDoc(ctx context.Context, id int64, input APIDocInput) (*APIDocDTO, error) {
	normalized, err := s.normalizeAPIDocInput(ctx, input)
	if err != nil {
		return nil, err
	}
	content, _ := json.Marshal(normalized.Content)
	tag, err := s.db.Exec(ctx, `
		UPDATE api_docs SET model_id=$1, slug=$2, title=$3, summary=$4, protocol=$5, base_url=$6,
			endpoint=$7, auth_header=$8, sdk=$9, content=$10, is_published=$11, sort_order=$12, updated_at=now()
		WHERE id=$13`,
		normalized.ModelID, normalized.Slug, normalized.Title, normalized.Summary, normalized.Protocol,
		normalized.BaseURL, normalized.Endpoint, normalized.AuthHeader, normalized.SDK, content, normalized.IsPublished, normalized.SortOrder, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, errors.New("API 文档不存在")
	}
	return s.GetAPIDocByID(ctx, id)
}

func (s *ModelService) DeleteAPIDoc(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM api_docs WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("API 文档不存在")
	}
	return nil
}

func (s *ModelService) GetAPIDocByID(ctx context.Context, id int64) (*APIDocDTO, error) {
	return scanAPIDocRow(s.db.QueryRow(ctx, `
		SELECT d.id, d.model_id, d.slug, d.title, COALESCE(d.summary,''), d.protocol, d.base_url, d.endpoint,
		       d.auth_header, COALESCE(d.sdk,''), d.content, d.is_published, d.sort_order, d.created_at, d.updated_at,
		       m.code, m.display_name, m.category, m.request_mode, m.new_api_model, m.icon_url, COALESCE(m.description,'')
		FROM api_docs d JOIN models m ON m.id=d.model_id WHERE d.id=$1`, id))
}

func (s *ModelService) normalizeAPIDocInput(ctx context.Context, input APIDocInput) (*APIDocInput, error) {
	if input.ModelID <= 0 {
		return nil, errors.New("请选择已接入模型")
	}
	var code, displayName, requestMode, upstreamEndpoint string
	err := s.db.QueryRow(ctx, `SELECT code, display_name, request_mode, new_api_endpoint FROM models WHERE id=$1`, input.ModelID).
		Scan(&code, &displayName, &requestMode, &upstreamEndpoint)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("选择的模型不存在或未接入")
		}
		return nil, err
	}
	if input.Slug == "" {
		input.Slug = code
	}
	if input.Title == "" {
		input.Title = displayName
	}
	if input.Protocol == "" {
		input.Protocol = defaultAPIDocProtocol(requestMode)
	}
	if input.BaseURL == "" {
		input.BaseURL = "https://api.your-starai-domain.com"
	}
	if input.Endpoint == "" {
		input.Endpoint = defaultAPIDocEndpoint(requestMode, upstreamEndpoint)
	}
	if input.AuthHeader == "" {
		input.AuthHeader = "Authorization: Bearer <API_KEY>"
	}
	if input.Content == nil {
		input.Content = map[string]interface{}{}
	}
	return &input, nil
}

func defaultAPIDocProtocol(requestMode string) string {
	switch requestMode {
	case "images":
		return "openai-compatible-image"
	case "video":
		return "new-api-compatible-video"
	case "audio":
		return "openai-compatible-audio"
	case "custom":
		return "custom-compatible"
	default:
		return "openai-compatible"
	}
}

func defaultAPIDocEndpoint(requestMode, upstreamEndpoint string) string {
	switch requestMode {
	case "responses":
		return "/v1/responses"
	case "images":
		return "/v1/images/generations"
	case "video":
		return "/v1/video/generations"
	case "audio":
		return "/v1/audio/speech"
	case "custom":
		if upstreamEndpoint != "" {
			return upstreamEndpoint
		}
	}
	return "/v1/chat/completions"
}

func scanAPIDoc(rows pgx.Rows) (*APIDocDTO, error) {
	var item APIDocDTO
	var content []byte
	var created, updated time.Time
	if err := rows.Scan(&item.ID, &item.ModelID, &item.Slug, &item.Title, &item.Summary, &item.Protocol,
		&item.BaseURL, &item.Endpoint, &item.AuthHeader, &item.SDK, &content, &item.IsPublished, &item.SortOrder,
		&created, &updated, &item.ModelCode, &item.ModelName, &item.Category, &item.RequestMode,
		&item.NewAPIModel, &item.ModelIconURL, &item.ModelDesc); err != nil {
		return nil, err
	}
	json.Unmarshal(content, &item.Content)
	item.Content = standardAPIDocContent(&item, item.Content)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func scanAPIDocRow(row pgx.Row) (*APIDocDTO, error) {
	var item APIDocDTO
	var content []byte
	var created, updated time.Time
	if err := row.Scan(&item.ID, &item.ModelID, &item.Slug, &item.Title, &item.Summary, &item.Protocol,
		&item.BaseURL, &item.Endpoint, &item.AuthHeader, &item.SDK, &content, &item.IsPublished, &item.SortOrder,
		&created, &updated, &item.ModelCode, &item.ModelName, &item.Category, &item.RequestMode,
		&item.NewAPIModel, &item.ModelIconURL, &item.ModelDesc); err != nil {
		return nil, err
	}
	json.Unmarshal(content, &item.Content)
	item.Content = standardAPIDocContent(&item, item.Content)
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func standardAPIDocContent(doc *APIDocDTO, content map[string]interface{}) map[string]interface{} {
	if content == nil {
		content = map[string]interface{}{}
	}
	setDefault := func(key string, value interface{}) {
		if _, ok := content[key]; !ok || content[key] == nil {
			content[key] = value
		}
	}
	setDefault("features", []string{"统一 API Key", "平台模型编码", "标准 JSON 响应"})
	requestExample := defaultAPIDocRequestExample(doc)
	responseExample := defaultAPIDocResponseExample(doc)
	if doc.RequestMode == "images" || doc.RequestMode == "video" || doc.RequestMode == "audio" {
		content["request_example"] = requestExample
		content["response_example"] = responseExample
	} else {
		setDefault("request_example", requestExample)
		setDefault("response_example", responseExample)
	}
	setDefault("notes", []string{
		"Authorization 使用平台 API Key，而不是上游供应商 Key。",
		"model 字段填写平台模型编码：" + doc.ModelCode,
		"计费、限流和路由以平台后台模型配置为准。",
	})
	content["status_code"] = 200
	content["http_status"] = 200
	content["response_status"] = 200
	content["responses"] = standardAPIDocResponses(content, responseExample)
	if doc.RequestMode == "images" || doc.RequestMode == "video" || doc.RequestMode == "audio" {
		content["async"] = true
		content["polling"] = map[string]interface{}{
			"method":   "GET",
			"endpoint": "/v1/tasks/{task_no}",
			"events":   "/v1/tasks/{task_no}/events",
			"notes":    "创建任务成功后轮询任务详情；status=succeeded 时读取 output，status=failed 时读取 error_message。",
		}
		content["parameters"] = defaultAPIDocParameters(doc)
	}
	content["standard"] = map[string]interface{}{
		"method":      "POST",
		"endpoint":    doc.Endpoint,
		"model":       doc.ModelCode,
		"category":    doc.Category,
		"requestMode": doc.RequestMode,
	}
	return content
}

func standardAPIDocResponses(content map[string]interface{}, successExample map[string]interface{}) map[string]interface{} {
	responses := map[string]interface{}{}
	if raw, ok := content["responses"].(map[string]interface{}); ok {
		for k, v := range raw {
			responses[k] = v
		}
	}
	if _, ok := responses["200"]; !ok {
		responses["200"] = map[string]interface{}{
			"description": "请求成功",
			"body":        successExample,
		}
	}
	if _, ok := responses["400"]; !ok {
		responses["400"] = map[string]interface{}{
			"description": "请求参数错误，例如 model 不存在或未启用",
			"body":        map[string]interface{}{"code": 400, "message": "模型不存在或未启用，请检查 model 是否为后台模型编码或接入模型名"},
		}
	}
	if _, ok := responses["401"]; !ok {
		responses["401"] = map[string]interface{}{
			"description": "API Key 无效或已停用",
			"body":        map[string]interface{}{"code": 401, "message": "API Key 无效或已停用"},
		}
	}
	if _, ok := responses["502"]; !ok {
		responses["502"] = map[string]interface{}{
			"description": "上游模型服务异常",
			"body":        map[string]interface{}{"code": 502, "message": "模型服务异常"},
		}
	}
	return responses
}

func defaultAPIDocParameters(doc *APIDocDTO) []map[string]interface{} {
	switch doc.RequestMode {
	case "images":
		return []map[string]interface{}{
			{"name": "model", "type": "string", "required": true, "description": "平台模型编码或后台接入模型名，例如 " + doc.ModelCode},
			{"name": "prompt", "type": "string", "required": true, "description": "图片生成提示词"},
			{"name": "n", "type": "integer", "required": false, "description": "生成数量，默认 1"},
			{"name": "aspect_ratio", "type": "string", "required": false, "description": "比例，例如 1:1、16:9、9:16、4:3、3:4"},
			{"name": "image_size", "type": "string", "required": false, "description": "清晰度档位，例如 1K、2K、4K"},
			{"name": "size", "type": "string", "required": false, "description": "实际像素尺寸，例如 1024x1024、3840x2160"},
			{"name": "image", "type": "string|string[]", "required": false, "description": "参考图 URL，支持单张或数组"},
		}
	case "video":
		return []map[string]interface{}{
			{"name": "model", "type": "string", "required": true, "description": "平台模型编码或后台接入模型名，例如 " + doc.ModelCode},
			{"name": "prompt", "type": "string", "required": true, "description": "视频生成提示词"},
			{"name": "count", "type": "integer", "required": false, "description": "生成数量，默认 1"},
			{"name": "duration", "type": "string|number", "required": false, "description": "视频时长，以后台模型支持为准，例如 12s"},
			{"name": "orientation", "type": "string", "required": false, "description": "画面方向，例如 portrait / landscape"},
			{"name": "aspect_ratio", "type": "string", "required": false, "description": "画面比例，例如 9:16、16:9"},
			{"name": "image", "type": "string|string[]", "required": false, "description": "图生视频参考图 URL"},
		}
	case "audio":
		return []map[string]interface{}{
			{"name": "model", "type": "string", "required": true, "description": "平台模型编码或后台接入模型名，例如 " + doc.ModelCode},
			{"name": "input", "type": "string", "required": true, "description": "需要合成的文本"},
			{"name": "voice", "type": "string", "required": false, "description": "音色，以后台模型支持为准"},
			{"name": "format", "type": "string", "required": false, "description": "输出格式，例如 mp3 / wav"},
		}
	default:
		return nil
	}
}

func defaultAPIDocRequestExample(doc *APIDocDTO) map[string]interface{} {
	switch doc.RequestMode {
	case "images":
		return map[string]interface{}{
			"model":        doc.ModelCode,
			"prompt":       "为莫来石产品生成电商商品主图，白底，高级质感，真实摄影风格",
			"n":            1,
			"aspect_ratio": "1:1",
			"image_size":   "1K",
			"size":         "1024x1024",
			"quality":      "standard",
			"image":        []string{"https://example.com/reference.png"},
		}
	case "video":
		return map[string]interface{}{
			"model":        doc.ModelCode,
			"prompt":       "生成一段商品展示短视频，突出产品质感、卖点和镜头推进",
			"duration":     "12s",
			"orientation":  "portrait",
			"aspect_ratio": "9:16",
			"count":        1,
			"image":        []string{"https://example.com/reference.png"},
		}
	case "audio":
		return map[string]interface{}{
			"model":  doc.ModelCode,
			"input":  "欢迎使用 StarAI 开放平台。",
			"voice":  "alloy",
			"format": "mp3",
		}
	case "responses":
		return map[string]interface{}{
			"model": doc.ModelCode,
			"input": "请用三句话介绍你的能力。",
		}
	default:
		return map[string]interface{}{
			"model": doc.ModelCode,
			"messages": []map[string]string{
				{"role": "user", "content": "你好，请介绍一下你的能力。"},
			},
			"stream": false,
		}
	}
}

func defaultAPIDocResponseExample(doc *APIDocDTO) map[string]interface{} {
	switch doc.RequestMode {
	case "images":
		return map[string]interface{}{
			"code":    0,
			"message": "ok",
			"data": map[string]interface{}{
				"task_no":        "task_xxx",
				"type":           "image",
				"status":         "pending",
				"model_code":     doc.ModelCode,
				"estimated_cost": 1.0,
				"created_at":     time.Now().Format(time.RFC3339),
				"poll_url":       "/v1/tasks/task_xxx",
			},
		}
	case "video":
		return map[string]interface{}{
			"code":    0,
			"message": "ok",
			"data": map[string]interface{}{
				"task_no":        "task_xxx",
				"type":           "video",
				"status":         "pending",
				"model_code":     doc.ModelCode,
				"estimated_cost": 1.0,
				"created_at":     time.Now().Format(time.RFC3339),
				"poll_url":       "/v1/tasks/task_xxx",
			},
		}
	case "audio":
		return map[string]interface{}{
			"code":    0,
			"message": "ok",
			"data": map[string]interface{}{
				"task_no":        "task_xxx",
				"type":           "audio",
				"status":         "pending",
				"model_code":     doc.ModelCode,
				"estimated_cost": 1.0,
				"created_at":     time.Now().Format(time.RFC3339),
				"poll_url":       "/v1/tasks/task_xxx",
			},
		}
	case "responses":
		return map[string]interface{}{
			"id":     "resp_xxx",
			"object": "response",
			"output": []map[string]interface{}{
				{"type": "message", "content": []map[string]string{{"type": "output_text", "text": "这是模型响应内容。"}}},
			},
		}
	default:
		return map[string]interface{}{
			"code":    0,
			"message": "ok",
			"data": map[string]interface{}{
				"request_id":      "req_xxx",
				"conversation_id": "conv_xxx",
				"content":         "这是模型响应内容。",
				"cost":            0.01,
			},
		}
	}
}

type CreateModelInput struct {
	Code              string                 `json:"code"`
	DisplayName       string                 `json:"display_name"`
	NewAPIModel       string                 `json:"new_api_model"`
	NewAPIEndpoint    string                 `json:"new_api_endpoint"`
	RequestMode       string                 `json:"request_mode"`
	Category          string                 `json:"category"`
	IconURL           string                 `json:"icon_url"`
	Description       string                 `json:"description"`
	Tags              []string               `json:"tags"`
	InputSchema       map[string]interface{} `json:"input_schema"`
	DefaultParams     map[string]interface{} `json:"default_params"`
	NewAPIExtraParams map[string]interface{} `json:"new_api_extra_params"`
	PriceRule         map[string]interface{} `json:"price_rule"`
	RuntimeRule       map[string]interface{} `json:"runtime_rule"`
	IsEnabled         bool                   `json:"is_enabled"`
	SortOrder         int                    `json:"sort_order"`
}

func (s *ModelService) Create(ctx context.Context, input CreateModelInput) (*ModelDTO, error) {
	if err := validateModelConnection(input); err != nil {
		return nil, err
	}
	if err := validateModelPriceRule(input.PriceRule); err != nil {
		return nil, err
	}
	input.NewAPIEndpoint = normalizeModelEndpoint(input.NewAPIEndpoint)
	tags, _ := json.Marshal(input.Tags)
	runtime, _ := json.Marshal(input.RuntimeRule)
	schema, _ := json.Marshal(input.InputSchema)
	defaults, _ := json.Marshal(input.DefaultParams)
	extra, _ := json.Marshal(input.NewAPIExtraParams)
	price, _ := json.Marshal(input.PriceRule)
	if input.NewAPIEndpoint == "" {
		input.NewAPIEndpoint = "/v1/chat/completions"
	}
	var id int64
	err := s.db.QueryRow(ctx, `
		INSERT INTO models (code, display_name, new_api_model, new_api_endpoint, request_mode, category, icon_url, description, tags, runtime_rule, input_schema, default_params, new_api_extra_params, price_rule, is_enabled, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
		input.Code, input.DisplayName, input.NewAPIModel, input.NewAPIEndpoint, input.RequestMode, input.Category,
		input.IconURL, input.Description, tags, runtime, schema, defaults, extra, price, input.IsEnabled, input.SortOrder,
	).Scan(&id)
	if err != nil {
		if friendly := modelCreateError(input.Code, err); friendly != nil {
			return nil, friendly
		}
		return nil, err
	}
	return s.GetByID(ctx, id)
}

func modelCreateError(code string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "models_code_key":
			return fmt.Errorf("模型编码 %q 已存在，请换一个编码或编辑已有模型", code)
		default:
			return errors.New("模型唯一字段已存在，请检查编码或名称")
		}
	}
	return nil
}

func (s *ModelService) Update(ctx context.Context, id int64, input CreateModelInput) (*ModelDTO, error) {
	if err := s.preserveExistingModelSecrets(ctx, id, &input); err != nil {
		return nil, err
	}
	if err := validateModelConnection(input); err != nil {
		return nil, err
	}
	if err := validateModelPriceRule(input.PriceRule); err != nil {
		return nil, err
	}
	input.NewAPIEndpoint = normalizeModelEndpoint(input.NewAPIEndpoint)
	tags, _ := json.Marshal(input.Tags)
	runtime, _ := json.Marshal(input.RuntimeRule)
	schema, _ := json.Marshal(input.InputSchema)
	defaults, _ := json.Marshal(input.DefaultParams)
	extra, _ := json.Marshal(input.NewAPIExtraParams)
	price, _ := json.Marshal(input.PriceRule)
	_, err := s.db.Exec(ctx, `
		UPDATE models SET display_name=$1, new_api_model=$2, new_api_endpoint=$3, request_mode=$4, category=$5,
			icon_url=$6, description=$7, tags=$8, runtime_rule=$9, input_schema=$10, default_params=$11, new_api_extra_params=$12, price_rule=$13, is_enabled=$14, sort_order=$15, updated_at=now()
		WHERE id=$16`,
		input.DisplayName, input.NewAPIModel, input.NewAPIEndpoint, input.RequestMode, input.Category,
		input.IconURL, input.Description, tags, runtime, schema, defaults, extra, price, input.IsEnabled, input.SortOrder, id)
	if err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

func validateModelPriceRule(rule map[string]interface{}) error {
	billingType := strings.ToLower(strings.TrimSpace(stringValue(rule["billing_type"])))
	allowed := map[string]bool{"per_token": true, "per_image": true, "per_request": true, "per_second": true, "dynamic": true}
	if !allowed[billingType] {
		return fmt.Errorf("不支持的计费类型：%s", billingType)
	}
	for _, key := range []string{"unit_price", "input_price", "output_price", "cache_read_price", "cache_write_price", "input_price_per_m", "output_price_per_m", "cache_read_price_per_m", "cache_write_price_per_m", "surcharge_per_m", "fallback_cost"} {
		if floatValue(rule[key]) < 0 {
			return fmt.Errorf("计费字段 %s 不能为负数", key)
		}
	}
	switch billingType {
	case "per_token":
		if perTokenPrice(rule, "input_price") <= 0 && perTokenPrice(rule, "output_price") <= 0 {
			return errors.New("Token 计费至少需要配置输入或输出单价")
		}
	case "per_image", "per_request", "per_second":
		if _, exists := rule["unit_price"]; !exists {
			return errors.New("按次、按图或按秒计费必须配置 unit_price")
		}
	case "dynamic":
		strategy := strings.ToLower(strings.TrimSpace(stringValue(rule["strategy"])))
		if strategy != "seedance_2_tokens" && strategy != "minimax_h3_seconds" && floatValue(rule["fallback_cost"]) <= 0 {
			return errors.New("动态计费策略无效，且未配置 fallback_cost")
		}
	}
	return nil
}

func (s *ModelService) preserveExistingModelSecrets(ctx context.Context, id int64, input *CreateModelInput) error {
	if input == nil || input.Category == "multi_collab" {
		return nil
	}
	nextConn, _ := input.NewAPIExtraParams["connection"].(map[string]interface{})
	if nextConn == nil {
		return nil
	}
	nextKey, _ := nextConn["api_key"].(string)
	if strings.TrimSpace(nextKey) != "" && !isMaskedSecret(nextKey) {
		return nil
	}
	var raw []byte
	if err := s.db.QueryRow(ctx, `SELECT new_api_extra_params FROM models WHERE id=$1`, id).Scan(&raw); err != nil {
		return err
	}
	var existing map[string]interface{}
	_ = json.Unmarshal(raw, &existing)
	oldConn, _ := existing["connection"].(map[string]interface{})
	oldKey, _ := oldConn["api_key"].(string)
	if strings.TrimSpace(oldKey) != "" {
		nextConn["api_key"] = oldKey
		input.NewAPIExtraParams["connection"] = nextConn
	}
	return nil
}

func maskModelSecrets(extra map[string]interface{}) map[string]interface{} {
	if extra == nil {
		return nil
	}
	out := copyMap(extra)
	if conn, ok := out["connection"].(map[string]interface{}); ok {
		if key, ok := conn["api_key"].(string); ok && strings.TrimSpace(key) != "" {
			conn["api_key"] = maskSecret(key)
		}
		out["connection"] = conn
	}
	return out
}

func copyMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]interface{}); ok {
			out[k] = copyMap(m)
		} else {
			out[k] = v
		}
	}
	return out
}

func isMaskedSecret(v string) bool {
	return strings.Contains(v, "***") || strings.Contains(v, "****")
}

func maskSecret(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	r := []rune(v)
	if len(r) <= 8 {
		return "****"
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}

func validateModelConnection(input CreateModelInput) error {
	if input.Category == "multi_collab" {
		return nil
	}
	conn, _ := input.NewAPIExtraParams["connection"].(map[string]interface{})
	if conn == nil {
		return errors.New("模型接入配置缺少 connection")
	}
	baseURL, _ := conn["base_url"].(string)
	if baseURL == "" {
		return errors.New("模型接入配置的 Base URL 为必填")
	}
	authType, _ := conn["auth_type"].(string)
	if authType == "" {
		authType = "bearer"
	}
	if authType != "none" {
		apiKey, _ := conn["api_key"].(string)
		if apiKey == "" {
			return errors.New("模型接入配置的 API Key 为必填")
		}
	}
	return nil
}

func normalizeModelEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	return "/" + endpoint
}

func (s *ModelService) Delete(ctx context.Context, id int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM models WHERE id=$1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("模型不存在")
	}

	// Detach historical records so the model row can be removed without losing user data.
	detachQueries := []string{
		`UPDATE conversations SET model_id=NULL WHERE model_id=$1`,
		`UPDATE ai_call_logs SET model_id=NULL WHERE model_id=$1`,
		`UPDATE tasks SET model_id=NULL WHERE model_id=$1`,
		`UPDATE works SET model_id=NULL WHERE model_id=$1`,
	}
	for _, q := range detachQueries {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM models WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanModels(rows pgx.Rows) ([]ModelDTO, error) {
	var models []ModelDTO
	for rows.Next() {
		var m ModelDTO
		var tags, runtime, schema, defaults, price []byte
		if err := rows.Scan(&m.ID, &m.Code, &m.DisplayName, &m.Category, &m.IconURL, &m.Description,
			&tags, &runtime, &schema, &defaults, &price, &m.IsEnabled, &m.SortOrder); err != nil {
			return nil, err
		}
		json.Unmarshal(tags, &m.Tags)
		json.Unmarshal(runtime, &m.RuntimeRule)
		json.Unmarshal(schema, &m.InputSchema)
		json.Unmarshal(defaults, &m.DefaultParams)
		json.Unmarshal(price, &m.PriceRule)
		models = append(models, m)
	}
	return models, nil
}

type CategoryCount struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

func (s *ModelService) CountEnabled(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM models WHERE is_enabled=true`).Scan(&n)
	return n, err
}

func parseTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
