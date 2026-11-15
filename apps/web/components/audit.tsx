"use client";

import { useQuery } from "@tanstack/react-query";
import { api, canViewAudit, type AuditLog, type OrgSummary } from "@/lib/api";

export function AuditPanel({ org }: { org: OrgSummary }) {
  const canView = canViewAudit(org.role);

  const logs = useQuery({
    queryKey: ["audit-logs", org.id],
    queryFn: () => api<{ data: AuditLog[] }>(`/organizations/${org.id}/audit-logs`),
    enabled: canView,
  });

  if (!canView) {
    return (
      <section
        id="audit-logs"
        className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] px-5 py-6 text-sm text-[var(--color-ink-muted)]"
      >
        Audit logs are available to finance and admin roles.
      </section>
    );
  }

  return (
    <section
      id="audit-logs"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Audit logs — append-only trail (DB-enforced)
      </div>
      {logs.isPending && (
        <div className="px-5 py-6 text-sm text-[var(--color-ink-muted)]">Loading…</div>
      )}
      {logs.isError && (
        <div className="px-5 py-6 text-sm text-[var(--color-alert)]">Failed to load audit logs.</div>
      )}
      {logs.isSuccess && logs.data.data.length === 0 && (
        <div className="px-5 py-8 text-sm text-[var(--color-ink-muted)]">No events yet.</div>
      )}
      {logs.isSuccess && logs.data.data.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
                <th className="px-5 py-2.5 font-medium">Time</th>
                <th className="px-5 py-2.5 font-medium">Action</th>
                <th className="px-5 py-2.5 font-medium">Entity</th>
                <th className="px-5 py-2.5 font-medium">Detail</th>
              </tr>
            </thead>
            <tbody>
              {logs.data.data.map((e) => (
                <tr key={e.id} className="border-b border-[var(--color-line)] last:border-b-0">
                  <td className="num whitespace-nowrap px-5 py-2.5 text-xs text-[var(--color-ink-muted)]">
                    {new Date(e.created_at).toLocaleString("id-ID")}
                  </td>
                  <td className="px-5 py-2.5 font-medium">{e.action}</td>
                  <td className="px-5 py-2.5 text-xs">
                    {e.entity_type}
                    {e.entity_id ? ` · ${e.entity_id.slice(0, 8)}` : ""}
                  </td>
                  <td className="num max-w-72 truncate px-5 py-2.5 text-xs text-[var(--color-ink-faint)]">
                    {e.reason ??
                      (e.new_state ? JSON.stringify(e.new_state).slice(0, 80) : "—")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
