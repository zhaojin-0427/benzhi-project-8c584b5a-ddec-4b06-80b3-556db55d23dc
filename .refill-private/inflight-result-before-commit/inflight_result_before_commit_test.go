package inflight_result_before_commit_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/domain"
)

var errCommitRejected = errors.New("injected commit rejection")

type controlledRepository struct {
	commitStarted chan struct{}
	releaseCommit chan struct{}

	mu                sync.Mutex
	cancelNextLookup  context.CancelFunc
	commitStartedOnce sync.Once
}

func (r *controlledRepository) Get(context.Context, string) (*domain.ConservationCase, error) {
	return nil, domain.ErrNotFound
}

func (r *controlledRepository) List(context.Context) ([]*domain.ConservationCase, error) {
	return nil, nil
}

func (r *controlledRepository) Query(context.Context, application.CaseQuery) (application.CasePage, error) {
	return application.CasePage{}, nil
}

func (r *controlledRepository) LookupIdempotency(context.Context, string, string, string) (domain.CommitResult, bool, error) {
	r.mu.Lock()
	cancel := r.cancelNextLookup
	r.cancelNextLookup = nil
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return domain.CommitResult{}, false, nil
}

func (r *controlledRepository) Commit(context.Context, domain.CommitRequest) (domain.CommitResult, error) {
	r.commitStartedOnce.Do(func() { close(r.commitStarted) })
	<-r.releaseCommit
	return domain.CommitResult{}, errCommitRejected
}

func (r *controlledRepository) Verify(context.Context) error { return nil }

func (r *controlledRepository) cancelWhenNextLookupReturns(cancel context.CancelFunc) {
	r.mu.Lock()
	r.cancelNextLookup = cancel
	r.mu.Unlock()
}

func TestInflightRetryWaitsForCommitOutcome(t *testing.T) {
	repository := &controlledRepository{
		commitStarted: make(chan struct{}),
		releaseCommit: make(chan struct{}),
	}
	service := application.NewService(repository, nil)
	command := application.CreateCaseCommand{
		CommandMeta: application.CommandMeta{
			ExpectedVersion: 0,
			IdempotencyKey:  "inflight-retry-key",
			Actor:           "修复师甲",
			Role:            domain.RoleConservator,
		},
		AccessionCode:          "INFLIGHT-001",
		ShelfLocation:          "库房一架",
		Title:                  "在途提交测试",
		ResponsibleConservator: "修复师甲",
	}

	leaderDone := make(chan error, 1)
	go func() {
		_, err := service.CreateCase(context.Background(), command)
		leaderDone <- err
	}()
	<-repository.commitStarted

	retryContext, cancelRetry := context.WithCancel(context.Background())
	repository.cancelWhenNextLookupReturns(cancelRetry)
	result, retryErr := service.CreateCase(retryContext, command)
	close(repository.releaseCommit)
	leaderErr := <-leaderDone

	if !errors.Is(leaderErr, errCommitRejected) {
		t.Fatalf("leader should expose the final commit rejection, got %v", leaderErr)
	}
	if !errors.Is(retryErr, context.Canceled) {
		t.Fatalf("retry observed provisional success before commit completed: err=%v case=%v", retryErr, result.Case)
	}
}
