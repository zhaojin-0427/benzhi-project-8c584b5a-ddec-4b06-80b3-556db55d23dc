package rotatedlogstalehandle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestCommitReopensAtomicallyReplacedEventLog(t *testing.T) {
	dir := t.TempDir()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	issuer, err := audit.NewIssuer("private-test-secret-for-log-rotation")
	if err != nil {
		t.Fatalf("new issuer: %v", err)
	}
	service := application.NewService(repository, issuer)
	first := createCase(t, service, "ROTATE-001", "rotate-key-first")

	logPath := filepath.Join(dir, "events.jsonl")
	oldLogPath := filepath.Join(dir, "events.before-rotation.jsonl")
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read original log: %v", err)
	}
	if err := os.Rename(logPath, oldLogPath); err != nil {
		t.Fatalf("rotate original log: %v", err)
	}
	if err := os.WriteFile(logPath, logBytes, 0o640); err != nil {
		t.Fatalf("install replacement log: %v", err)
	}

	second := createCase(t, service, "ROTATE-002", "rotate-key-second")
	if _, err := repository.Get(context.Background(), first.Case.ID); err != nil {
		t.Fatalf("first commit disappeared before restart: %v", err)
	}
	if _, err := repository.Get(context.Background(), second.Case.ID); err != nil {
		t.Fatalf("second commit not visible before restart: %v", err)
	}

	restarted, err := store.Open(dir)
	if err != nil {
		t.Fatalf("restart store: %v", err)
	}
	if _, err := restarted.Get(context.Background(), second.Case.ID); errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second commit disappeared after log replacement and restart")
	} else if err != nil {
		t.Fatalf("read second commit after restart: %v", err)
	}
}

func createCase(t *testing.T, service *application.Service, accession, key string) application.OperationResult {
	t.Helper()
	result, err := service.CreateCase(context.Background(), application.CreateCaseCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: 0,
			IdempotencyKey:  key,
			Actor:           "私有复现修复师",
			Role:            domain.RoleConservator,
			Reason:          "验证事件日志轮换后的持久性",
		},
		AccessionCode:          accession,
		ShelfLocation:          "善本库-轮换测试架",
		Title:                  "事件日志轮换复现档案",
		ResponsibleConservator: "私有复现修复师",
	})
	if err != nil {
		t.Fatalf("create %s: %v", accession, err)
	}
	return result
}
