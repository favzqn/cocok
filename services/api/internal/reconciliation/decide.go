package reconciliation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/matching"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
)

// Group is the API view of a match group with its member transactions.
type Group struct {
	ID             string         `json:"id"`
	Status         string         `json:"status"`
	Kind           string         `json:"kind"`
	Score          *float64       `json:"score,omitempty"`
	WeightsVersion *int           `json:"weights_version,omitempty"`
	Components     map[string]any `json:"components,omitempty"`
	CreditTxIDs    []string       `json:"credit_tx_ids"`
	DebitTxIDs     []string       `json:"debit_tx_ids"`
	CreatedAt      interface{}    `json:"created_at"`
	DecidedAt      interface{}    `json:"decided_at,omitempty"`
}

// ManualMatch creates an approved group from explicitly chosen transactions.
// Invariant: sum(credits) == sum(debits) — matches never mutate source rows.
func (s *Service) ManualMatch(ctx context.Context, orgID, actorID uuid.UUID, txIDs []uuid.UUID) (*Group, error) {
	if len(txIDs) < 2 {
		return nil, fmt.Errorf("%w: at least two transactions required", platform.ErrValidation)
	}
	seen := map[uuid.UUID]bool{}
	uniq := make([]uuid.UUID, 0, len(txIDs))
	for _, id := range txIDs {
		if !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	rows, err := s.queries.GetTransactionsByIDs(ctx, db.GetTransactionsByIDsParams{
		OrganizationID: orgID, Ids: uniq,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) != len(uniq) {
		return nil, platform.ErrNotFound // cross-org or unknown id
	}
	var creditSum, debitSum int64
	var creditIDs, debitIDs []uuid.UUID
	for _, r := range rows {
		if r.Direction == "credit" {
			creditSum += r.AmountMinor
			creditIDs = append(creditIDs, r.ID)
		} else {
			debitSum += r.AmountMinor
			debitIDs = append(debitIDs, r.ID)
		}
	}
	if len(creditIDs) == 0 || len(debitIDs) == 0 {
		return nil, fmt.Errorf("%w: manual match needs at least one credit and one debit", platform.ErrValidation)
	}
	if creditSum != debitSum {
		return nil, fmt.Errorf("%w: credit sum %d != debit sum %d", ErrUnbalanced, creditSum, debitSum)
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
	n, err := qtx.GetActiveItemCountForTxs(ctx, uniq)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("%w: some transactions already matched", ErrConflict)
	}
	g, err := qtx.CreateMatchGroup(ctx, db.CreateMatchGroupParams{
		OrganizationID: orgID,
		Status:         "approved",
		Kind:           matching.Kind(len(debitIDs), len(creditIDs)),
		CreatedBy:      &actorID,
		DecidedAt:      timePtr(time.Now()),
	})
	if err != nil {
		return nil, err
	}
	if err := addItems(ctx, qtx, g.ID, creditIDs, "credit"); err != nil {
		return nil, err
	}
	if err := addItems(ctx, qtx, g.ID, debitIDs, "debit"); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "match.manual", EntityType: "match_group", EntityID: &g.ID,
		NewState: map[string]any{"kind": g.Kind, "credit_sum": creditSum, "tx_count": len(uniq)},
	})
	out := toGroup(g, creditIDs, debitIDs)
	return out, nil
}

// Decide approves or rejects a proposed group. Rejected groups free their
// transactions for the next run.
func (s *Service) Decide(ctx context.Context, orgID, actorID, groupID uuid.UUID, approve bool) error {
	want := "approved"
	if !approve {
		want = "rejected"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.queries.WithTx(tx)
	if err := lockOrg(ctx, tx, orgID); err != nil {
		return err
	}
	g, err := qtx.GetMatchGroup(ctx, db.GetMatchGroupParams{OrganizationID: orgID, ID: groupID})
	if err != nil {
		return platform.ErrNotFound
	}
	if g.Status != "proposed" {
		return ErrNoProposedGroup
	}
	if err := qtx.DecideMatchGroup(ctx, db.DecideMatchGroupParams{
		OrganizationID: orgID, ID: groupID, Status: want,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	action := "match.rejected"
	if approve {
		action = "match.approved"
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: action, EntityType: "match_group", EntityID: &groupID,
		PreviousState: map[string]any{"status": g.Status},
		NewState:      map[string]any{"status": want},
	})
	return nil
}

// Unmatch deletes a proposed/approved group; its transactions become free.
func (s *Service) Unmatch(ctx context.Context, orgID, actorID, groupID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := s.queries.WithTx(tx)
	if err := lockOrg(ctx, tx, orgID); err != nil {
		return err
	}
	g, err := qtx.GetMatchGroup(ctx, db.GetMatchGroupParams{OrganizationID: orgID, ID: groupID})
	if err != nil {
		return platform.ErrNotFound
	}
	if g.Status == "rejected" {
		return ErrNoProposedGroup
	}
	if err := qtx.DeleteMatchGroup(ctx, db.DeleteMatchGroupParams{OrganizationID: orgID, ID: groupID}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "match.unmatched", EntityType: "match_group", EntityID: &groupID,
		PreviousState: map[string]any{"status": g.Status, "kind": g.Kind},
	})
	return nil
}

func toGroup(g db.MatchGroup, creditIDs, debitIDs []uuid.UUID) *Group {
	out := &Group{
		ID:          g.ID.String(),
		Status:      g.Status,
		Kind:        g.Kind,
		CreditTxIDs: idStrings(creditIDs),
		DebitTxIDs:  idStrings(debitIDs),
		CreatedAt:   g.CreatedAt,
	}
	if g.Score.Valid {
		v := g.Score.Float64
		out.Score = &v
	}
	if g.WeightsVersion.Valid {
		v := int(g.WeightsVersion.Int32)
		out.WeightsVersion = &v
	}
	if len(g.Components) > 0 {
		_ = json.Unmarshal(g.Components, &out.Components)
	}
	if g.DecidedAt != nil {
		out.DecidedAt = *g.DecidedAt
	}
	return out
}

func idStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func timePtr(t time.Time) *time.Time { return &t }
