package application

import (
	"context"

	"manuscript-conservation-gate/internal/domain"
)

type Repository interface {
	Get(context.Context, string) (*domain.ConservationCase, error)
	List(context.Context) ([]*domain.ConservationCase, error)
	Query(context.Context, CaseQuery) (CasePage, error)
	LookupIdempotency(context.Context, string, string, string) (domain.CommitResult, bool, error)
	Commit(context.Context, domain.CommitRequest) (domain.CommitResult, error)
	Verify(context.Context) error
}
