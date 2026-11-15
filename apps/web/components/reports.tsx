"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import {
  api,
  formatMinor,
  REPORT_KINDS,
  type OrgSummary,
  type ReportSummary,
} from "@/lib/api";

const API = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export function ReportsPanel({ org }: { org: OrgSummary }) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  const range = (from ? `from=${from}` : "") + (from && to ? "&" : "") + (to ? `to=${to}` : "");
  const rangeQS = range ? `?${range}` : "";

  const summary = useQuery({
    queryKey: ["report-summary", org.id, from, to],
    queryFn: () => api<ReportSummary>(`/organizations/${org.id}/reports/summary${rangeQS}`),
  });

  async function download(kind: string) {
    const res = await fetch(
      `${API}/api/v1/organizations/${org.id}/reports/export?kind=${kind}${range ? `&${range}` : ""}`,
      { credentials: "include" },
    );
    if (!res.ok) return;
    const blob = await res.blob();
    const cd = res.headers.get("Content-Disposition") ?? "";
    const m = /filename="([^"]+)"/.exec(cd);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = m?.[1] ?? `${kind}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const s = summary.data;
  const rate = s ? Math.round(s.matched.match_rate * 100) : 0;

  return (
    <section
      id="reports"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Reports &amp; analytics
      </div>
      <div className="flex flex-col gap-5 px-5 py-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
            From
            <input
              type="date"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
              className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
            To (exclusive)
            <input
              type="date"
              value={to}
              onChange={(e) => setTo(e.target.value)}
              className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
            />
          </label>
          {(from || to) && (
            <button
              type="button"
              onClick={() => {
                setFrom("");
                setTo("");
              }}
              className="h-9 rounded-md border border-[var(--color-line)] px-3 text-xs text-[var(--color-ink-muted)] hover:bg-[var(--color-canvas)]"
            >
              Clear
            </button>
          )}
        </div>

        {summary.isPending && (
          <div className="text-sm text-[var(--color-ink-muted)]">Loading summary…</div>
        )}
        {summary.isError && (
          <div className="text-sm text-[var(--color-alert)]">Failed to load summary.</div>
        )}

        {s && (
          <>
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
              <StatCard label="Transactions" value={String(s.transactions.total_count)} />
              <StatCard label="Match rate" value={`${rate}%`} />
              <StatCard label="Matched tx" value={String(s.matched.count)} />
              <StatCard
                label="Open exceptions"
                value={String(s.exceptions_by_status.open ?? 0)}
              />
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="rounded-md border border-[var(--color-line)] p-4">
                <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
                  Value flows
                </div>
                <div className="num mt-2 space-y-1 text-sm">
                  <div className="flex justify-between">
                    <span className="text-[var(--color-ink-muted)]">Credit in</span>
                    <span>{formatMinor(s.transactions.total_credit_minor, "IDR")}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-[var(--color-ink-muted)]">Debit out</span>
                    <span>{formatMinor(s.transactions.total_debit_minor, "IDR")}</span>
                  </div>
                  <div className="flex justify-between border-t border-[var(--color-line)] pt-1 font-medium">
                    <span className="text-[var(--color-ink-muted)]">Matched</span>
                    <span>
                      {formatMinor(s.matched.credit_minor + s.matched.debit_minor, "IDR")}
                    </span>
                  </div>
                </div>
              </div>

              <div className="rounded-md border border-[var(--color-line)] p-4">
                <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
                  Export CSV
                </div>
                <div className="mt-2 flex flex-wrap gap-2">
                  {REPORT_KINDS.map((k) => (
                    <button
                      key={k.kind}
                      type="button"
                      onClick={() => void download(k.kind)}
                      className="rounded-md border border-[var(--color-line)] px-3 py-1.5 text-xs hover:bg-[var(--color-canvas)]"
                    >
                      {k.label}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            <div className="grid gap-3 md:grid-cols-2">
              <div className="rounded-md border border-[var(--color-line)] p-4">
                <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
                  Value split
                </div>
                <div className="mt-2 h-48">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={[
                          { name: "Credit in", value: s.transactions.total_credit_minor },
                          { name: "Debit out", value: s.transactions.total_debit_minor },
                        ]}
                        dataKey="value"
                        nameKey="name"
                        innerRadius={50}
                        outerRadius={80}
                        label={({ name, value }) =>
                          `${name}: ${formatMinor(Number(value), "IDR")}`
                        }
                      >
                        <Cell fill="#0f766e" />
                        <Cell fill="#b45309" />
                      </Pie>
                      <Tooltip />
                      <Legend />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
              </div>

              <div className="rounded-md border border-[var(--color-line)] p-4">
                <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
                  Transactions per account (matched vs total)
                </div>
                <div className="mt-2 h-48">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={s.accounts.map((a) => ({ name: a.name, total: a.tx_count, matched: a.matched_count }))}>
                      <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
                      <XAxis dataKey="name" tick={{ fontSize: 11 }} />
                      <YAxis allowDecimals={false} tick={{ fontSize: 11 }} width={28} />
                      <Tooltip />
                      <Legend />
                      <Bar dataKey="total" fill="#94a3b8" name="Total" />
                      <Bar dataKey="matched" fill="#0f766e" name="Matched" />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
                    <th className="px-3 py-2.5 font-medium">Account</th>
                    <th className="px-3 py-2.5 text-right font-medium">Tx</th>
                    <th className="px-3 py-2.5 text-right font-medium">Matched</th>
                    <th className="px-3 py-2.5 text-right font-medium">Credit</th>
                    <th className="px-3 py-2.5 text-right font-medium">Debit</th>
                  </tr>
                </thead>
                <tbody>
                  {s.accounts.map((a) => (
                    <tr key={a.id} className="border-b border-[var(--color-line)] last:border-b-0">
                      <td className="px-3 py-3 font-medium">{a.name}</td>
                      <td className="num px-3 py-3 text-right">{a.tx_count}</td>
                      <td className="num px-3 py-3 text-right">
                        {a.matched_count}/{a.tx_count}
                      </td>
                      <td className="num px-3 py-3 text-right">
                        {formatMinor(a.credit_minor, a.currency)}
                      </td>
                      <td className="num px-3 py-3 text-right">
                        {formatMinor(a.debit_minor, a.currency)}
                      </td>
                    </tr>
                  ))}
                  {s.accounts.length === 0 && (
                    <tr>
                      <td colSpan={5} className="px-3 py-6 text-center text-[var(--color-ink-muted)]">
                        No accounts yet.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--color-line)] px-4 py-3">
      <div className="text-xs text-[var(--color-ink-faint)]">{label}</div>
      <div className="num mt-1 text-xl font-semibold">{value}</div>
    </div>
  );
}

