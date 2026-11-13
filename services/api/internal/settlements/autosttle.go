package settlements

import (
	"context"
	"math"
	"strings"
	"time"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// TryAutoSettle runs INSIDE the approve transaction (q must be the tx-scoped
// querier). When an approved group's amount equals a pending settlement
// (±1%) with expected_date within ±7 days of the group, mark it settled.
// Returns the settled row (for audit) or nil.
func TryAutoSettle(ctx context.Context, q *db.Queries, orgID, groupID uuid.UUID) (*db.Settlement, error) {
	items, err := q.ListMatchGroupItems(ctx, []uuid.UUID{groupID})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.TransactionID)
	}
	txs, err := q.GetTransactionsByIDs(ctx, db.GetTransactionsByIDsParams{
		OrganizationID: orgID, Ids: ids,
	})
	if err != nil {
		return nil, err
	}
	var creditSum, debitSum int64
	minT, maxT := time.Time{}, time.Time{}
	for _, t := range txs {
		if t.Direction == "credit" {
			creditSum += t.AmountMinor
		} else {
			debitSum += t.AmountMinor
		}
		if minT.IsZero() || t.BookedAt.Before(minT) {
			minT = t.BookedAt
		}
		if maxT.IsZero() || t.BookedAt.After(maxT) {
			maxT = t.BookedAt
		}
	}

	pending, err := q.ListPendingSettlements(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var best *db.Settlement
	var bestGap float64
	for i := range pending {
		p := pending[i]
		if !withinBand(p.ExpectedAmountMinor, creditSum) && !withinBand(p.ExpectedAmountMinor, debitSum) {
			continue
		}
		// expected_date within ±7 days of the group's date span
		pad := 7 * 24 * time.Hour
		if p.ExpectedDate.Before(minT.Add(-pad)) || p.ExpectedDate.After(maxT.Add(pad)) {
			continue
		}
		gap := math.Abs(p.ExpectedDate.Sub(minT).Hours())
		if best == nil || gap < bestGap {
			cp := p
			best = &cp
			bestGap = gap
		}
	}
	if best == nil {
		return nil, nil
	}

	now := time.Now()
	settledAmount := best.ExpectedAmountMinor
	if err := q.UpdateSettlement(ctx, db.UpdateSettlementParams{
		OrganizationID:      orgID,
		ID:                  best.ID,
		AccountID:           best.AccountID,
		Counterparty:        best.Counterparty,
		Reference:           best.Reference,
		ExpectedAmountMinor: best.ExpectedAmountMinor,
		Currency:            best.Currency,
		ExpectedDate:        best.ExpectedDate,
		Status:              "settled",
		SettledAmountMinor:  settledAmount,
		SettledAt:           &now,
		Notes:               best.Notes,
		LinkedGroupID:       &groupID,
	}); err != nil {
		return nil, err
	}
	out := *best
	out.Status = "settled"
	out.SettledAmountMinor = settledAmount
	out.SettledAt = &now
	out.LinkedGroupID = &groupID
	return &out, nil
}

// withinBand: ±1% (100bp) integer check, no float money math.
func withinBand(expected, actual int64) bool {
	if expected <= 0 || actual <= 0 {
		return false
	}
	tol := actual / 100
	if tol < 1 {
		tol = 1
	}
	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}
	return diff <= tol
}

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func optUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
