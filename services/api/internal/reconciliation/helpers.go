package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cocok/cocok/services/api/internal/matching"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// lockOrg serializes group mutations per organization (advisory xact lock;
// released on commit/rollback).
func lockOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, orgID.String())
	return err
}

func (s *Service) loadWeights(ctx context.Context) (*matching.Weights, error) {
	row, err := s.queries.GetActiveScoringWeights(ctx)
	if err != nil {
		return nil, fmt.Errorf("load scoring weights: %w", err)
	}
	w := &matching.Weights{
		Version:           int(row.Version),
		MinScore:          row.MinScore,
		AmountToleranceBP: int(row.AmountToleranceBp),
		DateWindowDays:    int(row.DateWindowDays),
	}
	if err := json.Unmarshal(row.Weights, w); err != nil {
		return nil, fmt.Errorf("corrupt scoring weights: %w", err)
	}
	return w, nil
}

func toMatchTx(r db.Transaction) matching.Tx {
	return matching.Tx{
		ID:           r.ID,
		BookedAt:     r.BookedAt,
		Direction:    r.Direction,
		AmountMinor:  r.AmountMinor,
		Description:  r.Description,
		Counterparty: r.Counterparty.String,
		ExternalRef:  r.ExternalRef.String,
	}
}

func txsFor(byID map[uuid.UUID]matching.Tx, ids []uuid.UUID) []matching.Tx {
	out := make([]matching.Tx, 0, len(ids))
	for _, id := range ids {
		if t, ok := byID[id]; ok {
			out = append(out, t)
		}
	}
	return out
}

type txQuerier interface {
	AddMatchGroupItem(ctx context.Context, arg db.AddMatchGroupItemParams) error
}

func addItems(ctx context.Context, q txQuerier, groupID uuid.UUID, ids []uuid.UUID, side string) error {
	for _, id := range ids {
		if err := q.AddMatchGroupItem(ctx, db.AddMatchGroupItemParams{
			MatchGroupID: groupID, TransactionID: id, Side: side,
		}); err != nil {
			return err
		}
	}
	return nil
}

// genNary enumerates bounded subsets (2..maxBucket members) grouped by
// normalized counterparty or booking date, emitting candidates whose exact
// integer sum equals an opposite-direction tx in the date window.
// Ceiling: buckets truncate at maxBucket — raise when statements need more.
func genNary(byID map[uuid.UUID]matching.Tx, windowDays int, add func(credit, debit []uuid.UUID)) {
	credits, debits := buckets(byID, "credit"), buckets(byID, "debit")
	emit := func(bucket []matching.Tx, targets []matching.Tx, memberSide string) {
		if len(bucket) < 2 {
			return
		}
		if len(bucket) > maxBucket {
			bucket = bucket[:maxBucket]
		}
		n := len(bucket)
		for mask := 1; mask < (1<<n)-1; mask++ {
			pop := 0
			for b := mask; b > 0; b &= b - 1 {
				pop++
			}
			if pop < 2 {
				continue
			}
			var members []matching.Tx
			var sum int64
			for i := 0; i < n; i++ {
				if mask&(1<<i) != 0 {
					members = append(members, bucket[i])
					sum += bucket[i].AmountMinor
				}
			}
			for _, t := range targets {
				if t.AmountMinor != sum || !inWindow(members, t, windowDays) {
					continue
				}
				var creditIDs, debitIDs []uuid.UUID
				for _, m := range members {
					if memberSide == "credit" {
						creditIDs = append(creditIDs, m.ID)
					} else {
						debitIDs = append(debitIDs, m.ID)
					}
				}
				if memberSide == "credit" {
					debitIDs = append(debitIDs, t.ID)
				} else {
					creditIDs = append(creditIDs, t.ID)
				}
				add(creditIDs, debitIDs)
			}
		}
	}
	for _, group := range creditGroups(credits) {
		emit(group, debits, "credit")
	}
	for _, group := range debitGroups(debits) {
		emit(group, credits, "debit")
	}
}
