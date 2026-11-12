package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/matching"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pairLimit = 500
	txLimit   = 5000
	// ceiling for 1:N subset enumeration; buckets are truncated at this size.
	maxBucket = 8
)

var (
	ErrConflict        = errors.New("conflict")
	ErrUnbalanced      = errors.New("amounts do not balance")
	ErrNoProposedGroup = errors.New("group is not proposed")
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

type RunResult struct {
	Candidates     int `json:"candidates"`
	ProposedGroups int `json:"proposed_groups"`
	WeightsVersion int `json:"weights_version"`
}

type candidate struct {
	creditIDs []uuid.UUID
	debitIDs  []uuid.UUID
	score     float64
	comps     map[string]float64
	fp        string
}

// Run generates candidates (indexed SQL pairs + bounded N-ary subsets), scores
// them, resolves conflicts greedily, and replaces proposed groups atomically.
func (s *Service) Run(ctx context.Context, orgID, actorID uuid.UUID) (*RunResult, error) {
	w, err := s.loadWeights(ctx)
	if err != nil {
		return nil, err
	}
	txRows, err := s.queries.ListUnmatchedTransactions(ctx, db.ListUnmatchedTransactionsParams{
		OrganizationID: orgID, TxLimit: txLimit,
	})
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]matching.Tx{}
	for _, r := range txRows {
		byID[r.ID] = toMatchTx(r)
	}

	pairs, err := s.queries.ListCandidatePairs(ctx, db.ListCandidatePairsParams{
		ToleranceBp:    int64(w.AmountToleranceBP),
		WindowDays:     int32(w.DateWindowDays),
		OrganizationID: orgID,
		PairLimit:      pairLimit,
	})
	if err != nil {
		return nil, err
	}
	// Load pair members missing from the (capped) unmatched set.
	missing := map[uuid.UUID]bool{}
	for _, p := range pairs {
		if _, ok := byID[p.DebitID]; !ok {
			missing[p.DebitID] = true
		}
		if _, ok := byID[p.CreditID]; !ok {
			missing[p.CreditID] = true
		}
	}
	if len(missing) > 0 {
		ids := make([]uuid.UUID, 0, len(missing))
		for id := range missing {
			ids = append(ids, id)
		}
		extra, err := s.queries.GetTransactionsByIDs(ctx, db.GetTransactionsByIDsParams{
			OrganizationID: orgID, Ids: ids,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range extra {
			byID[r.ID] = toMatchTx(r)
		}
	}

	cands := map[string]*candidate{}
	add := func(creditIDs, debitIDs []uuid.UUID) {
		fp := matching.Fingerprint(creditIDs, debitIDs)
		if _, ok := cands[fp]; ok {
			return
		}
		cands[fp] = &candidate{creditIDs: creditIDs, debitIDs: debitIDs, fp: fp}
	}
	for _, p := range pairs {
		add([]uuid.UUID{p.CreditID}, []uuid.UUID{p.DebitID})
	}
	// N-ary: subsets of one side whose exact sum equals a counterparty tx.
	genNary(byID, w.DateWindowDays, add)

	// Score + filter.
	all := make([]*candidate, 0, len(cands))
	for _, c := range cands {
		res := matching.Score(txsFor(byID, c.debitIDs), txsFor(byID, c.creditIDs), *w)
		if res.Score < w.MinScore {
			continue
		}
		c.score = res.Score
		c.comps = res.Components
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].fp < all[j].fp // deterministic tie-break
	})

	// Greedy conflict resolution: each tx joins at most one proposed group.
	used := map[uuid.UUID]bool{}
	var proposed []*candidate
	for _, c := range all {
		busy := false
		members := append(append([]uuid.UUID{}, c.creditIDs...), c.debitIDs...)
		for _, id := range members {
			if used[id] {
				busy = true
				break
			}
		}
		if busy {
			continue
		}
		for _, id := range members {
			used[id] = true
		}
		proposed = append(proposed, c)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.queries.WithTx(tx)
	if err := lockOrg(ctx, tx, orgID); err != nil {
		return nil, err
	}
	if err := qtx.ClearMatchStateForOrg(ctx, orgID); err != nil {
		return nil, err
	}
	if err := qtx.DeleteProposedGroups(ctx, orgID); err != nil {
		return nil, err
	}
	for _, c := range all {
		comps, _ := json.Marshal(c.comps)
		if err := qtx.InsertMatchCandidate(ctx, db.InsertMatchCandidateParams{
			OrganizationID: orgID,
			CreditIds:      c.creditIDs,
			DebitIds:       c.debitIDs,
			Score:          c.score,
			Components:     comps,
			WeightsVersion: int32(w.Version),
		}); err != nil {
			return nil, err
		}
	}
	for _, c := range proposed {
		// Re-check under org lock: a concurrent approve/manual may have
		// claimed these txs while we were scoring.
		members := append(append([]uuid.UUID{}, c.creditIDs...), c.debitIDs...)
		n, err := qtx.GetActiveItemCountForTxs(ctx, members)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			continue
		}
		comps, _ := json.Marshal(c.comps)
		g, err := qtx.CreateMatchGroup(ctx, db.CreateMatchGroupParams{
			OrganizationID: orgID,
			Status:         "proposed",
			Kind:           matching.Kind(len(c.debitIDs), len(c.creditIDs)),
			Score:          pgtype.Float8{Float64: c.score, Valid: true},
			WeightsVersion: pgtype.Int4{Int32: int32(w.Version), Valid: true},
			Components:     comps,
			CreatedBy:      &actorID,
		})
		if err != nil {
			return nil, err
		}
		if err := addItems(ctx, qtx, g.ID, c.creditIDs, "credit"); err != nil {
			return nil, err
		}
		if err := addItems(ctx, qtx, g.ID, c.debitIDs, "debit"); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "reconciliation.run", EntityType: "match_group",
		NewState: map[string]any{"candidates": len(all), "proposed": len(proposed), "weights_version": w.Version},
	})
	return &RunResult{Candidates: len(all), ProposedGroups: len(proposed), WeightsVersion: w.Version}, nil
}
