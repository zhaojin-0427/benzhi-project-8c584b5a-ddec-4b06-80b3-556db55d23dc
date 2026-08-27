package stale_runtime_integrity_cache_test

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

func TestRuntimeVerifyRechecksReplacedEventLog(t *testing.T) {
	dir := t.TempDir()
	repository, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	issuer, err := audit.NewIssuer("private-test-signing-secret")
	if err != nil {
		t.Fatalf("create issuer: %v", err)
	}
	service := application.NewService(repository, issuer)
	if err := service.VerifyStore(context.Background()); err != nil {
		t.Fatalf("initial verification: %v", err)
	}

	logPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(logPath, []byte("not-json\n"), 0o640); err != nil {
		t.Fatalf("replace event log: %v", err)
	}

	err = service.VerifyStore(context.Background())
	if !errors.Is(err, domain.ErrIntegrity) {
		t.Fatalf("runtime verification accepted replaced event log: %v", err)
	}
}
