package canceledreadcontext_test

import (
	"context"
	"errors"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestCanceledReadContextStopsRepositoryWork(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("canceled-context-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	created, err := service.CreateCase(context.Background(), application.CreateCaseCommand{
		CommandMeta:   application.CommandMeta{ExpectedVersion: 0, IdempotencyKey: "create-context-123", Actor: "测试修复师", Role: domain.RoleConservator},
		AccessionCode: "CTX-001", ShelfLocation: "库房一架", Title: "取消读取测试", ResponsibleConservator: "测试修复师",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.GetCase(ctx, created.Case.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetCase 未传播取消信号: %v", err)
	}
	if _, err := service.ListCases(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListCases 未传播取消信号: %v", err)
	}
	if _, err := service.QueryCases(ctx, application.CaseQuery{Page: 1, PageSize: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("QueryCases 未传播取消信号: %v", err)
	}
	if err := service.VerifyStore(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("VerifyStore 未传播取消信号: %v", err)
	}
}
