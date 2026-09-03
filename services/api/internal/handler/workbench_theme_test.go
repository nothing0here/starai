package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/starai/api/internal/service"
)

func TestWorkbenchThemeValidation(t *testing.T) {
	for _, value := range []interface{}{nil, true, 1, "", "system", "Dark", []string{"dark"}, map[string]string{"theme": "dark"}} {
		if service.ValidateWorkbenchTheme(value) == nil {
			t.Fatalf("invalid theme accepted: %#v", value)
		}
		if service.WorkbenchDefaultTheme(value) != "dark" {
			t.Fatal("unsafe fallback")
		}
		// Validation happens before any DB writes, including mixed-key patches.
		raw, _ := json.Marshal(map[string]interface{}{"workbench_default_theme": value, "site_name": "must not change"})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PATCH", "/system-configs", strings.NewReader(string(raw)))
		(&Handler{}).AdminUpdateConfig(c)
		if w.Code != 400 {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	}
}

func TestWorkbenchThemeDatabaseRoundTrip(t *testing.T) {
	dsn := os.Getenv("AGENT_DRAFT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set AGENT_DRAFT_TEST_DATABASE_URL for isolated SQL integration")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("theme_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
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
	if _, err := pool.Exec(ctx, `CREATE TABLE system_configs(key text PRIMARY KEY,value jsonb,updated_at timestamptz);
CREATE TABLE admin_operation_logs(admin_id bigint,action text,target_type text,target_id text,detail jsonb);`); err != nil {
		t.Fatal(err)
	}
	h := &Handler{admin: service.NewAdminService(pool, nil, "")}
	readTheme := func() string {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/system-configs/public", nil)
		h.GetPublicSystemConfigs(c)
		if w.Code != 200 {
			t.Fatalf("public config: %s", w.Body.String())
		}
		var response struct {
			Data struct {
				Theme string `json:"workbench_default_theme"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		return response.Data.Theme
	}
	if readTheme() != "dark" {
		t.Fatal("unconfigured default should be dark")
	}
	for _, theme := range []string{"light", "dark"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PATCH", "/system-configs", strings.NewReader(`{"workbench_default_theme":"`+theme+`"}`))
		h.AdminUpdateConfig(c)
		if w.Code != 200 || readTheme() != theme {
			t.Fatalf("theme round-trip failed: %s", w.Body.String())
		}
	}
}
