package query_leader_context_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/domain"
)

type controlledRepository struct {
	firstEntered       chan struct{}
	secondReady        chan struct{}
	firstReadyToFinish chan struct{}
	finishQueries      chan struct{}
	readyOnce          sync.Once
	queryCalls         atomic.Int32
}

func (r *controlledRepository) Get(context.Context, string) (*domain.ConservationCase, error) {
	panic("unexpected Get call")
}

func (r *controlledRepository) List(context.Context) ([]*domain.ConservationCase, error) {
	panic("unexpected List call")
}

func (r *controlledRepository) Query(ctx context.Context, query application.CaseQuery) (application.CasePage, error) {
	if r.queryCalls.Add(1) == 1 {
		close(r.firstEntered)
		<-r.secondReady
		close(r.firstReadyToFinish)
		<-r.finishQueries
		if err := ctx.Err(); err != nil {
			return application.CasePage{}, err
		}
		return application.CasePage{Page: query.Page, PageSize: query.PageSize, Total: 1}, nil
	}
	r.markSecondReady()
	<-r.finishQueries
	return application.CasePage{Page: query.Page, PageSize: query.PageSize, Total: 1}, nil
}

func (r *controlledRepository) LookupIdempotency(context.Context, string, string, string) (domain.CommitResult, bool, error) {
	panic("unexpected LookupIdempotency call")
}

func (r *controlledRepository) Commit(context.Context, domain.CommitRequest) (domain.CommitResult, error) {
	panic("unexpected Commit call")
}

func (r *controlledRepository) Verify(context.Context) error {
	panic("unexpected Verify call")
}

func (r *controlledRepository) markSecondReady() {
	r.readyOnce.Do(func() { close(r.secondReady) })
}

type observedContext struct {
	context.Context
	repository *controlledRepository
}

func (c observedContext) Done() <-chan struct{} {
	c.repository.markSecondReady()
	return c.Context.Done()
}

type queryResult struct {
	page application.CasePage
	err  error
}

func TestConcurrentQueryDoesNotInheritCanceledLeaderContext(t *testing.T) {
	repository := &controlledRepository{
		firstEntered:       make(chan struct{}),
		secondReady:        make(chan struct{}),
		firstReadyToFinish: make(chan struct{}),
		finishQueries:      make(chan struct{}),
	}
	service := application.NewService(repository, nil)
	query := application.CaseQuery{Page: 1, PageSize: 25}

	leaderContext, cancelLeader := context.WithCancel(context.Background())
	defer cancelLeader()
	leaderDone := make(chan error, 1)
	go func() {
		_, err := service.QueryCases(leaderContext, query)
		leaderDone <- err
	}()
	<-repository.firstEntered

	followerDone := make(chan queryResult, 1)
	followerContext := observedContext{Context: context.Background(), repository: repository}
	go func() {
		page, err := service.QueryCases(followerContext, query)
		followerDone <- queryResult{page: page, err: err}
	}()
	<-repository.firstReadyToFinish
	cancelLeader()
	close(repository.finishQueries)

	if err := <-leaderDone; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("首请求返回了非预期错误: %v", err)
	}
	follower := <-followerDone
	if follower.err != nil {
		t.Fatalf("仍存活的第二个请求继承了首请求的取消结果: %v", follower.err)
	}
	if follower.page.Total != 1 {
		t.Fatalf("仍存活的第二个请求未获得仓储结果: %+v", follower.page)
	}
}

var _ context.Context = observedContext{}
var _ application.Repository = (*controlledRepository)(nil)
