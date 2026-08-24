package domain

import (
	"errors"
	"testing"
	"time"
)

func TestCompatibilityBlockingRules(t *testing.T) {
	now := time.Date(2026, 2, 1, 9, 0, 0, 0, time.UTC)
	c, err := NewCase("case", "ACC-002", "库房 A2", "古籍", "修复师", now)
	if err != nil {
		t.Fatal(err)
	}
	damage, _ := NewDamage("damage", c.ID, "第 2 叶", "霉蚀", "左上角", SeverityHigh, "紫外照片可见霉斑", "修复师", now)
	if err := c.AddDamage(damage, now); err != nil {
		t.Fatal(err)
	}
	plan, err := NewPlan("plan", c, []TreatmentStep{{Description: "局部托补", Technique: "托补"}}, []Material{{Name: "酸性胶", Category: "粘合剂", PH: 4, Reversible: false, ContainsMetal: true, WaterBased: true}}, "酸化", "水敏颜料", "不可拆", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddPlan(plan, now); err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitPlan(now); err != nil {
		t.Fatal(err)
	}
	a, err := EvaluateCompatibility("assessment", plan, "核验员", now)
	if err != nil {
		t.Fatal(err)
	}
	if a.BlockingCount < 4 {
		t.Fatalf("expected multiple blockers, got %d", a.BlockingCount)
	}
	if err := c.AddAssessment(a, now); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReturned {
		t.Fatalf("expected returned status, got %s", c.Status)
	}
	if err := c.RecordSample("条件", "观察", SamplePass, "核验员", now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected transition error, got %v", err)
	}
}

func TestSubmittedPlanCannotBeOverwritten(t *testing.T) {
	now := time.Now().UTC()
	c, _ := NewCase("case", "ACC-003", "库房 A3", "古籍", "修复师", now)
	damage, _ := NewDamage("damage", c.ID, "第 1 叶", "撕裂", "书口", SeverityLow, "照片显示轻微撕裂", "修复师", now)
	_ = c.AddDamage(damage, now)
	plan, _ := NewPlan("plan", c, []TreatmentStep{{Description: "托补", Technique: "原位"}}, []Material{{Name: "补纸", Category: "纸", PH: 7, Reversible: true}}, "", "", "", "", now)
	_ = c.AddPlan(plan, now)
	_ = c.SubmitPlan(now)
	replacement, err := NewPlan("replacement", c, plan.Steps, plan.Materials, "", "", "", "直接覆盖", now)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected immutable submitted revision, got plan=%v err=%v", replacement.ID, err)
	}
}
