"use client";

import type { useQuery } from "@tanstack/react-query";
import { roleLabel, type OrgSummary } from "@/lib/api";

export function OrgList({
  state,
  activeOrgId,
  onSelect,
}: {
  state: ReturnType<typeof useQuery<{ data: OrgSummary[] }>>;
  activeOrgId?: string;
  onSelect?: (id: string) => void;
}) {
  if (state.isPending) {
    return (
      <section className="rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] px-5 py-6 text-sm text-[var(--color-ink-muted)]">
        Loading…
      </section>
    );
  }
  if (state.isError) {
    return (
      <section className="rounded-lg border border-[var(--color-alert)]/30 bg-[var(--color-surface)] px-5 py-6 text-sm text-[var(--color-alert)]">
        Failed to load organizations.
      </section>
    );
  }
  const items = state.data.data;
  return (
    <section className="rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]">
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Organizations
      </div>
      {items.length === 0 ? (
        <div className="px-5 py-8 text-sm text-[var(--color-ink-muted)]">
          No organizations yet. Create your first one below.
        </div>
      ) : (
        <ul>
          {items.map((o) => {
            const active = o.id === activeOrgId;
            return (
              <li key={o.id}>
                <button
                  type="button"
                  onClick={() => onSelect?.(o.id)}
                  className={`flex w-full items-center justify-between gap-4 px-5 py-3.5 text-left last:border-b-0 ${
                    active ? "bg-[var(--color-navy-950)]/[0.04]" : ""
                  } ${onSelect ? "hover:bg-[var(--color-navy-950)]/[0.03]" : ""} ${
                    active ? "border-l-2 border-[var(--color-navy-900)]" : "border-l-2 border-transparent"
                  }`}
                >
                  <div>
                    <div className="text-sm font-medium">{o.name}</div>
                    <div className="num text-xs text-[var(--color-ink-faint)]">
                      {o.slug} · {o.default_currency} · {o.timezone}
                    </div>
                  </div>
                  <span className="rounded border border-[var(--color-line)] px-2 py-0.5 text-xs font-medium uppercase">
                    {roleLabel(o.role)}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export function CreateOrgForm({
  value,
  onChange,
  onSubmit,
  pending,
  error,
}: {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  pending: boolean;
  error?: string;
}) {
  return (
    <div className="flex flex-col gap-3">
      <form
        className="flex gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit();
        }}
      >
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="New organization name"
          aria-label="New organization name"
          className="h-10 w-full max-w-xs rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)]"
        />
        <button
          type="submit"
          disabled={pending || !value.trim()}
          className="inline-flex h-10 shrink-0 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
        >
          {pending ? "Creating…" : "Create organization"}
        </button>
      </form>
      {error && <p className="text-sm text-[var(--color-alert)]">{error}</p>}
    </div>
  );
}