"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { api, type Organization, type User } from "@/lib/api";

const NAV = [
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

export default function DashboardPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [orgName, setOrgName] = useState("");

  const me = useQuery({
    queryKey: ["me"],
    queryFn: () => api<User>("/auth/me"),
    retry: false,
  });

  const orgs = useQuery({
    queryKey: ["organizations"],
    queryFn: () => api<{ data: Organization[] }>("/organizations"),
    enabled: me.isSuccess,
  });

  const createOrg = useMutation({
    mutationFn: (name: string) =>
      api<Organization>("/organizations", {
        method: "POST",
        body: JSON.stringify({ name }),
      }),
    onSuccess: () => {
      setOrgName("");
      qc.invalidateQueries({ queryKey: ["organizations"] });
    },
  });

  async function logout() {
    await api("/auth/logout", { method: "POST" });
    qc.clear();
    router.push("/login");
  }

  if (me.isPending) return <Shell />;
  if (me.isError) {
    router.push("/login");
    return null;
  }

  return (
    <Shell
      user={me.data}
      onLogout={logout}
      main={
        <div className="flex flex-col gap-8">
          <header>
            <h1 className="text-lg font-semibold">Overview</h1>
            <p className="mt-1 text-sm text-[var(--color-ink-muted)]">
              Your organizations
            </p>
          </header>
          <OrgList state={orgs} />
          <CreateOrgForm
            value={orgName}
            onChange={setOrgName}
            onSubmit={() => {
              if (orgName.trim()) createOrg.mutate(orgName.trim());
            }}
            pending={createOrg.isPending}
            error={
              createOrg.isError
                ? createOrg.error instanceof Error
                  ? createOrg.error.message
                  : "Failed to create organization"
                : undefined
            }
          />
        </div>
      }
    />
  );
}

function OrgList({
  state,
}: {
  state: ReturnType<typeof useQuery<{ data: Organization[] }>>;
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
          No organizations yet. Create your first workspace below.
        </div>
      ) : (
        <ul>
          {items.map((o) => (
            <li
              key={o.id}
              className="flex items-center justify-between border-b border-[var(--color-line)] px-5 py-3.5 last:border-b-0"
            >
              <div>
                <div className="text-sm font-medium">{o.name}</div>
                <div className="num text-xs text-[var(--color-ink-faint)]">
                  {o.slug} · {o.default_currency} · {o.timezone}
                </div>
              </div>
              <span className="rounded border border-[var(--color-line)] px-2 py-0.5 text-xs font-medium uppercase">
                {o.role}
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function CreateOrgForm({
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

function Shell({
  user,
  onLogout,
  main,
}: {
  user?: User;
  onLogout?: () => void;
  main?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-dvh">
      <aside className="flex w-60 shrink-0 flex-col bg-[var(--color-navy-950)] text-white">
        <div className="px-5 py-5 text-xs font-semibold tracking-[0.2em] text-white/60 uppercase">
          COCOK
        </div>
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
