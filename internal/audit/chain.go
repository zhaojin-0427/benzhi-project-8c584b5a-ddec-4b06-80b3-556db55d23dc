package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"manuscript-conservation-gate/internal/domain"
)

type chainPayload struct {
	Sequence      int64       `json:"sequence"`
	CaseID        string      `json:"caseID"`
	EventType     string      `json:"eventType"`
	Actor         string      `json:"actor"`
	Role          domain.Role `json:"role"`
	Reason        string      `json:"reason"`
	EntityVersion int64       `json:"entityVersion"`
	OccurredAt    string      `json:"occurredAt"`
	PreviousHash  string      `json:"previousHash"`
}

func Append(c *domain.ConservationCase, event domain.Event) domain.AuditRecord {
	record := domain.AuditRecord{Sequence: int64(len(c.AuditTrail) + 1), CaseID: c.ID, EventType: event.Type,
		Actor: event.Actor, Role: event.Role, Reason: event.Reason, EntityVersion: event.Version,
		OccurredAt: event.OccurredAt.UTC(), PreviousHash: c.AuditHeadHash}
	record.Hash = recordHash(record)
	c.AuditTrail = append(c.AuditTrail, record)
	c.AuditHeadHash = record.Hash
	return record
}

func Verify(records []domain.AuditRecord) error {
	previous := ""
	for i, record := range records {
		if record.Sequence != int64(i+1) {
			return fmt.Errorf("%w: 审计序号不连续", domain.ErrIntegrity)
		}
		if record.PreviousHash != previous {
			return fmt.Errorf("%w: 审计前序哈希不匹配", domain.ErrIntegrity)
		}
		if record.Hash != recordHash(record) {
			return fmt.Errorf("%w: 审计哈希不匹配", domain.ErrIntegrity)
		}
		previous = record.Hash
	}
	return nil
}

func recordHash(r domain.AuditRecord) string {
	payload := chainPayload{Sequence: r.Sequence, CaseID: r.CaseID, EventType: r.EventType, Actor: r.Actor,
		Role: r.Role, Reason: r.Reason, EntityVersion: r.EntityVersion, OccurredAt: r.OccurredAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), PreviousHash: r.PreviousHash}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
