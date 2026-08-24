package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

func TestCommitRecoverAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c, err := domain.NewCase("case-1", "ACC-1", "库一架二", "测试古籍", "修复师甲", now)
	if err != nil {
		t.Fatal(err)
	}
	event := domain.NewEvent("event-1", c.ID, "CASE_CREATED", "修复师甲", domain.RoleConservator, "建立档案", c.Version, now, c)
	audit.Append(c, event)
	response, _ := json.Marshal(c)
	req := domain.CommitRequest{Scope: "create", IdempotencyKey: "key-12345678", PayloadHash: "payload", ExpectedVersion: 0, Case: c, Event: event, Response: response}
	if _, err := s.Commit(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	result, err := s.Commit(context.Background(), req)
	if err != nil || !result.Duplicate {
		t.Fatalf("expected duplicate result: %v", err)
	}
	req.PayloadHash = "changed"
	if _, err := s.Commit(context.Background(), req); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(context.Background(), c.ID)
	if err != nil || loaded.AccessionCode != "ACC-1" {
		t.Fatalf("recovery failed: %v", err)
	}
}
