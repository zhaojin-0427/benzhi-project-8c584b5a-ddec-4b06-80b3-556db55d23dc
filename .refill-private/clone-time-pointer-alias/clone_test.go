package clonetimepointeralias_test

import (
	"context"
	"testing"
	"time"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestReturnedCaseCannotMutateStoredLifecycleTimestamps(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("clone-alias-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	ctx := context.Background()
	meta := func(version int64, key string, role domain.Role) application.CommandMeta {
		return application.CommandMeta{ExpectedVersion: version, IdempotencyKey: key + "-12345678", Actor: "测试操作人", Role: role}
	}
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{CommandMeta: meta(0, "create", domain.RoleConservator), AccessionCode: "CLONE-001", ShelfLocation: "库房一架", Title: "深拷贝测试", ResponsibleConservator: "测试修复师"})
	if err != nil {
		t.Fatal(err)
	}
	damaged, err := service.AddDamage(ctx, created.Case.ID, application.AddDamageCommand{CommandMeta: meta(created.Case.Version, "damage", domain.RoleConservator), FolioRef: "第 1 叶", DamageType: "撕裂", Extent: "书口 2 cm", Severity: domain.SeverityHigh, EvidenceNote: "照片显示纤维断裂"})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := service.CreatePlan(ctx, created.Case.ID, application.CreatePlanCommand{CommandMeta: meta(damaged.Case.Version, "plan", domain.RoleConservator), Steps: []domain.TreatmentStep{{Description: "原位托补", Technique: "可逆托补", DamageIDs: []string{damaged.Case.Damages[0].ID}}}, Materials: []domain.Material{{Name: "楮皮纸", Category: "补纸", PH: 7, Reversible: true}}})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.SubmitPlan(ctx, created.Case.ID, application.SubmitPlanCommand{CommandMeta: meta(planned.Case.Version, "submit", domain.RoleConservator)})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.AssessCompatibility(ctx, created.Case.ID, application.AssessCommand{CommandMeta: meta(submitted.Case.Version, "assess", domain.RoleVerifier)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RecordSample(ctx, created.Case.ID, application.RecordSampleCommand{CommandMeta: meta(assessed.Case.Version, "sample", domain.RoleVerifier), Round: 1, MaterialBatch: "BATCH-A", TemperatureC: 22, HumidityPercent: 50, DurationMinutes: 60, ColorDifference: "无明显色差", Deformation: "无明显形变", Observations: "干燥后无色差且粘结稳定", SampleOutcome: domain.SamplePass})
	if err != nil {
		t.Fatal(err)
	}
	returned, err := service.GetCase(ctx, created.Case.ID)
	if err != nil {
		t.Fatal(err)
	}
	if returned.PlanRevisions[0].SubmittedAt == nil {
		t.Fatal("提交时间缺失")
	}
	if returned.Assessments[0].SampledAt == nil {
		t.Fatal("试样时间缺失")
	}
	originalSubmitted := *returned.PlanRevisions[0].SubmittedAt
	originalSampled := *returned.Assessments[0].SampledAt
	*returned.PlanRevisions[0].SubmittedAt = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	*returned.Assessments[0].SampledAt = time.Date(1998, 1, 1, 0, 0, 0, 0, time.UTC)
	loaded, err := service.GetCase(ctx, created.Case.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PlanRevisions[0].SubmittedAt == nil || !loaded.PlanRevisions[0].SubmittedAt.Equal(originalSubmitted) {
		t.Fatalf("调用方修改返回对象后污染了仓储中的 SubmittedAt: got=%v want=%v", loaded.PlanRevisions[0].SubmittedAt, originalSubmitted)
	}
	if loaded.Assessments[0].SampledAt == nil || !loaded.Assessments[0].SampledAt.Equal(originalSampled) {
		t.Fatalf("调用方修改返回对象后污染了仓储中的 SampledAt: got=%v want=%v", loaded.Assessments[0].SampledAt, originalSampled)
	}
}
