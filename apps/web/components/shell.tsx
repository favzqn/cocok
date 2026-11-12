"use client";

import { useEffect, useState } from "react";
import type { OrgSummary, User } from "@/lib/api";

// anchor = section id on the dashboard; null = not built yet (disabled).
export const NAV: { label: string; anchor: string | null }[] = [
  { label: "Overview", anchor: "overview" },
  { label: "Transactions", anchor: "transactions" },
  { label: "Reconciliation", anchor: "reconciliation" },
  { label: "Settlements", anchor: null },
  { label: "Financial Accounts", anchor: "accounts" },
  { label: "Integrations", anchor: null },
  { label: "Reports", anchor: null },
  { label: "Audit Logs", anchor: null },
  { label: "Settings", anchor: null },
];

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
  const [hash, setHash] = useState("");
  useEffect(() => {
    const sync = () => setHash(window.location.hash);
    sync();
    window.addEventListener("hashchange", sync);
    return () => window.removeEventListener("hashchange", sync);
  }, []);
  const activeItem = NAV.find((n) => `#${n.anchor}` === hash)?.label ?? "Overview";
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
          {NAV.map((item) => {
            const active = item.label === (activeItem ?? "Overview");
            if (!item.anchor) {
              return (
                <span
                  key={item.label}
                  aria-disabled="true"
                  className="cursor-not-allowed rounded-md px-3 py-2 text-sm text-white/30"
                  title="Coming soon"
                >
                  {item.label}
                </span>
              );
            }
            return (
              <a
                key={item.label}
                href={`#${item.anchor}`}
                className={`rounded-md px-3 py-2 text-sm ${
                  active
                    ? "bg-white/10 font-medium text-white"
                    : "text-white/60 hover:bg-white/5 hover:text-white/80"
                }`}
              >
                {item.label}
              </a>
            );
          })}
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