"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, canResolveExceptions, type Exception, type OrgSummary } from "@/lib/api";

const STATUSES = ["open", "resolved", "dismissed"] as const;

export function ExceptionsPanel({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const [status, setStatus] = useState("open");
  const canResolve = canResolveExceptions(org.role);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();

  const exceptions = useQuery({
    queryKey: ["exceptions", org.id, status],
    queryFn: () =>
      api<{ data: Exception[] }>(
        `/organizations/${org.id}/exceptions?status=${status}`,
      ),
  });

  const decide = useMutation({
    mutationFn: (vars: { id: string; action: "resolve" | "dismiss" }) =>
      api<{ ok: boolean }>(
        `/organizations/${org.id}/exceptions/${vars.id}/${vars.action}`,
        { method: "POST", body: JSON.stringify({}) },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["exceptions", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Failed" }),
  });

  return (
    <section
      id="exceptions"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Exceptions — investigate what didn&apos;t match
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        {msg && !msg.ok && <p className="text-sm text-[var(--color-alert)]">{msg.text}</p>}

        <div className="flex gap-2">
          {STATUSES.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setStatus(s)}
              className={`rounded-md border px-3 py-1 text-xs ${
                status === s
                  ? "border-[var(--color-navy-900)] bg-[var(--color-navy-900)] text-white"
                  : "border-[var(--color-line)] text-[var(--color-ink-muted)] hover:bg-[var(--color-canvas)]"
              }`}
            >
              {s}
            </button>
          ))}
        </div>

        {exceptions.isPending && (
          <div className="text-sm text-[var(--color-ink-muted)]">Loading…</div>
        )}
        {exceptions.isError && (
          <div className="text-sm text-[var(--color-alert)]">Failed to load exceptions.</div>
        )}
        {exceptions.isSuccess && exceptions.data.data.length === 0 && (
          <div className="text-sm text-[var(--color-ink-muted)]">No {status} exceptions.</div>
        )}
        {exceptions.isSuccess && exceptions.data.data.length > 0 && (
          <ul className="flex flex-col gap-2">
            {exceptions.data.data.map((e) => (
              <li
                key={e.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-[var(--color-line)] px-4 py-2.5"
              >
                <div className="min-w-0">
                  <span className="mr-2 rounded border border-[var(--color-line)] px-1.5 py-0.5 text-[10px] font-medium uppercase">
                    {e.type.replace("_", " ")}
                  </span>
                  <span className="num text-xs text-[var(--color-ink-faint)]">
                    {e.subject_tx_ids.length} tx · {e.subject_tx_ids[0]?.slice(0, 8)}
                    {e.subject_tx_ids.length > 1 ? "…" : ""}
                  </span>
                  {e.note && (
                    <span className="ml-2 text-xs italic text-[var(--color-ink-muted)]">
                      {e.note}
                    </span>
                  )}
                </div>
                {e.status === "open" && canResolve && (
                  <div className="flex shrink-0 gap-2">
                    <button
                      type="button"
                      disabled={decide.isPending}
                      onClick={() => decide.mutate({ id: e.id, action: "resolve" })}
                      className="rounded-md bg-[var(--color-navy-900)] px-3 py-1 text-xs font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
                    >
                      Resolve
                    </button>
                    <button
                      type="button"
                      disabled={decide.isPending}
                      onClick={() => decide.mutate({ id: e.id, action: "dismiss" })}
                      className="rounded-md border border-[var(--color-line)] px-3 py-1 text-xs text-[var(--color-ink)] hover:bg-[var(--color-canvas)] disabled:opacity-60"
                    >
                      Dismiss
                    </button>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
