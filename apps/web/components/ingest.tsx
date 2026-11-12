"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import {
  api,
  canImport,
  canManageAccounts,
  formatMinor,
  type CanonicalTransaction,
  type FinancialAccount,
  type ImportBatch,
  type ImportPreview,
  type OrgSummary,
} from "@/lib/api";

const MAPPING_FIELDS = [
  { key: "date", label: "Date", required: true },
  { key: "amount", label: "Amount", required: true },
  { key: "description", label: "Description", required: false },
  { key: "counterparty", label: "Counterparty", required: false },
  { key: "direction", label: "Direction (debit/credit)", required: false },
  { key: "reference", label: "Reference", required: false },
] as const;

const API = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

// Best-effort column guess so the common case needs no manual mapping.
function guessMapping(cols: string[]): Record<string, string> {
  const find = (...patterns: RegExp[]) =>
    cols.find((c) => patterns.some((p) => p.test(c.toLowerCase()))) ?? "";
  return {
    date: find(/tanggal/, /^date$/, /tgl/, /booking/, /\bdate\b/),
    amount: find(/nominal/, /amount/, /nilai/, /jumlah/, /\bamt\b/, /debit.*kredit/, /amount/),
    description: find(/keterangan/, /description/, /deskripsi/, /narrative/, /memo/),
    counterparty: find(/counterparty/, /nama/, /payer/, /payee/, /merchant/, /partner/),
    direction: find(/^jenis$/, /direction/, /\bcr.?dr\b/, /debit.*credit/, /tipe/),
    reference: find(/^ref/, /referensi/, /reference/, /trx/, /no.?ref/),
  };
}


export function IngestPanel({ org }: { org: OrgSummary }) {
  return (
    <div className="flex flex-col gap-8">
      <AccountsSection org={org} />
      <ImportSection org={org} />
      <TransactionsSection org={org} />
    </div>
  );
}

function AccountsSection({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const manage = canManageAccounts(org.role);
  const [name, setName] = useState("");
  const [kind, setKind] = useState("bank");

  const accounts = useQuery({
    queryKey: ["accounts", org.id],
    queryFn: () => api<{ data: FinancialAccount[] }>(`/organizations/${org.id}/accounts`),
  });

  const create = useMutation({
    mutationFn: () =>
      api<FinancialAccount>(`/organizations/${org.id}/accounts`, {
        method: "POST",
        body: JSON.stringify({ name: name.trim(), kind }),
      }),
    onSuccess: () => {
      setName("");
      qc.invalidateQueries({ queryKey: ["accounts", org.id] });
    },
  });

  return (
    <section
      id="accounts"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Financial Accounts
      </div>
      {accounts.isPending && (
        <div className="px-5 py-6 text-sm text-[var(--color-ink-muted)]">Loading accounts…</div>
      )}
      {accounts.isError && (
        <div className="px-5 py-6 text-sm text-[var(--color-alert)]">Failed to load accounts.</div>
      )}
      {accounts.isSuccess && (
        <>
          {accounts.data.data.length === 0 ? (
            <div className="px-5 py-8 text-sm text-[var(--color-ink-muted)]">
              No accounts yet. Create one to start importing statements.
            </div>
          ) : (
            <ul>
              {accounts.data.data.map((a) => (
                <li
                  key={a.id}
                  className="flex items-center justify-between border-b border-[var(--color-line)] px-5 py-3 last:border-b-0"
                >
                  <div>
                    <div className="text-sm font-medium">{a.name}</div>
                    <div className="num text-xs text-[var(--color-ink-faint)]">
                      {a.kind.replace(/_/g, " ")}
                      {a.account_number ? ` · ${a.account_number}` : ""} · {a.currency}
                    </div>
                  </div>
                  <span className="rounded border border-[var(--color-line)] px-2 py-0.5 text-xs">
                    {a.status}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {manage && (
            <form
              className="flex flex-wrap items-center gap-3 border-t border-[var(--color-line)] px-5 py-4"
              onSubmit={(e) => {
                e.preventDefault();
                if (name.trim()) create.mutate();
              }}
            >
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Account name (e.g. BCA Operasional)"
                aria-label="Account name"
                className="h-9 w-72 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)]"
              />
              <select
                value={kind}
                onChange={(e) => setKind(e.target.value)}
                aria-label="Account kind"
                className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm"
              >
                <option value="bank">Bank</option>
                <option value="payment_gateway">Payment gateway</option>
                <option value="e_wallet">E-wallet</option>
                <option value="marketplace">Marketplace</option>
                <option value="other">Other</option>
              </select>
              <button
                type="submit"
                disabled={create.isPending || !name.trim()}
                className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
              >
                {create.isPending ? "Creating…" : "Add account"}
              </button>
              {create.isError && (
                <span className="text-sm text-[var(--color-alert)]">
                  {create.error instanceof Error ? create.error.message : "Failed"}
                </span>
              )}
            </form>
          )}
        </>
      )}
    </section>
  );
}

function ImportSection({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const allowed = canImport(org.role);
  const [accountId, setAccountId] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [headers, setHeaders] = useState<string[]>([]);
  const [mapping, setMapping] = useState<Record<string, string>>({});
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();
  const fileRef = useRef<HTMLInputElement>(null);

  // Read header client-side so mapping can be chosen before first upload.
  async function onFileChange(f: File | null) {
    setFile(f);
    setPreview(null);
    setMsg(undefined);
    if (!f) {
      setHeaders([]);
      setMapping({});
      return;
    }
    const text = await f.text();
    const firstLine = text.split(/\r?\n/, 1)[0] ?? "";
    const cols = firstLine.split(",").map((s) => s.trim().replace(/^"|"$/g, ""));
    setHeaders(cols);
    setMapping(guessMapping(cols));
  }

  const accounts = useQuery({
    queryKey: ["accounts", org.id],
    queryFn: () => api<{ data: FinancialAccount[] }>(`/organizations/${org.id}/accounts`),
    enabled: allowed,
  });

  const batches = useQuery({
    queryKey: ["imports", org.id],
    queryFn: () => api<{ data: ImportBatch[] }>(`/organizations/${org.id}/imports`),
    enabled: allowed,
  });

  const upload = useMutation({
    mutationFn: async () => {
      // Re-derive mapping if stale (e.g. file chosen before code reload).
      let m = mapping;
      if (!m.date || !m.amount) {
        const text = await file!.text();
        const cols = (text.split(/\r?\n/, 1)[0] ?? "")
          .split(",")
          .map((s) => s.trim().replace(/^"|"$/g, ""));
        m = { ...guessMapping(cols), ...m };
      }
      if (!m.date || !m.amount) {
        throw new Error("Map Date and Amount columns before uploading.");
      }
      const fd = new FormData();
      fd.append("file", file!);
      fd.append("account_id", accountId);
      fd.append("mapping", JSON.stringify(m));
      const res = await fetch(`${API}/api/v1/organizations/${org.id}/imports`, {
        method: "POST",
        body: fd,
        credentials: "include",
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body?.error?.message ?? `Upload failed (${res.status})`);
      }
      return (await res.json()) as ImportPreview;
    },
    onSuccess: (prev) => {
      setMsg(undefined);
      setPreview(prev);
      setHeaders(prev.headers);
      setMapping((m) => ({ ...m, ...prev.mapping }));
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Upload failed" }),
  });

  const commit = useMutation({
    mutationFn: () =>
      api<{ inserted: number; duplicates: number; invalid: number }>(
        `/organizations/${org.id}/imports/${preview!.batch_id}/commit`,
        { method: "POST" },
      ),
    onSuccess: (out) => {
      setPreview(null);
      setFile(null);
      if (fileRef.current) fileRef.current.value = "";
      qc.invalidateQueries({ queryKey: ["imports", org.id] });
      qc.invalidateQueries({ queryKey: ["transactions", org.id] });
      setMsg({
        ok: true,
        text: `Committed: ${out.inserted} inserted, ${out.duplicates} duplicates skipped, ${out.invalid} invalid.`,
      });
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Commit failed" }),
  });

  if (!allowed) return null;

  const items = accounts.data?.data ?? [];
  const selected = items.find((a) => a.id === accountId);

  return (
    <section className="rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]">
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Import CSV statement
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        <div className="flex flex-wrap items-end gap-3">
          <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
            Account
            <select
              value={accountId}
              onChange={(e) => setAccountId(e.target.value)}
              className="h-9 w-56 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
            >
              <option value="">Select account…</option>
              {items.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} ({a.currency})
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
            CSV file
            <input
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              onChange={(e) => void onFileChange(e.target.files?.[0] ?? null)}
              className="text-sm text-[var(--color-ink)]"
            />
          </label>
          <button
            type="button"
            disabled={!accountId || !file || upload.isPending}
            onClick={() => upload.mutate()}
            className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
          >
            {upload.isPending ? "Uploading…" : "Upload & preview"}
          </button>
        </div>

        {!accountId && (
          <p className="text-xs text-[var(--color-ink-faint)]">
            Select an account first.
          </p>
        )}
        {file && (!mapping.date || !mapping.amount) && (
          <p className="text-xs text-[var(--color-alert)]">
            Map the Date and Amount columns below before uploading.
          </p>
        )}

        {msg && (
          <p
            className={`text-sm ${msg.ok ? "text-[var(--color-ink-muted)]" : "text-[var(--color-alert)]"}`}
          >
            {msg.text}
          </p>
        )}

        {headers.length > 0 && !preview && (
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
            {MAPPING_FIELDS.map((f) => (
              <label
                key={f.key}
                className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]"
              >
                {f.label}
                <select
                  value={mapping[f.key] ?? ""}
                  onChange={(e) => setMapping((m) => ({ ...m, [f.key]: e.target.value }))}
                  className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
                >
                  <option value="">— none —</option>
                  {headers.map((h) => (
                    <option key={h} value={h}>
                      {h}
                    </option>
                  ))}
                </select>
              </label>
            ))}
          </div>
        )}

        {preview && (
          <div className="flex flex-col gap-4">
            <div className="num text-xs text-[var(--color-ink-faint)]">
              {preview.filename} · {preview.row_count} rows · {preview.valid_count} valid ·{" "}
              {preview.invalid_count} invalid
            </div>

            <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
              {MAPPING_FIELDS.map((f) => (
                <label
                  key={f.key}
                  className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]"
                >
                  {f.label}
                  <select
                    value={mapping[f.key] ?? ""}
                    onChange={(e) => setMapping((m) => ({ ...m, [f.key]: e.target.value }))}
                    className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
                  >
                    <option value="">— none —</option>
                    {headers.map((h) => (
                      <option key={h} value={h}>
                        {h}
                      </option>
                    ))}
                  </select>
                </label>
              ))}
            </div>

            {preview.sample.length > 0 && (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
                    <th className="py-2 pr-3 font-medium">Date</th>
                    <th className="py-2 pr-3 font-medium">Dir</th>
                    <th className="py-2 pr-3 text-right font-medium">Amount</th>
                    <th className="py-2 font-medium">Description</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.sample.map((r) => (
                    <tr key={r.row} className="border-b border-[var(--color-line)] last:border-b-0">
                      <td className="num py-2 pr-3">{r.date}</td>
                      <td className="py-2 pr-3">{r.direction}</td>
                      <td className="num py-2 pr-3 text-right">
                        {formatMinor(r.amount_minor, selected?.currency ?? "IDR")}
                      </td>
                      <td className="truncate py-2">{r.description}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}

            {preview.errors.length > 0 && (
              <ul className="max-h-40 overflow-y-auto text-xs text-[var(--color-alert)]">
                {preview.errors.map((e) => (
                  <li key={`${e.row}-${e.field}-${e.message}`}>
                    Row {e.row}
                    {e.field ? ` (${e.field})` : ""}: {e.message}
                  </li>
                ))}
              </ul>
            )}

            <div>
              <button
                type="button"
                disabled={commit.isPending || preview.valid_count === 0}
                onClick={() => commit.mutate()}
                className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
              >
                {commit.isPending ? "Committing…" : `Commit ${preview.valid_count} rows`}
              </button>
            </div>
          </div>
        )}

        {batches.data && batches.data.data.length > 0 && (
          <div className="border-t border-[var(--color-line)] pt-3">
            <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-ink-faint)]">
              Recent imports
            </div>
            <ul className="mt-2">
              {batches.data.data.slice(0, 5).map((b) => (
                <li key={b.id} className="num flex justify-between py-1 text-xs">
                  <span className="truncate">{b.filename}</span>
                  <span className="shrink-0 pl-3 text-[var(--color-ink-faint)]">
                    {b.status} · {b.inserted_count} in · {b.duplicate_count} dup
                  </span>
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </section>
  );
}


function TransactionsSection({ org }: { org: OrgSummary }) {
  const txs = useQuery({
    queryKey: ["transactions", org.id],
    queryFn: () =>
      api<{ data: CanonicalTransaction[] }>(`/organizations/${org.id}/transactions?limit=20`),
  });

  return (
    <section
      id="transactions"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Canonical transactions
      </div>
      {txs.isPending && (
        <div className="px-5 py-6 text-sm text-[var(--color-ink-muted)]">Loading…</div>
      )}
      {txs.isError && (
        <div className="px-5 py-6 text-sm text-[var(--color-alert)]">
          Failed to load transactions.
        </div>
      )}
      {txs.isSuccess &&
        (txs.data.data.length === 0 ? (
          <div className="px-5 py-8 text-sm text-[var(--color-ink-muted)]">
            Nothing imported yet.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--color-line)] text-left text-xs uppercase tracking-wide text-[var(--color-ink-faint)]">
                  <th className="px-5 py-2.5 font-medium">Booked</th>
                  <th className="px-5 py-2.5 font-medium">Dir</th>
                  <th className="px-5 py-2.5 text-right font-medium">Amount</th>
                  <th className="px-5 py-2.5 font-medium">Description</th>
                  <th className="px-5 py-2.5 font-medium">Ref</th>
                </tr>
              </thead>
              <tbody>
                {txs.data.data.map((t) => (
                  <tr key={t.id} className="border-b border-[var(--color-line)] last:border-b-0">
                    <td className="num px-5 py-3">
                      {new Date(t.booked_at).toLocaleDateString("id-ID")}
                    </td>
                    <td className="px-5 py-3">{t.direction}</td>
                    <td className="num px-5 py-3 text-right">
                      {formatMinor(t.amount_minor, t.currency)}
                    </td>
                    <td className="max-w-64 truncate px-5 py-3 text-[var(--color-ink-muted)]">
                      {t.description}
                    </td>
                    <td className="num px-5 py-3 text-xs text-[var(--color-ink-faint)]">
                      {t.external_ref ?? "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
    </section>
  );
}

