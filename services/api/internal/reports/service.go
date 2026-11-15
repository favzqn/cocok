package reports

import (
	"context"
	"time"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
)

type Service struct {
	queries *db.Queries
}

func NewService(q *db.Queries) *Service { return &Service{queries: q} }

type Summary struct {
	Transactions struct {
		TotalCount  int   `json:"total_count"`
		TotalCredit int64 `json:"total_credit_minor"`
		TotalDebit  int64 `json:"total_debit_minor"`
	} `json:"transactions"`
	Matched struct {
		Count       int     `json:"count"`
		CreditCount int     `json:"credit_count"`
		DebitCount  int     `json:"debit_count"`
		CreditTotal int64   `json:"credit_minor"`
		DebitTotal  int64   `json:"debit_minor"`
		Rate        float64 `json:"match_rate"` // matched tx / total tx
	} `json:"matched"`
	Groups      map[string]int  `json:"groups_by_status"`
	Exceptions  map[string]int  `json:"exceptions_by_status"`
	Settlements map[string]any  `json:"settlements_by_status"`
	Accounts    []AccountRollup `json:"accounts"`
}

type AccountRollup struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Currency     string `json:"currency"`
	TxCount      int    `json:"tx_count"`
	CreditTotal  int64  `json:"credit_minor"`
	DebitTotal   int64  `json:"debit_minor"`
	MatchedCount int    `json:"matched_count"`
}

// Summary aggregates the org's operational picture in one round-trip per table.
func (s *Service) Summary(ctx context.Context, orgID uuid.UUID, from, to *time.Time) (*Summary, error) {
	out := &Summary{
		Groups:      map[string]int{},
		Exceptions:  map[string]int{},
		Settlements: map[string]any{},
	}

	txs, err := s.queries.TxSummaryByDirection(ctx, db.TxSummaryByDirectionParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, r := range txs {
		out.Transactions.TotalCount += int(r.N)
		if r.Direction == "credit" {
			out.Transactions.TotalCredit += r.Total
		} else {
			out.Transactions.TotalDebit += r.Total
		}
	}

	matched, err := s.queries.MatchedSummaryByDirection(ctx, db.MatchedSummaryByDirectionParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, r := range matched {
		out.Matched.Count += int(r.N)
		if r.Direction == "credit" {
			out.Matched.CreditCount = int(r.N)
			out.Matched.CreditTotal = r.Total
		} else {
			out.Matched.DebitCount = int(r.N)
			out.Matched.DebitTotal = r.Total
		}
	}
	if out.Transactions.TotalCount > 0 {
		out.Matched.Rate = float64(out.Matched.Count) / float64(out.Transactions.TotalCount)
	}

	groups, err := s.queries.GroupSummaryByStatus(ctx, db.GroupSummaryByStatusParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, r := range groups {
		out.Groups[r.Status] = int(r.N)
	}
	exc, err := s.queries.ExceptionSummaryByStatus(ctx, db.ExceptionSummaryByStatusParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, r := range exc {
		out.Exceptions[r.Status] = int(r.N)
	}
	setl, err := s.queries.SettlementSummaryByStatus(ctx, db.SettlementSummaryByStatusParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, r := range setl {
		out.Settlements[r.Status] = map[string]any{
			"count": int(r.N), "expected_minor": r.ExpectedTotal, "settled_minor": r.SettledTotal,
		}
	}

	accts, err := s.queries.AccountRollup(ctx, db.AccountRollupParams{OrganizationID: orgID, FromT: from, ToT: to})
	if err != nil {
		return nil, err
	}
	for _, a := range accts {
		out.Accounts = append(out.Accounts, AccountRollup{
			ID: a.ID.String(), Name: a.Name, Currency: a.Currency,
			TxCount: int(a.TxCount), CreditTotal: a.CreditTotal,
			DebitTotal: a.DebitTotal, MatchedCount: int(a.MatchedCount),
		})
	}
	if out.Accounts == nil {
		out.Accounts = []AccountRollup{}
	}
	return out, nil
}
