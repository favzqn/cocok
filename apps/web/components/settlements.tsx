"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  api,
  canRunRecon,
  formatMinor,
  SETTLEMENT_STATUSES,
  type OrgSummary,
  type Settlement,
} from "@/lib/api";

export function SettlementsPanel({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const canRun = canRunRecon(org.role);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const [form, setForm] = useState({
    counterparty: "",
    reference: "",
    amount: "",
    date: "",
    notes: "",
  });

  const settlements = useQuery({
    queryKey: ["settlements", org.id],
    queryFn: () => api<{ data: Settlement[] }>(`/organizations/${org.id}/settlements`),
  });

  const create = useMutation({
    mutationFn: () =>
      api<Settlement>(`/organizations/${org.id}/settlements`, {
        method: "POST",
        body: JSON.stringify({
          counterparty: form.counterparty.trim() || undefined,
          reference: form.reference.trim() || undefined,
          expected_amount_minor: Math.round(Number(form.amount)),
          expected_date: form.date,
          notes: form.notes.trim() || undefined,
        }),
      }),
    onSuccess: () => {
      setForm({ counterparty: "", reference: "", amount: "", date: "", notes: "" });
      setMsg({ ok: true, text: "Settlement created." });
      qc.invalidateQueries({ queryKey: ["settlements", org.id] });
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Create failed" }),
  });

  const patch = useMutation({
    mutationFn: (vars: { id: string; status: string }) =>
      api<Settlement>(`/organizations/${org.id}/settlements/${vars.id}`, {
        method: "PATCH",
        body: JSON.stringify({ status: vars.status }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settlements", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Update failed" }),
  });

  const remove = useMutation({
    mutationFn: (id: string) =>
      api<{ ok: boolean }>(`/organizations/${org.id}/settlements/${id}`, {
        method: "DELETE",
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["settlements", org.id] }),
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Delete failed" }),
  });

  const valid =
    form.date !== "" && Number.isFinite(Number(form.amount)) && Number(form.amount) > 0;

  return (
    <section
      id="settlements"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Settlements — expected cash movements
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        {msg && (
          <p
            className={`text-sm ${msg.ok ? "text-[var(--color-ink-muted)]" : "text-[var(--color-alert)]"}`}
          >
            {msg.text}
          </p>
        )}

        {canRun && (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (valid) create.mutate();
            }}
          >
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Counterparty
              <input
                value={form.counterparty}
                onChange={(e) => setForm((f) => ({ ...f, counterparty: e.target.value }))}
                placeholder="Shopee"
                className="h-9 w-40 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Reference
              <input
                value={form.reference}
                onChange={(e) => setForm((f) => ({ ...f, reference: e.target.value }))}
                placeholder="PAY-123"
                className="h-9 w-36 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Amount (major units)
              <input
                type="number"
                min="1"
                required
                value={form.amount}
                onChange={(e) => setForm((f) => ({ ...f, amount: e.target.value }))}
                placeholder="1500000"
                className="h-9 w-36 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Expected date
              <input
                type="date"
                required
                value={form.date}
                onChange={(e) => setForm((f) => ({ ...f, date: e.target.value }))}
                className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <button
              type="submit"
              disabled={!valid || create.isPending}
              className="inline-flex h-9 items-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
            >
              {create.isPending ? "Adding…" : "Add settlement"}
            </button>
          </form>
        )}

        {settlements.isPending && (
          <div className="text-sm text-[var(--color-ink-muted)]">Loading…</div>
        )}
        {settlements.isError && (
          <div className="text-sm text-[var(--color-alert)]">Failed to load settlements.</div>
        )}
        {settlements.isSuccess && settlements.data.data.length === 0 && (
          <div className="text-sm text-[var(--color-ink-muted)]">No settlements yet.</div>
        )}
        {settlements.isSuccess && settlements.data.data.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
                  <th className="px-3 py-2.5 font-medium">Expected</th>
                  <th className="px-3 py-2.5 font-medium">Date</th>
                  <th className="px-3 py-2.5 font-medium">Counterparty</th>
                  <th className="px-3 py-2.5 text-right font-medium">Amount</th>
                  <th className="px-3 py-2.5 font-medium">Status</th>
                  <th className="px-3 py-2.5 text-right font-medium">Actions</th>
                </tr>
              </thead>
              <tbody>
                {settlements.data.data.map((s) => (
                  <tr key={s.id} className="border-b border-[var(--color-line)] last:border-b-0">
                    <td className="num px-3 py-3 text-xs text-[var(--color-ink-faint)]">
                      {s.reference ?? "—"}
                    </td>
                    <td className="num px-3 py-3">
                      {new Date(s.expected_date).toLocaleDateString("id-ID")}
                    </td>
                    <td className="px-3 py-3">{s.counterparty ?? "—"}</td>
                    <td className="num px-3 py-3 text-right">
                      {formatMinor(s.expected_amount_minor, s.currency)}
                    </td>
                    <td className="px-3 py-3">
                      <span
                        className={`rounded border px-2 py-0.5 text-xs ${
                          s.status === "settled"
                            ? "border-emerald-600/40 text-emerald-700"
                            : s.status === "failed"
                              ? "border-red-600/40 text-red-700"
                              : s.status === "partial"
                                ? "border-amber-600/40 text-amber-700"
                                : "border-[var(--color-line)] text-[var(--color-ink-muted)]"
                        }`}
                      >
                        {s.status}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-right">
                      {canRun && (
                        <span className="inline-flex gap-1.5">
                          {SETTLEMENT_STATUSES.filter((st) => st !== s.status)
                            .slice(0, 2)
                            .map((st) => (
                              <button
                                key={st}
                                type="button"
                                disabled={patch.isPending}
                                onClick={() => patch.mutate({ id: s.id, status: st })}
                                className="rounded border border-[var(--color-line)] px-2 py-0.5 text-xs hover:bg-[var(--color-canvas)] disabled:opacity-60"
                              >
                                {st}
                              </button>
                            ))}
                          <button
                            type="button"
                            disabled={remove.isPending}
                            onClick={() => remove.mutate(s.id)}
                            className="rounded border border-[var(--color-line)] px-2 py-0.5 text-xs text-[var(--color-alert)] hover:bg-[var(--color-canvas)] disabled:opacity-60"
                          >
                            delete
                          </button>
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}

