package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starai/api/internal/util"
)

const maxCanvasDocumentBytes = 2 * 1024 * 1024

type CanvasService struct {
	db *pgxpool.Pool
}

func NewCanvasService(db *pgxpool.Pool) *CanvasService {
	return &CanvasService{db: db}
}

type CanvasDTO struct {
	PublicID  string          `json:"public_id"`
	Title     string          `json:"title"`
	Document  json.RawMessage `json:"document,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

type SaveCanvasInput struct {
	Title    string          `json:"title"`
	Document json.RawMessage `json:"document"`
}

func validateCanvasInput(input *SaveCanvasInput) error {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "未命名画布"
	}
	if len([]rune(input.Title)) > 120 {
		return errors.New("画布名称不能超过 120 个字符")
	}
	if len(input.Document) == 0 || !json.Valid(input.Document) {
		return errors.New("画布数据格式无效")
	}
	if len(input.Document) > maxCanvasDocumentBytes {
		return errors.New("画布数据不能超过 2MB")
	}
	var shape struct {
		Version  int               `json:"version"`
		Nodes    []json.RawMessage `json:"nodes"`
		Edges    []json.RawMessage `json:"edges"`
		Viewport json.RawMessage   `json:"viewport"`
	}
	if err := json.Unmarshal(input.Document, &shape); err != nil {
		return errors.New("画布数据格式无效")
	}
	if len(shape.Nodes) > 500 || len(shape.Edges) > 1000 {
		return errors.New("单个画布最多支持 500 个节点和 1000 条连线")
	}
	return nil
}

func (s *CanvasService) Create(ctx context.Context, userID int64, input SaveCanvasInput) (*CanvasDTO, error) {
	if err := validateCanvasInput(&input); err != nil {
		return nil, err
	}
	item := &CanvasDTO{PublicID: util.NewPublicID("canvas")}
	var created, updated time.Time
	err := s.db.QueryRow(ctx, `
		INSERT INTO infinite_canvases (public_id, user_id, title, document)
		VALUES ($1,$2,$3,$4)
		RETURNING title, document, created_at, updated_at`,
		item.PublicID, userID, input.Title, input.Document).
		Scan(&item.Title, &item.Document, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = created.Format(time.RFC3339)
	item.UpdatedAt = updated.Format(time.RFC3339)
	return item, nil
}

func (s *CanvasService) List(ctx context.Context, userID int64, page, pageSize int) ([]CanvasDTO, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM infinite_canvases WHERE user_id=$1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT public_id, title, created_at, updated_at
		FROM infinite_canvases
		WHERE user_id=$1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3`, userID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]CanvasDTO, 0)
	for rows.Next() {
		var item CanvasDTO
		var created, updated time.Time
		if err := rows.Scan(&item.PublicID, &item.Title, &created, &updated); err != nil {
			return nil, 0, err
		}
		item.CreatedAt = created.Format(time.RFC3339)
		item.UpdatedAt = updated.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *CanvasService) Get(ctx context.Context, userID int64, publicID string) (*CanvasDTO, error) {
	var item CanvasDTO
	var created, updated time.Time
	err := s.db.QueryRow(ctx, `
		SELECT public_id, title, document, created_at, updated_at
		FROM infinite_canvases
		WHERE user_id=$1 AND public_id=$2`, userID, publicID).
		Scan(&item.PublicID, &item.Title, &item.Document, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = created.Format(time.RFC3339)
	item.UpdatedAt = updated.Format(time.RFC3339)
	return &item, nil
}

func (s *CanvasService) Update(ctx context.Context, userID int64, publicID string, input SaveCanvasInput) (*CanvasDTO, error) {
	if err := validateCanvasInput(&input); err != nil {
		return nil, err
	}
	var item CanvasDTO
	var created, updated time.Time
	err := s.db.QueryRow(ctx, `
		UPDATE infinite_canvases
		SET title=$3, document=$4, updated_at=now()
		WHERE user_id=$1 AND public_id=$2
		RETURNING public_id, title, document, created_at, updated_at`,
		userID, publicID, input.Title, input.Document).
		Scan(&item.PublicID, &item.Title, &item.Document, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt = created.Format(time.RFC3339)
	item.UpdatedAt = updated.Format(time.RFC3339)
	return &item, nil
}

func (s *CanvasService) Delete(ctx context.Context, userID int64, publicID string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM infinite_canvases WHERE user_id=$1 AND public_id=$2`, userID, publicID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
