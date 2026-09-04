package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestParseWorkRetentionDays(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		fallback int
		want     int
	}{
		{name: "json number", value: float64(7), fallback: 30, want: 7},
		{name: "numeric string", value: "14", fallback: 30, want: 14},
		{name: "permanent", value: float64(0), fallback: 30, want: 0},
		{name: "invalid falls back", value: "invalid", fallback: 7, want: 7},
		{name: "negative is permanent", value: float64(-1), fallback: 7, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ParseWorkRetentionDays(test.value, test.fallback); got != test.want {
				t.Fatalf("ParseWorkRetentionDays(%v, %d) = %d, want %d", test.value, test.fallback, got, test.want)
			}
		})
	}
}

func TestGalleryWorksArePermanentAndSkippedByCleanup(t *testing.T) {
	dsn := os.Getenv("WORKS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("AGENT_DRAFT_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set WORKS_TEST_DATABASE_URL for isolated database regression")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("works_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `
		CREATE TABLE models (id BIGINT PRIMARY KEY, code TEXT);
		CREATE TABLE tasks (id BIGINT PRIMARY KEY, output JSONB NOT NULL DEFAULT '{}');
		CREATE TABLE assets (id BIGINT PRIMARY KEY, user_id BIGINT NOT NULL);
		CREATE TABLE works (
			id BIGSERIAL PRIMARY KEY, public_id TEXT UNIQUE NOT NULL, user_id BIGINT NOT NULL,
			task_id BIGINT, model_id BIGINT, asset_id BIGINT, type TEXT NOT NULL,
			title TEXT, prompt TEXT, thumbnail_url TEXT, metadata JSONB NOT NULL DEFAULT '{}',
			expires_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE gallery_items (
			id BIGSERIAL PRIMARY KEY, public_id TEXT UNIQUE NOT NULL, work_id BIGINT REFERENCES works(id) ON DELETE SET NULL,
			user_id BIGINT, model_code TEXT, title TEXT, prompt TEXT, cover_url TEXT, type TEXT NOT NULL,
			tags JSONB NOT NULL DEFAULT '[]', status TEXT NOT NULL DEFAULT 'pending', is_featured BOOLEAN NOT NULL DEFAULT false,
			is_paid BOOLEAN NOT NULL DEFAULT false, price NUMERIC NOT NULL DEFAULT 0, like_count INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		INSERT INTO models(id,code) VALUES (1,'image_test');
		INSERT INTO works(public_id,user_id,model_id,type,prompt,thumbnail_url,metadata,expires_at) VALUES
			('gallery_work',1,1,'image','gallery','https://example.com/gallery.png','{"image_url":"https://example.com/gallery.png"}',now()-interval '1 day'),
			('ordinary_work',1,1,'image','ordinary','https://example.com/ordinary.png','{"image_url":"https://example.com/ordinary.png"}',now()-interval '1 day');`); err != nil {
		t.Fatal(err)
	}
	gallery := NewGalleryService(pool)
	if _, err = gallery.PublishWork(ctx, 1, "gallery_work", "gallery", nil, true, false, 0); err != nil {
		t.Fatal(err)
	}
	var expiresAt *time.Time
	if err = pool.QueryRow(ctx, `SELECT expires_at FROM works WHERE public_id='gallery_work'`).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if expiresAt == nil {
		t.Fatal("pending gallery work lost its expiration before approval")
	}
	works := NewWorksService(pool)
	deleted, err := works.deleteExpired(ctx, 1, "gallery_work")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("final cleanup guard deleted a gallery work")
	}
	removed, err := works.CleanupExpired(ctx, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed %d works, want only the ordinary expired work", removed)
	}
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM works WHERE public_id='gallery_work'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatal("gallery work was removed by expiration cleanup")
	}
	var galleryID int64
	if err = pool.QueryRow(ctx, `SELECT id FROM gallery_items WHERE work_id=(SELECT id FROM works WHERE public_id='gallery_work')`).Scan(&galleryID); err != nil {
		t.Fatal(err)
	}
	if err = gallery.Audit(ctx, galleryID, "approved", nil); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT expires_at FROM works WHERE public_id='gallery_work'`).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if expiresAt != nil {
		t.Fatal("approved gallery work still has an expiration")
	}
}
