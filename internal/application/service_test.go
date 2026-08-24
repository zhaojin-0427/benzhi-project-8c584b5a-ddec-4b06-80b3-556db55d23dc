package application_test

import (
	"context"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func TestHappyPathAndIdempotentReplay(t *testing.T) {
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("unit-test-credential-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{CommandMeta: meta(0, "create-key", domain.RoleConservator), AccessionCode: "ACC-100", ShelfLocation: "库房一架", Title: "测试经卷", ResponsibleConservator: "修复师甲"})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.CreateCase(ctx, application.CreateCaseCommand{CommandMeta: meta(0, "create-key", domain.RoleConservator), AccessionCode: "ACC-100", ShelfLocation: "库房一架", Title: "测试经卷", ResponsibleConservator: "修复师甲"})
	if err != nil || !replayed.Replayed || replayed.Case.ID != created.Case.ID {
		t.Fatalf("idempotent replay failed: %+v %v", replayed, err)
	}
	c := created.Case
	damaged, err := service.AddDamage(ctx, c.ID, application.AddDamageCommand{CommandMeta: meta(c.Version, "damage-key", domain.RoleConservator), FolioRef: "第 1 叶", DamageType: "撕裂", Extent: "书口 2 cm", Severity: domain.SeverityModerate, EvidenceNote: "照片显示纤维断裂"})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := service.CreatePlan(ctx, c.ID, application.CreatePlanCommand{CommandMeta: meta(damaged.Case.Version, "plan-key-01", domain.RoleConservator), Steps: []domain.TreatmentStep{{Description: "清洁并托补", Technique: "原位托补"}}, Materials: []domain.Material{{Name: "楮皮纸", Category: "补纸", PH: 7, Reversible: true}}, PaperConstraint: "轻度酸化", PigmentConstraint: "稳定", BindingConstraint: "保持原装帧"})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.SubmitPlan(ctx, c.ID, application.SubmitPlanCommand{CommandMeta: meta(planned.Case.Version, "submit-key", domain.RoleConservator)})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.AssessCompatibility(ctx, c.ID, application.AssessCommand{CommandMeta: meta(submitted.Case.Version, "assess-key", domain.RoleVerifier)})
	if err != nil || assessed.Case.Status != domain.StatusPendingSample {
		t.Fatalf("assessment failed: %v status=%s", err, assessed.Case.Status)
	}
	sampled, err := service.RecordSample(ctx, c.ID, application.RecordSampleCommand{CommandMeta: meta(assessed.Case.Version, "sample-key", domain.RoleVerifier), SampleConditions: "温度 22℃，湿度 50%", SampleObservations: "干燥后无色差", SampleOutcome: domain.SamplePass})
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.Review(ctx, c.ID, application.ReviewCommand{CommandMeta: meta(sampled.Case.Version, "review-key", domain.RoleExpert), Decision: domain.ReviewApproved, Comments: []domain.ReviewComment{}})
	if err != nil {
		t.Fatal(err)
	}
	released, err := service.Release(ctx, c.ID, application.ReleaseCommand{CommandMeta: meta(reviewed.Case.Version, "release-key", domain.RoleExpert)})
	if err != nil {
		t.Fatal(err)
	}
	if released.Credential == nil || released.Case.Status != domain.StatusReleased {
		t.Fatalf("release incomplete: %+v", released)
	}
	verified, err := service.VerifyCredential(ctx, c.ID)
	if err != nil || !verified.Valid {
		t.Fatalf("credential invalid: %+v %v", verified, err)
	}
}

func meta(version int64, key string, role domain.Role) application.CommandMeta {
	return application.CommandMeta{ExpectedVersion: version, IdempotencyKey: key + "-12345678", Actor: "测试操作人", Role: role, Reason: "自动化测试"}
}
