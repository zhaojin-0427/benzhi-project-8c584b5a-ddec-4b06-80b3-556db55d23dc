package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"manuscript-conservation-gate/internal/application"
	"manuscript-conservation-gate/internal/audit"
	"manuscript-conservation-gate/internal/domain"
)

type Store struct {
	mu           sync.RWMutex
	dir          string
	logPath      string
	logFile      *os.File
	snapshotPath string
	cases        map[string]*domain.ConservationCase
	idempotency  map[string]domain.IdempotencyRecord
	sequence     int64
	headHash     string
}

func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	s := &Store{dir: dir, logPath: filepath.Join(dir, "events.jsonl"), snapshotPath: filepath.Join(dir, "projection.json"), cases: map[string]*domain.ConservationCase{}, idempotency: map[string]domain.IdempotencyRecord{}}
	if err := s.recover(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Get(_ context.Context, id string) (*domain.ConservationCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.cases[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c.Clone(), nil
}

func (s *Store) List(_ context.Context) ([]*domain.ConservationCase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]*domain.ConservationCase, 0, len(s.cases))
	for _, c := range s.cases {
		items = append(items, c.Clone())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (s *Store) Query(_ context.Context, query application.CaseQuery) (application.CasePage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	conservator := strings.ToLower(strings.TrimSpace(query.ResponsibleConservator))
	counts := map[domain.Status]int{domain.StatusDraft: 0, domain.StatusPendingCompatibility: 0, domain.StatusPendingSample: 0, domain.StatusPendingReview: 0, domain.StatusReturned: 0, domain.StatusApproved: 0, domain.StatusReleased: 0}
	base := make([]*domain.ConservationCase, 0, len(s.cases))
	for _, c := range s.cases {
		if keyword != "" && !strings.Contains(strings.ToLower(c.AccessionCode+"\x1f"+c.Title), keyword) {
			continue
		}
		if conservator != "" && !strings.Contains(strings.ToLower(c.ResponsibleConservator), conservator) {
			continue
		}
		counts[c.Status]++
		if query.Status != "" && c.Status != query.Status {
			continue
		}
		base = append(base, c.Clone())
	}
	sort.Slice(base, func(i, j int) bool {
		if base[i].UpdatedAt.Equal(base[j].UpdatedAt) {
			return base[i].ID < base[j].ID
		}
		return base[i].UpdatedAt.After(base[j].UpdatedAt)
	})
	total := len(base)
	start := (query.Page - 1) * query.PageSize
	if start > total {
		start = total
	}
	end := start + query.PageSize
	if end > total {
		end = total
	}
	return application.CasePage{Cases: base[start:end], Total: total, Page: query.Page, PageSize: query.PageSize, Counts: counts, ProjectionVersion: s.headHash}, nil
}

func (s *Store) LookupIdempotency(_ context.Context, scope, key, payloadHash string) (domain.CommitResult, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.idempotency[idempotencyID(scope, key)]
	if !ok {
		return domain.CommitResult{}, false, nil
	}
	if record.PayloadHash != payloadHash {
		return domain.CommitResult{}, true, domain.ErrIdempotencyConflict
	}
	return domain.CommitResult{Response: cloneRaw(record.Response), Duplicate: true}, true, nil
}

func (s *Store) Commit(ctx context.Context, request domain.CommitRequest) (domain.CommitResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CommitResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := idempotencyID(request.Scope, request.IdempotencyKey)
	if record, ok := s.idempotency[key]; ok {
		if record.PayloadHash != request.PayloadHash {
			return domain.CommitResult{}, domain.ErrIdempotencyConflict
		}
		return domain.CommitResult{Response: cloneRaw(record.Response), Duplicate: true}, nil
	}
	current, exists := s.cases[request.Case.ID]
	if request.ExpectedVersion == 0 {
		if exists {
			return domain.CommitResult{}, domain.ErrVersionConflict
		}
		for _, item := range s.cases {
			if item.AccessionCode == request.Case.AccessionCode {
				return domain.CommitResult{}, domain.ErrDuplicateAccession
			}
		}
	} else if !exists || current.Version != request.ExpectedVersion {
		return domain.CommitResult{}, domain.ErrVersionConflict
	}
	if err := audit.Verify(request.Case.AuditTrail); err != nil {
		return domain.CommitResult{}, err
	}
	if err := domain.ValidateAggregate(request.Case); err != nil {
		return domain.CommitResult{}, err
	}
	record := domain.IdempotencyRecord{Scope: request.Scope, Key: request.IdempotencyKey, PayloadHash: request.PayloadHash, Response: cloneRaw(request.Response), CreatedAt: time.Now().UTC()}
	envelope := transactionEnvelope{SchemaVersion: schemaVersion, Sequence: s.sequence + 1, PreviousHash: s.headHash, CommittedAt: time.Now().UTC(), Event: request.Event, Case: request.Case.Clone(), Idempotency: record}
	hash, err := envelopeHash(envelope)
	if err != nil {
		return domain.CommitResult{}, err
	}
	envelope.Hash = hash
	if err := s.appendEnvelope(envelope); err != nil {
		return domain.CommitResult{}, err
	}
	s.cases[request.Case.ID] = request.Case.Clone()
	s.idempotency[key] = record
	s.sequence, s.headHash = envelope.Sequence, envelope.Hash
	if err := s.writeSnapshot(); err != nil {
		return domain.CommitResult{}, fmt.Errorf("事件已持久化但投影写入失败，重启后可恢复: %w", err)
	}
	return domain.CommitResult{Response: cloneRaw(request.Response)}, nil
}

func (s *Store) Verify(_ context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cases {
		if err := audit.Verify(c.AuditTrail); err != nil {
			return err
		}
		if err := domain.ValidateAggregate(c); err != nil {
			return err
		}
	}
	_, _, err := replayLog(s.logPath)
	return err
}

func idempotencyID(scope, key string) string { return scope + "\x1f" + key }
