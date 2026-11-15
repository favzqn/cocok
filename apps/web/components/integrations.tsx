"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  api,
  canManageIntegrations,
  type FinancialAccount,
  type Integration,
  type OrgSummary,
  type WebhookEvent,
} from "@/lib/api";

export function IntegrationsPanel({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const manage = canManageIntegrations(org.role);
  const [provider, setProvider] = useState("xendit");
  const [secret, setSecret] = useState("");
  const [accountId, setAccountId] = useState("");
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const [copied, setCopied] = useState<string>();

  const integrations = useQuery({
    queryKey: ["integrations", org.id],
    queryFn: () => api<{ data: Integration[] }>(`/organizations/${org.id}/integrations`),
  });
  const events = useQuery({
    queryKey: ["webhook-events", org.id],
    queryFn: () => api<{ data: WebhookEvent[] }>(`/organizations/${org.id}/webhook-events`),
  });
  const accounts = useQuery({
    queryKey: ["accounts", org.id],
    queryFn: () => api<{ data: FinancialAccount[] }>(`/organizations/${org.id}/accounts`),
    enabled: manage,
  });

  const create = useMutation({
    mutationFn: () =>
      api<Integration>(`/organizations/${org.id}/integrations`, {
        method: "POST",
        body: JSON.stringify({
          provider,
          webhook_secret: secret,
          default_account_id: accountId || undefined,
        }),
      }),
    onSuccess: () => {
      setSecret("");
      setMsg({
        ok: true,
        text: "Integration created — paste the webhook URL into the provider dashboard.",
      });
      qc.invalidateQueries({ queryKey: ["integrations", org.id] });
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Create failed" }),
  });

  const disable = useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean }>(`/organizations/${org.id}/integrations/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["integrations", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Disable failed" }),
  });

  const valid = secret.trim().length >= 8;

  return (
    <section
      id="integrations"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Integrations — payment providers (sandbox)
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        {msg && (
          <p
            className={`text-sm ${msg.ok ? "text-[var(--color-ink-muted)]" : "text-[var(--color-alert)]"}`}
          >
            {msg.text}
          </p>
        )}

        {integrations.isPending && (
          <div className="text-sm text-[var(--color-ink-muted)]">Loading…</div>
        )}

        {integrations.data && integrations.data.data.length > 0 && (
          <ul className="flex flex-col gap-3">
            {integrations.data.data.map((it) => (
              <li
                key={it.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-[var(--color-line)] px-4 py-3"
              >
                <div className="min-w-0">
                  <div className="text-sm font-medium capitalize">
                    {it.provider}{" "}
                    <span className="ml-1 rounded border border-[var(--color-line)] px-1.5 py-0.5 text-[10px] uppercase">
                      {it.mode}
                    </span>
                  </div>
                  <code className="num block max-w-lg truncate text-xs text-[var(--color-ink-faint)]">
                    {it.webhook_url}
                  </code>
                </div>
                <div className="flex shrink-0 gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      void navigator.clipboard.writeText(it.webhook_url);
                      setCopied(it.id);
                      setTimeout(() => setCopied(undefined), 1500);
                    }}
                    className="rounded-md border border-[var(--color-line)] px-3 py-1.5 text-xs hover:bg-[var(--color-canvas)]"
                  >
                    {copied === it.id ? "Copied!" : "Copy URL"}
                  </button>
                  {manage && it.status === "active" && (
                    <button
                      type="button"
                      disabled={disable.isPending}
                      onClick={() => disable.mutate(it.id)}
                      className="rounded-md border border-[var(--color-line)] px-3 py-1.5 text-xs text-[var(--color-alert)] hover:bg-[var(--color-canvas)] disabled:opacity-60"
                    >
                      Disable
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}

        <div className="border-t border-[var(--color-line)] pt-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
            Recent webhook deliveries
          </div>
          {events.isPending && (
            <div className="mt-2 text-sm text-[var(--color-ink-muted)]">Loading…</div>
          )}
          {events.isSuccess && events.data.data.length === 0 && (
            <div className="mt-2 text-sm text-[var(--color-ink-muted)]">
              No webhooks received yet — paste the URL into the provider sandbox dashboard.
            </div>
          )}
          {events.isSuccess && events.data.data.length > 0 && (
            <ul className="mt-2">
              {events.data.data.slice(0, 8).map((ev) => (
                <li key={ev.id} className="num flex flex-wrap justify-between gap-2 py-1 text-xs">
                  <span className="text-[var(--color-ink-muted)]">
                    {new Date(ev.created_at).toLocaleString("id-ID")} · {ev.provider} ·{" "}
                    {ev.event_type || "—"}
                  </span>
                  <span className="truncate">
                    {ev.event_key}
                    <span
                      className={`ml-2 rounded border px-1.5 py-0.5 uppercase ${
                        ev.status === "processed"
                          ? "border-emerald-600/40 text-emerald-700"
                          : ev.status === "ignored"
                            ? "border-amber-600/40 text-amber-700"
                            : "border-red-600/40 text-red-700"
                      }`}
                    >
                      {ev.status}
                    </span>
                    {ev.reason && (
                      <span className="ml-2 text-[var(--color-ink-faint)]">{ev.reason}</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        {manage && (
          <form
            className="flex flex-wrap items-end gap-3 border-t border-[var(--color-line)] pt-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (valid) create.mutate();
            }}
          >
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Provider
              <select
                value={provider}
                onChange={(e) => setProvider(e.target.value)}
                className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
              >
                <option value="xendit">Xendit</option>
                <option value="midtrans">Midtrans</option>
              </select>
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Webhook secret (callback token / server key)
              <input
                type="password"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                placeholder="min 8 chars"
                className="h-9 w-56 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Default account (incoming payments)
              <select
                value={accountId}
                onChange={(e) => setAccountId(e.target.value)}
                className="h-9 w-52 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
              >
                <option value="">— none —</option>
                {(accounts.data?.data ?? []).map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="submit"
              disabled={!valid || create.isPending}
              className="inline-flex h-9 items-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
            >
              {create.isPending ? "Creating…" : "Connect"}
            </button>
          </form>
        )}
      </div>
    </section>
  );
}

