package notifications

import (
	"context"
	"log/slog"

	db "github.com/cocok/cocok/services/api/internal/platform/db"
)

// OutboxMailer is the sandbox mailer: every "sent" email is stored in
// email_outbox (queryable by tests / dev) and logged. Swap for SMTP/SES in
// Phase 7 by implementing auth.Mailer.
type OutboxMailer struct {
	queries *db.Queries
	log     *slog.Logger
}

func NewOutboxMailer(q *db.Queries, log *slog.Logger) *OutboxMailer {
	return &OutboxMailer{queries: q, log: log}
}

func (m *OutboxMailer) Send(ctx context.Context, to, subject, body, kind string) error {
	err := m.queries.InsertEmailOutbox(ctx, db.InsertEmailOutboxParams{
		ToEmail: to, Subject: subject, Body: body, Kind: kind,
	})
	if err != nil {
		m.log.Error("email outbox write failed", "to", to, "error", err)
		return err
	}
	m.log.Info("email sent (sandbox outbox)", "to", to, "subject", subject, "kind", kind)
	return nil
}
