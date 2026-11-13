package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
)

// Export renders one of the supported reports as CSV bytes.
func (s *Service) Export(ctx context.Context, orgID uuid.UUID, kind string) (filename string, data []byte, err error) {
	switch kind {
	case "transactions":
		return s.exportTransactions(ctx, orgID)
	case "matches":
		return s.exportMatches(ctx, orgID)
	case "exceptions":
		return s.exportExceptions(ctx, orgID)
	case "settlements":
		return s.exportSettlements(ctx, orgID)
	default:
		return "", nil, fmt.Errorf("%w: unknown report kind", errUnknownKind)
	}
}

// ErrUnknownKind is returned for unsupported ?kind= values.
var ErrUnknownKind = fmt.Errorf("unknown report kind")

var errUnknownKind = ErrUnknownKind

func csvFile(base string, write func(w *csv.Writer) error) (string, []byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := write(w); err != nil {
		return "", nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", nil, err
	}
	name := fmt.Sprintf("%s_%s.csv", base, time.Now().UTC().Format("20060102_150405"))
	return name, buf.Bytes(), nil
}

func (s *Service) exportTransactions(ctx context.Context, orgID uuid.UUID) (string, []byte, error) {
	rows, err := s.queries.ListTransactionsExport(ctx, orgID)
	if err != nil {
		return "", nil, err
	}
	return csvFile("transactions", func(w *csv.Writer) error {
		if err := w.Write([]string{
			"id", "booked_at", "direction", "amount_minor", "currency",
			"description", "counterparty", "external_ref", "matched",
		}); err != nil {
			return err
		}
		for _, r := range rows {
			if err := w.Write([]string{
				r.ID.String(),
				r.BookedAt.UTC().Format(time.RFC3339),
				r.Direction,
				strconv.FormatInt(r.AmountMinor, 10),
				r.Currency,
				r.Description,
				r.Counterparty.String,
				r.ExternalRef.String,
				strconv.FormatBool(r.Matched),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) exportMatches(ctx context.Context, orgID uuid.UUID) (string, []byte, error) {
	groups, err := s.queries.ListMatchGroups(ctx, db.ListMatchGroupsParams{OrganizationID: orgID})
	if err != nil {
		return "", nil, err
	}
	ids := make([]uuid.UUID, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.ID)
	}
	items, err := s.queries.ListMatchGroupItems(ctx, ids)
	if err != nil {
		return "", nil, err
	}
	byGroup := map[uuid.UUID][2]string{} // credit ids | debit ids (comma joined)
	join := func(cur, add string) string {
		if cur == "" {
			return add
		}
		return cur + ";" + add
	}
	for _, it := range items {
		v := byGroup[it.MatchGroupID]
		if it.Side == "credit" {
			v[0] = join(v[0], it.TransactionID.String())
		} else {
			v[1] = join(v[1], it.TransactionID.String())
		}
		byGroup[it.MatchGroupID] = v
	}
	return csvFile("matches", func(w *csv.Writer) error {
		if err := w.Write([]string{
			"id", "status", "kind", "score", "weights_version",
			"credit_tx_ids", "debit_tx_ids", "created_at", "decided_at",
		}); err != nil {
			return err
		}
		for _, g := range groups {
			score := ""
			if g.Score.Valid {
				score = strconv.FormatFloat(g.Score.Float64, 'f', 4, 64)
			}
			ver := ""
			if g.WeightsVersion.Valid {
				ver = strconv.Itoa(int(g.WeightsVersion.Int32))
			}
			decided := ""
			if g.DecidedAt != nil {
				decided = g.DecidedAt.UTC().Format(time.RFC3339)
			}
			m := byGroup[g.ID]
			if err := w.Write([]string{
				g.ID.String(), g.Status, g.Kind, score, ver,
				m[0], m[1],
				g.CreatedAt.UTC().Format(time.RFC3339), decided,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) exportExceptions(ctx context.Context, orgID uuid.UUID) (string, []byte, error) {
	rows, err := s.queries.ListExceptions(ctx, db.ListExceptionsParams{OrganizationID: orgID})
	if err != nil {
		return "", nil, err
	}
	return csvFile("exceptions", func(w *csv.Writer) error {
		if err := w.Write([]string{
			"id", "type", "status", "subject_tx_ids", "note", "detected_at", "resolved_at",
		}); err != nil {
			return err
		}
		for _, e := range rows {
			ids := ""
			for i, id := range e.SubjectTxIds {
				if i > 0 {
					ids += ";"
				}
				ids += id.String()
			}
			resolved := ""
			if e.ResolvedAt != nil {
				resolved = e.ResolvedAt.UTC().Format(time.RFC3339)
			}
			if err := w.Write([]string{
				e.ID.String(), e.Type, e.Status, ids, e.Note.String,
				e.DetectedAt.Format(time.RFC3339), resolved,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) exportSettlements(ctx context.Context, orgID uuid.UUID) (string, []byte, error) {
	rows, err := s.queries.ListSettlements(ctx, db.ListSettlementsParams{OrganizationID: orgID})
	if err != nil {
		return "", nil, err
	}
	return csvFile("settlements", func(w *csv.Writer) error {
		if err := w.Write([]string{
			"id", "counterparty", "reference", "expected_amount_minor", "currency",
			"expected_date", "status", "settled_amount_minor", "settled_at",
			"linked_group_id", "notes",
		}); err != nil {
			return err
		}
		for _, r := range rows {
			settledAt := ""
			if r.SettledAt != nil {
				settledAt = r.SettledAt.UTC().Format(time.RFC3339)
			}
			linked := ""
			if r.LinkedGroupID != nil {
				linked = r.LinkedGroupID.String()
			}
			if err := w.Write([]string{
				r.ID.String(), r.Counterparty.String, r.Reference.String,
				strconv.FormatInt(r.ExpectedAmountMinor, 10), r.Currency,
				r.ExpectedDate.Format("2006-01-02"), r.Status,
				strconv.FormatInt(r.SettledAmountMinor, 10), settledAt, linked, r.Notes.String,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}
