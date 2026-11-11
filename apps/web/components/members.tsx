"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  api,
  canManage,
  roleLabel,
  ROLES,
  type Invite,
  type Member,
  type OrgSummary,
} from "@/lib/api";

export function MembersPanel({ org }: { org: OrgSummary }) {
  const manage = canManage(org.role);

  return (
    <section className="rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]">
      <div className="flex items-baseline justify-between border-b border-[var(--color-line)] px-5 py-4">
        <span className="text-sm font-medium">Members — {org.name}</span>
        <span className="num text-xs text-[var(--color-ink-faint)]">
          your role: {roleLabel(org.role)}
        </span>
      </div>
      <MemberList org={org} manage={manage} />
      {manage && <InviteForm org={org} />}
    </section>
  );
}

function MemberList({ org, manage }: { org: OrgSummary; manage: boolean }) {
  const qc = useQueryClient();
  const [rowError, setRowError] = useState<string>();

  const members = useQuery({
    queryKey: ["members", org.id],
    queryFn: () => api<{ data: Member[] }>(`/organizations/${org.id}/members`),
  });

  const changeRole = useMutation({
    mutationFn: (vars: { userId: string; role: string }) =>
      api<{ ok: boolean }>(`/organizations/${org.id}/members/role`, {
        method: "PATCH",
        body: JSON.stringify({ user_id: vars.userId, role: vars.role }),
      }),
    onSuccess: () => {
      setRowError(undefined);
      qc.invalidateQueries({ queryKey: ["members", org.id] });
    },
    onError: (err) =>
      setRowError(err instanceof Error ? err.message : "Failed to change role"),
  });

  if (members.isPending) {
    return (
      <div className="px-5 py-6 text-sm text-[var(--color-ink-muted)]">
        Loading members…
      </div>
    );
  }
  if (members.isError) {
    return (
      <div className="px-5 py-6 text-sm text-[var(--color-alert)]">
        Failed to load members.
      </div>
    );
  }

  const items = members.data.data;
  return (
    <div>
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
            <th className="px-5 py-2.5 font-medium">Name</th>
            <th className="px-5 py-2.5 font-medium">Email</th>
            <th className="px-5 py-2.5 font-medium">Role</th>
            <th className="px-5 py-2.5 text-right font-medium">Status</th>
          </tr>
        </thead>
        <tbody>
          {items.map((m) => (
            <tr key={m.id} className="border-b border-[var(--color-line)] last:border-b-0">
              <td className="px-5 py-3 font-medium">{m.full_name}</td>
              <td className="px-5 py-3 text-[var(--color-ink-muted)]">{m.email}</td>
              <td className="px-5 py-3">
                {manage ? (
                  <select
                    value={m.role}
                    aria-label={`Role for ${m.full_name}`}
                    onChange={(e) =>
                      changeRole.mutate({ userId: m.user_id, role: e.target.value })
                    }
                    disabled={changeRole.isPending}
                    className="h-8 rounded border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm outline-none focus:border-[var(--color-navy-900)]"
                  >
                    {ROLES.map((r) => (
                      <option key={r} value={r}>
                        {roleLabel(r)}
                      </option>
                    ))}
                  </select>
                ) : (
                  <span>{roleLabel(m.role)}</span>
                )}
              </td>
              <td className="px-5 py-3 text-right text-xs uppercase text-[var(--color-ink-muted)]">
                {m.status}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rowError && (
        <div className="border-t border-[var(--color-line)] px-5 py-3 text-sm text-[var(--color-alert)]">
          {rowError}
        </div>
      )}
    </div>
  );
}

function InviteForm({ org }: { org: OrgSummary }) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<string>("viewer");
  const [invite, setInvite] = useState<Invite>();
  const [copied, setCopied] = useState(false);

  const createInvite = useMutation({
    mutationFn: () =>
      api<Invite>(`/organizations/${org.id}/invitations`, {
        method: "POST",
        body: JSON.stringify({ email: email.trim(), role }),
      }),
    onSuccess: (inv) => {
      setInvite(inv);
      setEmail("");
      setCopied(false);
    },
  });

  const origin = typeof window !== "undefined" ? window.location.origin : "";
  const link = invite ? `${origin}/invite?token=${invite.invite_token}` : undefined;

  return (
    <div className="border-t border-[var(--color-line)] px-5 py-4">
      <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
        Invite teammate
      </div>
      <form
        className="mt-2.5 flex flex-wrap items-center gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (email.trim()) createInvite.mutate();
        }}
      >
        <input
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="colleague@company.id"
          aria-label="Invite email"
          className="h-9 w-64 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)]"
        />
        <select
          value={role}
          onChange={(e) => setRole(e.target.value)}
          aria-label="Invite role"
          className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm outline-none focus:border-[var(--color-navy-900)]"
        >
          {ROLES.map((r) => (
            <option key={r} value={r}>
              {roleLabel(r)}
            </option>
          ))}
        </select>
        <button
          type="submit"
          disabled={createInvite.isPending || !email.trim()}
          className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
        >
          {createInvite.isPending ? "Creating…" : "Create invite link"}
        </button>
      </form>

      {createInvite.isError && (
        <p className="mt-2 text-sm text-[var(--color-alert)]">
          {createInvite.error instanceof Error
            ? createInvite.error.message
            : "Failed to create invite"}
        </p>
      )}

      {link && (
        <div className="mt-3 rounded-md border border-[var(--color-line)] bg-[var(--color-canvas)] px-3 py-2.5">
          <p className="text-xs text-[var(--color-ink-muted)]">
            Invite link for{" "}
            <span className="font-medium text-[var(--color-ink)]">{invite?.email}</span> —
            shown once, expires{" "}
            {invite ? new Date(invite.expires_at).toLocaleDateString() : ""}.
            Recipient must sign in with this email first.
          </p>
          <div className="mt-2 flex items-center gap-3">
            <code className="num min-w-0 flex-1 truncate text-xs text-[var(--color-ink)]">
              {link}
            </code>
            <button
              type="button"
              onClick={() => {
                navigator.clipboard.writeText(link);
                setCopied(true);
              }}
              className="shrink-0 text-xs font-medium underline underline-offset-2"
            >
              {copied ? "Copied" : "Copy"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}