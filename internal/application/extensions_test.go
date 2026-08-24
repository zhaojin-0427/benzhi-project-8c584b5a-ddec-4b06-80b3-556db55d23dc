package application_test

import (
	"context"
	"errors"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
	"manuscript-conservation-gate/internal/store"
)

func newExtensionService(t *testing.T) *application.Service {
	t.Helper()
	repository, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := audit.NewIssuer("extension-test-credential-secret")
	if err != nil {
		t.Fatal(err)
	}
	return application.NewService(repository, issuer)
}

func TestDamageBatchIsAtomicAndQueryCountsShareProjection(t *testing.T) {
	service := newExtensionService(t)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{CommandMeta: meta(0, "batch-create", domain.RoleConservator), AccessionCode: "BATCH-001", ShelfLocation: "库房一架", Title: "批量基线测试", ResponsibleConservator: "修复师甲"})
	if err != nil {
		t.Fatal(err)
	}
	records := []domain.DamageInput{{FolioRef: "第 1 叶", DamageType: "撕裂", Extent: "书口 2 cm", Severity: domain.SeverityHigh, EvidenceNote: "照片显示纤维断裂"}, {FolioRef: "第 2 叶", DamageType: "虫蛀", Extent: "版心 1 cm", Severity: domain.SeverityModerate, EvidenceNote: "透射照片显示缺损"}, {FolioRef: "第 3 叶", DamageType: "霉斑", Extent: "左上角", Severity: domain.SeverityLow, EvidenceNote: "紫外照片显示斑迹"}}
	damaged, err := service.AddDamage(ctx, created.Case.ID, application.AddDamageCommand{CommandMeta: meta(created.Case.Version, "batch-damage", domain.RoleConservator), Records: records})
	if err != nil {
		t.Fatal(err)
	}
	if damaged.Case.Version != created.Case.Version+1 || damaged.Case.DamageSummary.Total != 3 {
		t.Fatalf("批次未按一次版本提交: version=%d summary=%+v", damaged.Case.Version, damaged.Case.DamageSummary)
	}
	_, err = service.AddDamage(ctx, created.Case.ID, application.AddDamageCommand{CommandMeta: meta(damaged.Case.Version, "batch-conflict", domain.RoleConservator), Records: []domain.DamageInput{{FolioRef: "第 1 叶", DamageType: "撕裂", Extent: "重复", Severity: domain.SeverityLow, EvidenceNote: "重复证据摘要"}, {FolioRef: "第 4 叶", DamageType: "水渍", Extent: "整页", Severity: domain.SeverityLow, EvidenceNote: "可见光照片显示水渍"}}})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("预期整批重复校验失败，得到 %v", err)
	}
	loaded, _ := service.GetCase(ctx, created.Case.ID)
	if loaded.Version != damaged.Case.Version || len(loaded.Damages) != 3 {
		t.Fatalf("失败批次产生了部分写入: version=%d damages=%d", loaded.Version, len(loaded.Damages))
	}
	page, err := service.QueryCases(ctx, application.CaseQuery{Keyword: "批量", ResponsibleConservator: "修复师甲", Status: domain.StatusDraft, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Counts[domain.StatusDraft] != 1 || page.ProjectionVersion == "" {
		t.Fatalf("查询投影统计不一致: %+v", page)
	}
}

func TestWarningsBlockTwoRoundSampleUntilConfirmed(t *testing.T) {
	service := newExtensionService(t)
	ctx := context.Background()
	created, err := service.CreateCase(ctx, application.CreateCaseCommand{CommandMeta: meta(0, "warning-create", domain.RoleConservator), AccessionCode: "WARN-001", ShelfLocation: "库房二架", Title: "警示试样测试", ResponsibleConservator: "修复师乙"})
	if err != nil {
		t.Fatal(err)
	}
	damaged, err := service.AddDamage(ctx, created.Case.ID, application.AddDamageCommand{CommandMeta: meta(created.Case.Version, "warning-damage", domain.RoleConservator), Records: []domain.DamageInput{{FolioRef: "第 1 叶", DamageType: "脆化", Extent: "整页", Severity: domain.SeverityHigh, EvidenceNote: "弯折测试显示纸张脆化"}}})
	if err != nil {
		t.Fatal(err)
	}
	damageID := damaged.Case.Damages[0].ID
	planned, err := service.CreatePlan(ctx, created.Case.ID, application.CreatePlanCommand{CommandMeta: meta(damaged.Case.Version, "warning-plan", domain.RoleConservator), Steps: []domain.TreatmentStep{{Description: "局部加固", Technique: "原位托补", DamageIDs: []string{damageID}}}, Materials: []domain.Material{{Name: "弱碱补纸", Category: "补纸", PH: 8.2, Reversible: true}}, PaperConstraint: "纸张酸化", RequiredSampleRounds: 2})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service.SubmitPlan(ctx, created.Case.ID, application.SubmitPlanCommand{CommandMeta: meta(planned.Case.Version, "warning-submit", domain.RoleConservator)})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := service.AssessCompatibility(ctx, created.Case.ID, application.AssessCommand{CommandMeta: meta(submitted.Case.Version, "warning-assess", domain.RoleVerifier)})
	if err != nil {
		t.Fatal(err)
	}
	a := assessed.Case.Assessments[0]
	if a.WarningCount != 1 || a.UnconfirmedWarningCount != 1 {
		t.Fatalf("警示统计错误: %+v", a)
	}
	sample := application.RecordSampleCommand{CommandMeta: meta(assessed.Case.Version, "warning-blocked-sample", domain.RoleConservator), Round: 1, MaterialBatch: "BATCH-A", TemperatureC: 22, HumidityPercent: 50, DurationMinutes: 60, ColorDifference: "无明显色差", Deformation: "无明显形变", Observations: "试样干燥后保持稳定", SampleOutcome: domain.SamplePass}
	if _, err := service.RecordSample(ctx, created.Case.ID, sample); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("未确认警示应阻断试样，得到 %v", err)
	}
	confirmed, err := service.AssessCompatibility(ctx, created.Case.ID, application.AssessCommand{CommandMeta: meta(assessed.Case.Version, "warning-confirm", domain.RoleVerifier), WarningDispositions: []domain.WarningDisposition{{FindingID: a.RuleFindings[0].ID, Method: "限制局部用量", ControlMeasures: "分区操作并逐分钟观察色差变化"}}})
	if err != nil {
		t.Fatal(err)
	}
	sample.CommandMeta = meta(confirmed.Case.Version, "warning-round-one", domain.RoleConservator)
	first, err := service.RecordSample(ctx, created.Case.ID, sample)
	if err != nil {
		t.Fatal(err)
	}
	if first.Case.Status != domain.StatusPendingSample {
		t.Fatalf("首轮通过后状态错误: %s", first.Case.Status)
	}
	sample.CommandMeta = meta(first.Case.Version, "warning-round-two", domain.RoleConservator)
	sample.Round = 2
	second, err := service.RecordSample(ctx, created.Case.ID, sample)
	if err != nil {
		t.Fatal(err)
	}
	if second.Case.Status != domain.StatusPendingReview {
		t.Fatalf("两轮通过后状态错误: %s", second.Case.Status)
	}
}
