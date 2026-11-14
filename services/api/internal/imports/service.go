package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxFileBytes   = 10 << 20 // 10 MiB raw CSV stored inline
	sampleSize     = 20
	maxErrPerBatch = 100
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

// Preview is the response after upload / before commit.
type Preview struct {
	BatchID      string      `json:"batch_id"`
	AccountID    string      `json:"account_id"`
	Filename     string      `json:"filename"`
	Headers      []string    `json:"headers"`
	Mapping      Mapping     `json:"mapping"`
	RowCount     int         `json:"row_count"`
	ValidCount   int         `json:"valid_count"`
	InvalidCount int         `json:"invalid_count"`
	Errors       []RowError  `json:"errors"`
	Sample       []SampleRow `json:"sample"`
}

// SampleRow is a preview-safe row (no raw dump).
type SampleRow struct {
	Row          int    `json:"row"`
	Date         string `json:"date"`
	Direction    string `json:"direction"`
	AmountMinor  int64  `json:"amount_minor"`
	Description  string `json:"description,omitempty"`
	Counterparty string `json:"counterparty,omitempty"`
	ExternalRef  string `json:"external_ref,omitempty"`
}

// UploadInput carries one CSV import attempt.
type UploadInput struct {
	AccountID uuid.UUID
	Filename  string
	Raw       []byte
	Mapping   Mapping
}

// Upload validates + stores the raw file and returns a preview. Nothing is
// written to transactions until Commit.
func (s *Service) Upload(ctx context.Context, orgID, actorID uuid.UUID, in UploadInput) (*Preview, error) {
	if len(in.Raw) == 0 {
		return nil, fmt.Errorf("%w: file is empty", platform.ErrValidation)
	}
	if len(in.Raw) > maxFileBytes {
		return nil, fmt.Errorf("%w: file exceeds 10MB", platform.ErrValidation)
	}
	acc, err := s.queries.GetFinancialAccount(ctx, db.GetFinancialAccountParams{
		OrganizationID: orgID, ID: in.AccountID,
	})
	if err != nil {
		return nil, platform.ErrNotFound
	}

	loc, err := loadLocation()
	if err != nil {
		return nil, err
	}
	parsable, err := loadParsable(in.Filename, in.Raw)
	if err != nil {
		return nil, err
	}
	m := in.Mapping
	if strings.TrimSpace(m.Date) == "" || strings.TrimSpace(m.Amount) == "" {
		// Server-side column guess (xlsx clients cannot read headers themselves).
		header, herr := PeekHeaders(in.Filename, in.Raw)
		if herr != nil {
			return nil, herr
		}
		guess := GuessMapping(header)
		if m.Date == "" {
			m.Date = guess.Date
		}
		if m.Amount == "" {
			m.Amount = guess.Amount
		}
		if m.Description == "" {
			m.Description = guess.Description
		}
		if m.Counterparty == "" {
			m.Counterparty = guess.Counterparty
		}
		if m.Direction == "" {
			m.Direction = guess.Direction
		}
		if m.Reference == "" {
			m.Reference = guess.Reference
		}
		if m.Date == "" || m.Amount == "" {
			return nil, fmt.Errorf("%w: cannot guess date/amount columns from header %v — send mapping", platform.ErrValidation, header)
		}
	}
	rows, rowErrs, header, err := ParseCSV(parsable, m, acc.Currency, loc)
	if err != nil {
		return nil, err
	}

	mapJSON, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	batch, err := s.queries.CreateImportBatch(ctx, db.CreateImportBatchParams{
		OrganizationID: orgID,
		AccountID:      in.AccountID,
		Filename:       filename(in.Filename),
		Mapping:        mapJSON,
		RawFile:        in.Raw,
		CreatedBy:      &actorID,
	})
	if err != nil {
		return nil, err
	}
	if err := s.queries.UpdateImportBatchPreview(ctx, db.UpdateImportBatchPreviewParams{
		OrganizationID: orgID,
		ID:             batch.ID,
		Mapping:        mapJSON,
		RowCount:       int32(len(rows) + len(rowErrs)),
		ValidCount:     int32(len(rows)),
		InvalidCount:   int32(len(rowErrs)),
	}); err != nil {
		return nil, err
	}

	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "import.uploaded", EntityType: "import_batch", EntityID: &batch.ID,
		NewState: map[string]any{
			"filename": batch.Filename, "rows": len(rows) + len(rowErrs),
			"valid": len(rows), "invalid": len(rowErrs),
		},
	})
	return buildPreview(batch.ID, in.AccountID, batch.Filename, header, m, rows, rowErrs), nil
}

// Commit re-parses the stored raw file inside a transaction and inserts
// canonical transactions with ON CONFLICT dedup.
func (s *Service) Commit(ctx context.Context, orgID, actorID, batchID uuid.UUID) (map[string]any, error) {
	batch, err := s.queries.GetImportBatch(ctx, db.GetImportBatchParams{
		OrganizationID: orgID, ID: batchID,
	})
	if err != nil {
		return nil, platform.ErrNotFound
	}
	if batch.Status == "committed" {
		return nil, fmt.Errorf("%w: batch already committed", platform.ErrValidation)
	}
	acc, err := s.queries.GetFinancialAccount(ctx, db.GetFinancialAccountParams{
		OrganizationID: orgID, ID: batch.AccountID,
	})
	if err != nil {
		return nil, platform.ErrNotFound
	}
	var m Mapping
	if err := json.Unmarshal(batch.Mapping, &m); err != nil {
		return nil, fmt.Errorf("%w: corrupt mapping", platform.ErrValidation)
	}
	loc, err := loadLocation()
	if err != nil {
		return nil, err
	}
	parsable, err := loadParsable(batch.Filename, batch.RawFile)
	if err != nil {
		return nil, err
	}
	rows, rowErrs, _, err := ParseCSV(parsable, m, acc.Currency, loc)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: no valid rows to commit", platform.ErrValidation)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.queries.WithTx(tx)

	var inserted, dupes int64
	bID := batch.ID
	for _, r := range rows {
		raw, _ := json.Marshal(r.Raw)
		n, err := qtx.InsertTransaction(ctx, db.InsertTransactionParams{
			OrganizationID: orgID,
			AccountID:      batch.AccountID,
			ImportBatchID:  &bID,
			BookedAt:       r.BookedAt,
			Direction:      r.Direction,
			AmountMinor:    r.AmountMinor,
			Currency:       acc.Currency,
			Description:    r.Description,
			Counterparty:   toText(r.Counterparty),
			ExternalRef:    toText(r.ExternalRef),
			Fingerprint:    r.Fingerprint,
			SourceRow:      int32(r.SourceRow),
			RawRow:         raw,
		})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			dupes++
		} else {
			inserted++
		}
	}
	if err := qtx.MarkImportBatchCommitted(ctx, db.MarkImportBatchCommittedParams{
		OrganizationID: orgID,
		ID:             batch.ID,
		InsertedCount:  int32(inserted),
		DuplicateCount: int32(dupes),
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "import.committed", EntityType: "import_batch", EntityID: &batch.ID,
		NewState: map[string]any{
			"filename": batch.Filename, "inserted": inserted,
			"duplicates": dupes, "invalid": len(rowErrs),
		},
	})
	return map[string]any{
		"batch_id":   batch.ID.String(),
		"inserted":   inserted,
		"duplicates": dupes,
		"invalid":    len(rowErrs),
	}, nil
}

// ListBatches returns recent import batches (newest first).
func (s *Service) ListBatches(ctx context.Context, orgID uuid.UUID) ([]db.ImportBatch, error) {
	return s.queries.ListImportBatches(ctx, orgID)
}

// ListTransactions returns canonical transactions for the org, optional account filter.
func (s *Service) ListTransactions(ctx context.Context, orgID, accountID uuid.UUID, limit int32) ([]db.Transaction, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var filter *uuid.UUID
	if accountID != uuid.Nil {
		filter = &accountID
	}
	return s.queries.ListTransactions(ctx, db.ListTransactionsParams{
		OrganizationID: orgID,
		AccountID:      filter,
		Limit:          pgtype.Int4{Int32: limit, Valid: true},
	})
}

func buildPreview(batchID, accountID uuid.UUID, fname string, header []string, m Mapping, rows []ParsedRow, errs []RowError) *Preview {
	sample := make([]SampleRow, 0, sampleSize)
	for _, r := range SampleRows(rows, sampleSize) {
		sample = append(sample, SampleRow{
			Row:          r.SourceRow,
			Date:         r.BookedAt.Format("2006-01-02"),
			Direction:    r.Direction,
			AmountMinor:  r.AmountMinor,
			Description:  r.Description,
			Counterparty: r.Counterparty,
			ExternalRef:  r.ExternalRef,
		})
	}
	if errs == nil {
		errs = []RowError{}
	}
	if len(errs) > maxErrPerBatch {
		errs = errs[:maxErrPerBatch]
	}
	return &Preview{
		BatchID:      batchID.String(),
		AccountID:    accountID.String(),
		Filename:     fname,
		Headers:      header,
		Mapping:      m,
		RowCount:     len(rows) + len(errs),
		ValidCount:   len(rows),
		InvalidCount: len(errs),
		Errors:       errs,
		Sample:       sample,
	}
}

func loadLocation() (*time.Location, error) {
	// Durable fact: default tenant timezone (PRODUCT.md). Org-level override
	// lands with org settings.
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return nil, fmt.Errorf("load Asia/Jakarta: %w", err)
	}
	return loc, nil
}

func filename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "import.csv"
	}
	if len(s) > 255 {
		s = s[:255]
	}
	return s
}

func nullUUID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
