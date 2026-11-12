package reconciliation

import (
	"context"
	"encoding/json"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListGroups returns groups with member tx ids; status "" = all.
func (s *Service) ListGroups(ctx context.Context, orgID uuid.UUID, status string) ([]Group, error) {
	var st pgtype.Text
	if status != "" {
		st = pgtype.Text{String: status, Valid: true}
	}
	rows, err := s.queries.ListMatchGroups(ctx, db.ListMatchGroupsParams{
		OrganizationID: orgID, Status: st,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []Group{}, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, g := range rows {
		ids = append(ids, g.ID)
	}
	items, err := s.queries.ListMatchGroupItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	byGroup := map[uuid.UUID]*Group{}
	out := make([]Group, 0, len(rows))
	for _, g := range rows {
		out = append(out, *toGroup(g, nil, nil))
		byGroup[g.ID] = &out[len(out)-1]
	}
	for _, it := range items {
		g, ok := byGroup[it.MatchGroupID]
		if !ok {
			continue
		}
		if it.Side == "credit" {
			g.CreditTxIDs = append(g.CreditTxIDs, it.TransactionID.String())
		} else {
			g.DebitTxIDs = append(g.DebitTxIDs, it.TransactionID.String())
		}
	}
	return out, nil
}

// Candidate is the persisted scoring output of the latest run.
type Candidate struct {
	ID             string         `json:"id"`
	CreditTxIDs    []string       `json:"credit_tx_ids"`
	DebitTxIDs     []string       `json:"debit_tx_ids"`
	Score          float64        `json:"score"`
	Components     map[string]any `json:"components"`
	WeightsVersion int            `json:"weights_version"`
	CreatedAt      interface{}    `json:"created_at"`
}

func (s *Service) ListCandidates(ctx context.Context, orgID uuid.UUID) ([]Candidate, error) {
	rows, err := s.queries.ListMatchCandidates(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(rows))
	for _, c := range rows {
		cand := Candidate{
			ID:             c.ID.String(),
			CreditTxIDs:    idStrings(c.CreditIds),
			DebitTxIDs:     idStrings(c.DebitIds),
			Score:          c.Score,
			WeightsVersion: int(c.WeightsVersion),
			CreatedAt:      c.CreatedAt,
		}
		_ = json.Unmarshal(c.Components, &cand.Components)
		out = append(out, cand)
	}
	return out, nil
}
