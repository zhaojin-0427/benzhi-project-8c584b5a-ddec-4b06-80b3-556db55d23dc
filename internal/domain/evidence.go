package domain

import (
	"fmt"
	"sort"
)

// EvidenceSnapshot 是专家复核与最终放行共用的只读门禁证据投影。
type EvidenceSnapshot struct {
	PlanRevisionID   string   `json:"planRevisionID"`
	PlanContentHash  string   `json:"planContentHash"`
	ItemIDs          []string `json:"itemIDs"`
	ConfirmedItemIDs []string `json:"confirmedItemIDs,omitempty"`
	Digest           string   `json:"digest"`
}

func (e EvidenceSnapshot) Clone() EvidenceSnapshot {
	e.ItemIDs = append([]string(nil), e.ItemIDs...)
	e.ConfirmedItemIDs = append([]string(nil), e.ConfirmedItemIDs...)
	return e
}

func (e EvidenceSnapshot) ComputeDigest() string {
	digest, _ := HashJSON(struct {
		Plan  string   `json:"plan"`
		Hash  string   `json:"hash"`
		Items []string `json:"items"`
	}{e.PlanRevisionID, e.PlanContentHash, e.ItemIDs})
	return digest
}

func (c *ConservationCase) BuildEvidenceSnapshot() (EvidenceSnapshot, error) {
	p, err := c.CurrentPlan()
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	a, err := c.currentAssessment()
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	items := []string{"plan:" + p.ID + ":" + p.ContentHash, "coverage:" + p.ContentHash}
	for _, f := range a.RuleFindings {
		items = append(items, "finding:"+f.ID+":"+string(f.Level))
	}
	for _, d := range a.WarningDispositions {
		items = append(items, "warning-disposition:"+d.FindingID)
	}
	for _, round := range a.SampleRounds {
		items = append(items, fmt.Sprintf("sample:%s:%d:%s", round.ID, round.Round, round.Outcome))
	}
	for _, item := range c.RemediationItems {
		if item.PlanRevisionID == p.ID || item.ClosedByPlanRevisionID == p.ID {
			items = append(items, "remediation:"+item.ID+":"+fmt.Sprint(item.Closed))
		}
	}
	sort.Strings(items)
	e := EvidenceSnapshot{PlanRevisionID: p.ID, PlanContentHash: p.ContentHash, ItemIDs: items}
	e.Digest = e.ComputeDigest()
	return e, nil
}

func missingEvidence(required, confirmed []string) []string {
	set := map[string]struct{}{}
	for _, id := range confirmed {
		set[id] = struct{}{}
	}
	missing := []string{}
	for _, id := range required {
		if _, ok := set[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

func (c *ConservationCase) GateEvidenceSnapshot() (EvidenceSnapshot, error) {
	e, err := c.BuildEvidenceSnapshot()
	if err != nil {
		return e, err
	}
	if !c.HasApprovedCurrentPlan() {
		return e, invalid("review", "当前方案尚未获得专家批准")
	}
	for i := len(c.Reviews) - 1; i >= 0; i-- {
		r := c.Reviews[i]
		if r.PlanRevisionID == e.PlanRevisionID && r.Decision == ReviewApproved {
			e.ItemIDs = append(e.ItemIDs, "review:"+r.ID+":"+r.EvidenceSnapshot.Digest)
			break
		}
	}
	sort.Strings(e.ItemIDs)
	e.Digest = e.ComputeDigest()
	return e, nil
}
