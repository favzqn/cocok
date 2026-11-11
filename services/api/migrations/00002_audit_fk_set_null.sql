-- +goose Up
ALTER TABLE audit_logs
  DROP CONSTRAINT audit_logs_organization_id_fkey,
  ADD CONSTRAINT audit_logs_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE SET NULL,
  DROP CONSTRAINT audit_logs_actor_id_fkey,
  ADD CONSTRAINT audit_logs_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE audit_logs
  DROP CONSTRAINT audit_logs_organization_id_fkey,
  ADD CONSTRAINT audit_logs_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organizations(id),
  DROP CONSTRAINT audit_logs_actor_id_fkey,
  ADD CONSTRAINT audit_logs_actor_id_fkey
    FOREIGN KEY (actor_id) REFERENCES users(id);
