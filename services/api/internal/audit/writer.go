package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/cocok/cocok/services/api/internal/organizations"
	db "github.com/cocok/cocok/services/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Writer appends audit events. Never updates or deletes.
type Writer struct {
	queries *db.Queries
	log     *slog.Logger
}

func NewWriter(q *db.Queries, log *slog.Logger) *Writer {
	return &Writer{queries: q, log: log}
}

type Event struct {
	OrganizationID *uuid.UUID
	ActorID        uuid.UUID
	Action         string
	EntityType     string
	EntityID       *uuid.UUID
	PreviousState  map[string]any
	NewState       map[string]any
	Reason         string
	Metadata       map[string]any
}

// Write is fire-and-forgive: audit failure must never break the business op,
// but it is logged loudly. Financial decisions that must never be lost should
// write audit inside the business transaction (see Phase 3).
func (w *Writer) Write(ctx context.Context, e Event) {
	prev := toJSONB(e.PreviousState)
	next := toJSONB(e.NewState)
	meta := toJSONB(e.Metadata)
	if meta == nil {
		meta = []byte("{}")
	}
	err := w.queries.InsertAuditLog(ctx, db.InsertAuditLogParams{
		OrganizationID: e.OrganizationID,
		ActorID:        nullUUID(e.ActorID),
		Action:         e.Action,
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		PreviousState:  prev,
		NewState:       next,
		Reason:         pgtype.Text{String: e.Reason, Valid: e.Reason != ""},
		Metadata:       meta,
	})
	if err != nil {
		w.log.Error("audit write failed", "action", e.Action, "error", err)
	}
}

// WriteOrg satisfies organizations.AuditWriter.
func (w *Writer) WriteOrg(ctx context.Context, orgID *uuid.UUID, actorID uuid.UUID, action, entityType string, entityID *uuid.UUID, prev, next map[string]any, reason string) {
	w.Write(ctx, Event{
		OrganizationID: orgID, ActorID: actorID, Action: action,
		EntityType: entityType, EntityID: entityID,
		PreviousState: prev, NewState: next, Reason: reason,
	})
}

// OrgAdapter lets organizations.Service use this writer (avoids import cycle).
type OrgAdapter struct{ W *Writer }

func (a OrgAdapter) Write(ctx context.Context, e organizations.AuditEvent) {
	a.W.Write(ctx, Event{
		OrganizationID: e.OrganizationID,
		ActorID:        e.ActorID,
		Action:         e.Action,
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		PreviousState:  e.PreviousState,
		NewState:       e.NewState,
		Reason:         e.Reason,
		Metadata:       e.Metadata,
	})
}

func toJSONB(m map[string]any) []byte {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

func nullUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
