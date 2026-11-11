"use client";

import type { OrgSummary, User } from "@/lib/api";

export const NAV = [
  "Overview",
  "Transactions",
  "Reconciliation",
  "Settlements",
  "Financial Accounts",
  "Integrations",
  "Reports",
  "Audit Logs",
  "Settings",
] as const;

export function Shell({
  user,
  orgs,
  activeOrgId,
  onSwitchOrg,
  onLogout,
  main,
}: {
  user?: User;
  orgs?: OrgSummary[];
  activeOrgId?: string;
  onSwitchOrg?: (id: string) => void;
  onLogout?: () => void;
  main?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh">
      <aside className="flex w-60 shrink-0 flex-col bg-[var(--color-navy-950)] text-white">
        <div className="px-5 py-5 text-xs font-semibold tracking-[0.2em] text-white/60 uppercase">
          COCOK
        </div>
        {orgs && orgs.length > 0 && onSwitchOrg && (
          <div className="px-3 pb-3">
            <label htmlFor="org-switch" className="sr-only">
              Organization
            </label>
            <select
              id="org-switch"
              value={activeOrgId ?? ""}
              onChange={(e) => onSwitchOrg(e.target.value)}
              className="w-full rounded-md border border-white/15 bg-white/10 px-2.5 py-2 text-sm text-white outline-none focus:border-white/40"
            >
              {orgs.map((o) => (
                <option key={o.id} value={o.id} className="bg-[var(--color-navy-950)]">
                  {o.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <nav className="flex flex-1 flex-col gap-0.5 px-2">
          {NAV.map((item, i) => (
            <span
              key={item}
              className={
                i === 0
                  ? "rounded-md bg-white/10 px-3 py-2 text-sm font-medium text-white"
                  : "rounded-md px-3 py-2 text-sm text-white/60"
              }
            >
              {item}
            </span>
          ))}
        </nav>
        <div className="border-t border-white/10 px-5 py-4">
          <div className="truncate text-sm text-white/80">
            {user?.full_name ?? "…"}
          </div>
          <div className="truncate text-xs text-white/50">{user?.email}</div>
          <button
            onClick={onLogout}
            className="mt-3 text-xs font-medium text-white/70 underline underline-offset-2 hover:text-white"
          >
            Sign out
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-x-auto bg-[var(--color-canvas)] px-10 py-8">
        <div className="mx-auto max-w-5xl">{main ?? <p>Loading…</p>}</div>
      </main>
    </div>
  );
}