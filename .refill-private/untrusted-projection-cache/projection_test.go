package untrustedprojectioncache_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestRecoveryRejectsTamperedProjectionContent(t *testing.T) {
	dir := t.TempDir()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("projection-cache-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	created, err := service.CreateCase(context.Background(), application.CreateCaseCommand{
		CommandMeta:   application.CommandMeta{ExpectedVersion: 0, IdempotencyKey: "projection-create-123", Actor: "测试修复师", Role: domain.RoleConservator},
		AccessionCode: "PROJ-001", ShelfLocation: "库房一架", Title: "事件日志中的原题名", ResponsibleConservator: "测试修复师",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "projection.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var projection map[string]any
	if err := json.Unmarshal(data, &projection); err != nil {
		t.Fatal(err)
	}
	cases := projection["cases"].(map[string]any)
	item := cases[created.Case.ID].(map[string]any)
	item["title"] = "被篡改但版本和审计头未变的题名"
	data, err = json.MarshalIndent(projection, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(context.Background(), created.Case.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "事件日志中的原题名" {
		t.Fatalf("恢复过程信任了被篡改的 projection 内容: got=%q", loaded.Title)
	}
}
