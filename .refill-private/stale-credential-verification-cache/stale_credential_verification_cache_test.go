package stalecredentialverificationcache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

type caseOverrideRepository struct {
	application.Repository
	value *domain.ConservationCase
}

func (r *caseOverrideRepository) Get(context.Context, string) (*domain.ConservationCase, error) {
	return r.value.Clone(), nil
}

func TestCredentialSignatureCacheRejectsChangedCredential(t *testing.T) {
	ctx := context.Background()
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("private-test-credential-secret")
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewService(repository, issuer)
	released := releaseCase(t, ctx, service)

	first, err := service.VerifyCredential(ctx, released.Case.ID)
	if err != nil || !first.Valid {
		t.Fatalf("首次校验合法凭据失败: valid=%v err=%v", first.Valid, err)
	}

	variants := []struct {
		name   string
		change func(*domain.ReleaseCredential)
	}{
		{name: "Signature", change: func(c *domain.ReleaseCredential) { c.Signature = strings.Repeat("0", 64) }},
		{name: "ApprovedBy", change: func(c *domain.ReleaseCredential) { c.ApprovedBy = "被替换的批准人" }},
		{name: "IssuedAt", change: func(c *domain.ReleaseCredential) { c.IssuedAt = c.IssuedAt.Add(time.Second) }},
		{name: "CaseID", change: func(c *domain.ReleaseCredential) { c.CaseID = "被替换的档案" }},
		{name: "FrozenPlanRevisionID", change: func(c *domain.ReleaseCredential) { c.FrozenPlanRevisionID = "被替换的方案" }},
	}
	accepted := make([]string, 0, len(variants))
	for _, variant := range variants {
		changed := released.Case.Clone()
		variant.change(changed.Credential)
		if domain.VerifyCredentialSignature(*changed.Credential, "private-test-credential-secret") {
			t.Fatalf("测试前提错误：修改 %s 后签名仍然有效", variant.name)
		}
		changedRepository := &caseOverrideRepository{Repository: repository, value: changed}
		changedService := application.NewService(changedRepository, issuer)
		second, err := changedService.VerifyCredential(ctx, changed.ID)
		if err != nil {
			t.Fatalf("修改 %s 后再次校验返回非预期错误: %v", variant.name, err)
		}
		if second.Valid {
			accepted = append(accepted, variant.name)
		}
	}
	if len(accepted) > 0 {
		t.Fatalf("同一凭据 ID 的签名输入已经变化，校验缓存却仍返回 Valid=true: %v", accepted)
	}
}

func releaseCase(t *testing.T, ctx context.Context, service *application.Service) application.OperationResult {
	t.Helper()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{
		CommandMeta:            meta(0, "create-credential", domain.RoleConservator),
		AccessionCode:          "CACHE-001",
		ShelfLocation:          "珍本库一架",
		Title:                  "缓存校验测试经卷",
		ResponsibleConservator: "修复师甲",
	})
	if err != nil {
		t.Fatal(err)
	}
	damaged, err := service.AddDamage(ctx, created.Case.ID, application.AddDamageCommand{
		CommandMeta: meta(created.Case.Version, "damage-credential", domain.RoleConservator),
		FolioRef:    "第一叶", DamageType: "撕裂", Extent: "书口二厘米",
		Severity: domain.SeverityModerate, EvidenceNote: "可见纤维断裂",
	})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := service.CreatePlan(ctx, created.Case.ID, application.CreatePlanCommand{
		CommandMeta:     meta(damaged.Case.Version, "plan-credential", domain.RoleConservator),
		Steps:           []domain.TreatmentStep{{Description: "清洁并托补", Technique: "原位托补"}},
		Materials:       []domain.Material{{Name: "楮皮纸", Category: "补纸", PH: 7, Reversible: true}},
		PaperConstraint: "轻度酸化", PigmentConstraint: "稳定", BindingConstraint: "保持原装帧",
	})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.SubmitPlan(ctx, created.Case.ID, application.SubmitPlanCommand{CommandMeta: meta(planned.Case.Version, "submit-credential", domain.RoleConservator)})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.AssessCompatibility(ctx, created.Case.ID, application.AssessCommand{CommandMeta: meta(submitted.Case.Version, "assess-credential", domain.RoleVerifier)})
	if err != nil {
		t.Fatal(err)
	}
	sampled, err := service.RecordSample(ctx, created.Case.ID, application.RecordSampleCommand{
		CommandMeta:      meta(assessed.Case.Version, "sample-credential", domain.RoleVerifier),
		SampleConditions: "温度 22℃，湿度 50%", SampleObservations: "干燥后无色差", SampleOutcome: domain.SamplePass,
	})
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := service.Review(ctx, created.Case.ID, application.ReviewCommand{
		CommandMeta: meta(sampled.Case.Version, "review-credential", domain.RoleExpert), Decision: domain.ReviewApproved,
	})
	if err != nil {
		t.Fatal(err)
	}
	released, err := service.Release(ctx, created.Case.ID, application.ReleaseCommand{CommandMeta: meta(reviewed.Case.Version, "release-credential", domain.RoleExpert)})
	if err != nil {
		t.Fatal(err)
	}
	return released
}

func meta(version int64, key string, role domain.Role) application.CommandMeta {
	return application.CommandMeta{
		ExpectedVersion: version,
		IdempotencyKey:  key + "-12345678",
		Actor:           "私有复现操作人",
		Role:            role,
		Reason:          "确定性私有复现",
	}
}
