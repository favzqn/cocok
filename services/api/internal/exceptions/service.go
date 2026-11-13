package exceptions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cocok/cocok/services/api/internal/audit"
	"github.com/cocok/cocok/services/api/internal/platform"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// nearMissFloor: candidates below min_score but at least this interesting
// end up as near_miss exceptions instead of being dropped.
const nearMissFloor = 0.3

var (
	ErrAlreadyOpen = errors.New("exception already open")
	ErrNotOpen     = errors.New("exception is not open")
)

type Service struct {
	queries *db.Queries
	pool    *pgxpool.Pool
	audit   *audit.Writer
}

func NewService(q *db.Queries, pool *pgxpool.Pool, a *audit.Writer) *Service {
	return &Service{queries: q, pool: pool, audit: a}
}

// Miss is a below-threshold candidate handed over by the matcher.
type Miss struct {
	TxIDs      []uuid.UUID
	Score      float64
	Components map[string]float64
}

// GenerateInTx rebuilds the open unmatched/near_miss queues for an org.
// Runs inside the reconciliation run transaction. Dismissed fingerprints are
// never re-opened; resolved ones may reappear while still a problem.
func (s *Service) GenerateInTx(ctx context.Context, q *db.Queries, orgID, actorID uuid.UUID, unmatched []uuid.UUID, misses []Miss) error {
	if err := q.DeleteOpenGenerated(ctx, db.DeleteOpenGeneratedParams{
		OrganizationID: orgID,
		Column2:        []string{"unmatched", "near_miss"},
	}); err != nil {
		return err
	}
	skip := map[string]bool{}
	for _, typ := range []string{"unmatched", "near_miss"} {
		fps, err := q.GetDismissedExceptionFps(ctx, db.GetDismissedExceptionFpsParams{
			OrganizationID: orgID, Type: typ,
		})
		if err != nil {
			return err
		}
		for _, fp := range fps {
			skip[fp] = true
		}
	}

	insert := func(typ string, ids []uuid.UUID, detail map[string]any) error {
		fp := Fingerprint(typ, ids)
		if skip[fp] {
			return nil
		}
		d, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		_, err = q.InsertException(ctx, db.InsertExceptionParams{
			OrganizationID: orgID,
			Type:           typ,
			SubjectTxIds:   ids,
			Fingerprint:    fp,
			Detail:         d,
			CreatedBy:      &actorID,
		})
		return err
	}

	for _, id := range unmatched {
		if err := insert("unmatched", []uuid.UUID{id}, map[string]any{
			"reason": "not matched by latest run",
		}); err != nil {
			return err
		}
	}
	for _, m := range misses {
		if err := insert("near_miss", m.TxIDs, map[string]any{
			"score": m.Score, "components": m.Components,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Floor exposes the near-miss interest floor to the matcher.
func Floor() float64 { return nearMissFloor }

// Fingerprint dedupes open exceptions per type.
func Fingerprint(typ string, ids []uuid.UUID) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = id.String()
	}
	sort.Strings(s)
	h := sha256.Sum256([]byte(typ + "|" + strings.Join(s, ",")))
	return hex.EncodeToString(h[:])
}

// Exception is the API view.
type Exception struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Status       string         `json:"status"`
	SubjectTxIDs []string       `json:"subject_tx_ids"`
	Detail       map[string]any `json:"detail"`
	Note         *string        `json:"note,omitempty"`
	DetectedAt   interface{}    `json:"detected_at"`
	ResolvedAt   interface{}    `json:"resolved_at,omitempty"`
}

func toException(e db.Exception) Exception {
	out := Exception{
		ID:           e.ID.String(),
		Type:         e.Type,
		Status:       e.Status,
		SubjectTxIDs: idStrings(e.SubjectTxIds),
		DetectedAt:   e.DetectedAt,
	}
	_ = json.Unmarshal(e.Detail, &out.Detail)
	if e.Note.Valid {
		n := e.Note.String
		out.Note = &n
	}
	if e.ResolvedAt != nil {
		out.ResolvedAt = *e.ResolvedAt
	}
	return out
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID, status, typ string) ([]Exception, error) {
	rows, err := s.queries.ListExceptions(ctx, db.ListExceptionsParams{
		OrganizationID: orgID,
		Status:         optText(status),
		Type:           optText(typ),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Exception, 0, len(rows))
	for _, e := range rows {
		out = append(out, toException(e))
	}
	return out, nil
}

// Flag lets a user raise a manual exception on chosen transactions.
func (s *Service) Flag(ctx context.Context, orgID, actorID uuid.UUID, txIDs []uuid.UUID, note string) (*Exception, error) {
	if len(txIDs) == 0 {
		return nil, fmt.Errorf("%w: transaction_ids required", platform.ErrValidation)
	}
	rows, err := s.queries.GetTransactionsByIDs(ctx, db.GetTransactionsByIDsParams{
		OrganizationID: orgID, Ids: txIDs,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) != len(txIDs) {
		return nil, platform.ErrNotFound
	}
	d, _ := json.Marshal(map[string]any{"flagged_by": actorID.String()})
	n, err := s.queries.InsertException(ctx, db.InsertExceptionParams{
		OrganizationID: orgID,
		Type:           "manual",
		SubjectTxIds:   txIDs,
		Fingerprint:    Fingerprint("manual", txIDs),
		Detail:         d,
		Note:           optText(note),
		CreatedBy:      &actorID,
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrAlreadyOpen
	}
	list, err := s.queries.ListExceptions(ctx, db.ListExceptionsParams{
		OrganizationID: orgID,
		Status:         pgtype.Text{String: "open", Valid: true},
		Type:           pgtype.Text{String: "manual", Valid: true},
	})
	if err != nil {
		return nil, err
	}
	want := Fingerprint("manual", txIDs)
	for _, e := range list {
		if e.Fingerprint == want {
			out := toException(e)
			return &out, nil
		}
	}
	return nil, ErrAlreadyOpen
}

// Decide resolves or dismisses an open exception.
func (s *Service) Decide(ctx context.Context, orgID, actorID, id uuid.UUID, status, note string) error {
	if status != "resolved" && status != "dismissed" {
		return fmt.Errorf("%w: status must be resolved or dismissed", platform.ErrValidation)
	}
	n, err := s.queries.DecideException(ctx, db.DecideExceptionParams{
		OrganizationID: orgID,
		ID:             id,
		Status:         status,
		ResolvedBy:     &actorID,
		Note:           optText(note),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		if _, gerr := s.queries.GetException(ctx, db.GetExceptionParams{OrganizationID: orgID, ID: id}); gerr != nil {
			return platform.ErrNotFound
		}
		return ErrNotOpen
	}
	s.audit.Write(ctx, audit.Event{
		OrganizationID: &orgID, ActorID: actorID,
		Action: "exception." + status, EntityType: "exception", EntityID: &id,
		NewState: map[string]any{"status": status, "note": note},
	})
	return nil
}

func optText(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	return pgtype.Text{String: s, Valid: s != ""}
}

func idStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}
