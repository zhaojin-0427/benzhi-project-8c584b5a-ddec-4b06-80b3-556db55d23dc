package domain

import (
	"strconv"
	"strings"
	"time"
)

type ReviewComment struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	FolioRef   string `json:"folioRef,omitempty"`
	ObjectType string `json:"objectType,omitempty"`
	ObjectID   string `json:"objectID,omitempty"`
	Comment    string `json:"comment"`
	Required   bool   `json:"required"`
}

type ExpertReview struct {
	ID               string           `json:"id"`
	PlanRevisionID   string           `json:"planRevisionID"`
	Decision         ReviewDecision   `json:"decision"`
	Comments         []ReviewComment  `json:"comments"`
	EvidenceSnapshot EvidenceSnapshot `json:"evidenceSnapshot"`
	ReviewedBy       string           `json:"reviewedBy"`
	ReviewedAt       time.Time        `json:"reviewedAt"`
}

func NewReview(id, planID string, decision ReviewDecision, comments []ReviewComment, snapshot EvidenceSnapshot, actor string, now time.Time) (ExpertReview, error) {
	r := ExpertReview{ID: id, PlanRevisionID: planID, Decision: decision, Comments: append([]ReviewComment(nil), comments...), EvidenceSnapshot: snapshot.Clone(), ReviewedBy: strings.TrimSpace(actor), ReviewedAt: now}
	if r.ReviewedBy == "" {
		return r, invalid("actor", "复核专家不能为空")
	}
	if decision != ReviewApproved && decision != ReviewReturned {
		return r, invalid("decision", "复核决定无效")
	}
	if decision == ReviewReturned && len(r.Comments) == 0 {
		return r, invalid("comments", "退回时必须提供结构化意见")
	}
	for i := range r.Comments {
		r.Comments[i].ID = strings.TrimSpace(r.Comments[i].ID)
		if r.Comments[i].ID == "" {
			r.Comments[i].ID = id + "_comment_" + strconv.Itoa(i+1)
		}
		r.Comments[i].Category = strings.TrimSpace(r.Comments[i].Category)
		r.Comments[i].Comment = strings.TrimSpace(r.Comments[i].Comment)
		if r.Comments[i].Category == "" || r.Comments[i].Comment == "" {
			return r, invalid("comments", "意见类别和内容不能为空")
		}
		if r.Comments[i].Required && strings.TrimSpace(r.Comments[i].FolioRef) == "" && (r.Comments[i].ObjectType == "" || r.Comments[i].ObjectID == "") {
			return r, invalid("comments", "必改意见必须关联页位、步骤、材料或试样轮次")
		}
	}
	return r, nil
}

func (c *ConservationCase) ApplyReview(review ExpertReview, now time.Time) error {
	if c.Status != StatusPendingReview {
		return ErrInvalidTransition
	}
	if review.PlanRevisionID != c.CurrentPlanRevisionID {
		return invalid("planRevisionID", "只能复核当前方案版本")
	}
	a, err := c.currentAssessment()
	if err != nil {
		return err
	}
	if a.BlockingCount != 0 || a.SampleOutcome != SamplePass {
		return invalid("assessment", "相容性核验或模拟试样尚未通过")
	}
	if err := c.validateReviewReferences(review.Comments); err != nil {
		return err
	}
	current, err := c.BuildEvidenceSnapshot()
	if err != nil {
		return err
	}
	if review.EvidenceSnapshot.Digest != "" && review.EvidenceSnapshot.Digest != current.Digest {
		return invalid("evidenceDigest", "复核证据已变化，请刷新后重新确认")
	}
	confirmed := append([]string(nil), review.EvidenceSnapshot.ConfirmedItemIDs...)
	review.EvidenceSnapshot = current
	review.EvidenceSnapshot.ConfirmedItemIDs = confirmed
	if review.Decision == ReviewApproved {
		if review.EvidenceSnapshot.PlanRevisionID != current.PlanRevisionID || review.EvidenceSnapshot.PlanContentHash != current.PlanContentHash {
			return invalid("planRevisionID", "批准请求使用了过期方案或摘要")
		}
		missing := missingEvidence(current.ItemIDs, review.EvidenceSnapshot.ConfirmedItemIDs)
		if len(missing) > 0 {
			return invalid("confirmedEvidenceItemIDs", "批准前必须确认全部证据项："+strings.Join(missing, "、"))
		}
		review.EvidenceSnapshot.ConfirmedItemIDs = append([]string(nil), current.ItemIDs...)
	}
	c.Reviews = append(c.Reviews, review)
	if review.Decision == ReviewReturned {
		for _, comment := range review.Comments {
			if comment.Required {
				c.addRemediationItem(review.PlanRevisionID, "EXPERT_COMMENT", comment.ID, comment.FolioRef, comment.Comment)
			}
		}
		c.Advance(StatusReturned, now)
	} else {
		c.Advance(StatusApproved, now)
	}
	return nil
}

func (c *ConservationCase) validateReviewReferences(comments []ReviewComment) error {
	p, _ := c.CurrentPlan()
	a, _ := c.currentAssessment()
	for _, comment := range comments {
		if !comment.Required {
			continue
		}
		kind := strings.TrimSpace(comment.ObjectType)
		id := strings.TrimSpace(comment.ObjectID)
		if strings.TrimSpace(comment.FolioRef) != "" {
			kind = "folio"
			id = strings.TrimSpace(comment.FolioRef)
		}
		valid := false
		switch kind {
		case "folio":
			for _, d := range c.Damages {
				if d.FolioRef == id || d.ID == id {
					valid = true
				}
			}
		case "step":
			for _, step := range p.Steps {
				if id == strconv.Itoa(step.Order) {
					valid = true
				}
			}
		case "material":
			for _, material := range p.Materials {
				if material.Name == id {
					valid = true
				}
			}
		case "sample":
			for _, round := range a.SampleRounds {
				if round.ID == id || id == strconv.Itoa(round.Round) {
					valid = true
				}
			}
		}
		if !valid {
			return invalid("comments.objectID", "必改意见关联了当前方案中不存在的页位、步骤、材料或试样轮次："+id)
		}
	}
	return nil
}

func (c *ConservationCase) HasApprovedCurrentPlan() bool {
	for i := len(c.Reviews) - 1; i >= 0; i-- {
		if c.Reviews[i].PlanRevisionID == c.CurrentPlanRevisionID {
			return c.Reviews[i].Decision == ReviewApproved
		}
	}
	return false
}
