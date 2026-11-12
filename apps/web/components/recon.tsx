"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  api,
  canApproveMatches,
  canRunRecon,
  type MatchGroup,
  type OrgSummary,
} from "@/lib/api";

const STATUSES = ["proposed", "approved", "rejected"] as const;

export function ReconPanel({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const [status, setStatus] = useState<string>("proposed");
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const canRun = canRunRecon(org.role);
  const canApprove = canApproveMatches(org.role);

  const groups = useQuery({
    queryKey: ["recon-groups", org.id, status],
    queryFn: () =>
      api<{ data: MatchGroup[] }>(
        `/organizations/${org.id}/reconciliation/groups?status=${status}`,
      ),
  });

  const run = useMutation({
    mutationFn: () =>
      api<{ candidates: number; proposed_groups: number }>(
        `/organizations/${org.id}/reconciliation/runs`,
        { method: "POST" },
      ),
    onSuccess: (out) => {
      setMsg({
        ok: true,
        text: `Run complete: ${out.candidates} candidates scored, ${out.proposed_groups} proposed.`,
      });
      qc.invalidateQueries({ queryKey: ["recon-groups", org.id] });
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Run failed" }),
  });

  const decide = useMutation({
    mutationFn: (vars: { id: string; action: "approve" | "reject" }) =>
      api<{ ok: boolean }>(
        `/organizations/${org.id}/reconciliation/groups/${vars.id}/${vars.action}`,
        { method: "POST" },
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["recon-groups", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Decision failed" }),
  });

  const unmatch = useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean }>(`/organizations/${org.id}/reconciliation/groups/${id}/unmatch`, {
        method: "POST",
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["recon-groups", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Unmatch failed" }),
  });

  return (
    <section
      id="reconciliation"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="flex items-center justify-between border-b border-[var(--color-line)] px-5 py-4">
        <span className="text-sm font-medium">Reconciliation</span>
        {canRun && (
          <button
            type="button"
            disabled={run.isPending}
            onClick={() => run.mutate()}
            className="inline-flex h-8 items-center rounded-md bg-[var(--color-navy-900)] px-3 text-xs font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
          >
            {run.isPending ? "Running…" : "Run matching"}
          </button>
        )}
      </div>

      <div className="flex flex-col gap-4 px-5 py-4">
        {msg && (
          <p
            className={`text-sm ${msg.ok ? "text-[var(--color-ink-muted)]" : "text-[var(--color-alert)]"}`}
          >
            {msg.text}
          </p>
        )}

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


        {groups.isPending && (
          <div className="text-sm text-[var(--color-ink-muted)]">Loading groups…</div>
        )}
        {groups.isError && (
          <div className="text-sm text-[var(--color-alert)]">Failed to load groups.</div>
        )}
        {groups.isSuccess && groups.data.data.length === 0 && (
          <div className="text-sm text-[var(--color-ink-muted)]">
            No {status} groups.
            {canRun && status === "proposed" ? " Run matching to generate proposals." : ""}
          </div>
        )}
        {groups.isSuccess && groups.data.data.length > 0 && (
          <ul className="flex flex-col gap-3">
            {groups.data.data.map((g) => (
              <li
                key={g.id}
                className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-[var(--color-line)] px-4 py-3"
              >
                <div className="min-w-0">
                  <div className="text-sm font-medium">
                    {g.kind}
                    {g.score !== undefined && (
                      <span className="num ml-2 text-xs text-[var(--color-ink-faint)]">
                        score {g.score.toFixed(3)}
                      </span>
                    )}
                  </div>
                  <div className="num truncate text-xs text-[var(--color-ink-faint)]">
                    {g.credit_tx_ids.length} credit / {g.debit_tx_ids.length} debit ·{" "}
                    {g.credit_tx_ids[0]?.slice(0, 8)}
                    {g.credit_tx_ids.length > 1 ? "…" : ""} ↔ {g.debit_tx_ids[0]?.slice(0, 8)}
                    {g.debit_tx_ids.length > 1 ? "…" : ""}
                  </div>
                </div>
                <div className="flex shrink-0 gap-2">
                  {g.status === "proposed" && canApprove && (
                    <>
                      <button
                        type="button"
                        disabled={decide.isPending}
                        onClick={() => decide.mutate({ id: g.id, action: "approve" })}
                        className="rounded-md bg-[var(--color-navy-900)] px-3 py-1.5 text-xs font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
                      >
                        Approve
                      </button>
                      <button
                        type="button"
                        disabled={decide.isPending}
                        onClick={() => decide.mutate({ id: g.id, action: "reject" })}
                        className="rounded-md border border-[var(--color-line)] px-3 py-1.5 text-xs text-[var(--color-ink)] hover:bg-[var(--color-canvas)] disabled:opacity-60"
                      >
                        Reject
                      </button>
                    </>
                  )}
                  {(g.status === "proposed" || g.status === "approved") && canApprove && (
                    <button
                      type="button"
                      disabled={unmatch.isPending}
                      onClick={() => unmatch.mutate(g.id)}
                      className="rounded-md border border-[var(--color-line)] px-3 py-1.5 text-xs text-[var(--color-alert)] hover:bg-[var(--color-canvas)] disabled:opacity-60"
                    >
                      Unmatch
                    </button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
