package cachedqueryresultalias_test

import (
	"context"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestCachedQueryResultIsolatedFromCallerMutation(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("private-test-credential-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: 0,
			IdempotencyKey:  "private-create-case",
			Actor:           "私有测试修复师",
			Role:            domain.RoleConservator,
			Reason:          "建立缓存隔离测试档案",
		},
		AccessionCode:          "PRIVATE-CACHE-001",
		ShelfLocation:          "私有测试库位",
		Title:                  "未污染题名",
		ResponsibleConservator: "私有测试修复师",
	})
	if err != nil {
		t.Fatal(err)
	}

	query := application.CaseQuery{Page: 1, PageSize: 25}
	first, err := service.QueryCases(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Cases) != 1 || first.Cases[0].ID != created.Case.ID {
		t.Fatalf("unexpected first query: %+v", first)
	}
	first.Cases[0].Title = "调用方污染题名"
	first.Counts[domain.StatusDraft] = 99

	second, err := service.QueryCases(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Cases[0].Title; got != "未污染题名" {
		t.Fatalf("cached query leaked case mutation: got title %q", got)
	}
	if got := second.Counts[domain.StatusDraft]; got != 1 {
		t.Fatalf("cached query leaked counts mutation: got draft count %d", got)
	}
}
