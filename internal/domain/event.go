package domain

import (
	"encoding/json"
	"time"
)

func NewEvent(id, caseID, eventType, actor string, role Role, reason string, version int64, now time.Time, data any) Event {
	b, _ := json.Marshal(data)
	return Event{ID: id, CaseID: caseID, Type: eventType, Actor: actor, Role: role, Reason: reason, Version: version, OccurredAt: now, Data: b}
}

func ValidRole(role Role) bool {
	return role == RoleConservator || role == RoleVerifier || role == RoleExpert
}

func RequireRole(actual Role, allowed ...Role) error {
	for _, role := range allowed {
		if actual == role {
			return nil
		}
	}
	return ErrForbidden
}
