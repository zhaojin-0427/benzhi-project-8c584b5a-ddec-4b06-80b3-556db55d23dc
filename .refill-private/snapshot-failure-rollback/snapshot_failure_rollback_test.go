package snapshot_failure_rollback_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestSnapshotFailureRetryPreservesDurableSequence(t *testing.T) {
	dataDir := t.TempDir()
	repository, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("snapshot-retry-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: 0,
			IdempotencyKey:  "create-snapshot-case",
			Actor:           "测试修复师",
			Role:            domain.RoleConservator,
			Reason:          "建立投影失败复现档案",
		},
		AccessionCode:          "SNAPSHOT-001",
		ShelfLocation:          "善本库一架",
		Title:                  "投影失败复现本",
		ResponsibleConservator: "测试修复师",
	})
	if err != nil {
		t.Fatal(err)
	}

	projectionPath := filepath.Join(dataDir, "projection.json")
	if err := os.Remove(projectionPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(projectionPath, 0o700); err != nil {
		t.Fatal(err)
	}
	damageCommand := application.AddDamageCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: created.Case.Version,
			IdempotencyKey:  "damage-after-snapshot-failure",
			Actor:           "测试修复师",
			Role:            domain.RoleConservator,
			Reason:          "登记稳定复现的损伤",
		},
		FolioRef:     "第一叶",
		DamageType:   "撕裂",
		Extent:       "书口两厘米",
		Severity:     domain.SeverityModerate,
		EvidenceNote: "图像显示纤维断裂",
	}
	if _, err := service.AddDamage(ctx, created.Case.ID, damageCommand); err == nil {
		t.Fatal("投影路径失效时提交应报告写入失败")
	}
	if err := os.Remove(projectionPath); err != nil {
		t.Fatal(err)
	}

	retried, err := service.AddDamage(ctx, created.Case.ID, damageCommand)
	if err != nil {
		t.Fatalf("重试同一幂等命令失败: %v", err)
	}
	if err := service.VerifyStore(ctx); err != nil {
		t.Fatalf("重试不得重复追加已经持久化的事件事务: %v", err)
	}
	if !retried.Replayed {
		t.Fatal("重试应复用首次已持久化事务的幂等结果")
	}
}
