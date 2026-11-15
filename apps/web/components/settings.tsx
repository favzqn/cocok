"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, canManage, TIMEZONES, type OrgSummary } from "@/lib/api";

export function SettingsPanel({ org }: { org: OrgSummary }) {
  const qc = useQueryClient();
  const manage = canManage(org.role);
  const [name, setName] = useState(org.name);
  const [timezone, setTimezone] = useState(org.timezone);
  const [msg, setMsg] = useState<{ ok: boolean; text: string }>();

  useEffect(() => {
    setName(org.name);
    setTimezone(org.timezone);
  }, [org.name, org.timezone]);

  const save = useMutation({
    mutationFn: () =>
      api<OrgSummary>(`/organizations/${org.id}`, {
        method: "PATCH",
        body: JSON.stringify({ name, timezone }),
      }),
    onSuccess: () => {
      setMsg({ ok: true, text: "Settings saved." });
      qc.invalidateQueries({ queryKey: ["organizations"] });
      qc.invalidateQueries({ queryKey: ["report-summary", org.id] });
    },
    onError: (err) =>
      setMsg({ ok: false, text: err instanceof Error ? err.message : "Save failed" }),
  });

  return (
    <section
      id="settings"
      className="scroll-mt-8 rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)]"
    >
      <div className="border-b border-[var(--color-line)] px-5 py-4 text-sm font-medium">
        Settings — {org.name}
      </div>
      <div className="flex flex-col gap-4 px-5 py-4">
        {msg && (
          <p
            className={`text-sm ${msg.ok ? "text-[var(--color-ink-muted)]" : "text-[var(--color-alert)]"}`}
          >
            {msg.text}
          </p>
        )}
        {!manage && (
          <p className="text-sm text-[var(--color-ink-muted)]">
            Only owners and admins can change settings. Your role: {org.role}.
          </p>
        )}
        {manage && (
          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (name.trim()) save.mutate();
            }}
          >
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Organization name
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="h-9 w-64 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm text-[var(--color-ink)]"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-[var(--color-ink-faint)]">
              Timezone (import dates &amp; report ranges)
              <select
                value={timezone}
                onChange={(e) => setTimezone(e.target.value)}
                className="h-9 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-2 text-sm text-[var(--color-ink)]"
              >
                {TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="submit"
              disabled={save.isPending}
              className="inline-flex h-9 items-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
            >
              {save.isPending ? "Saving…" : "Save settings"}
            </button>
          </form>
        )}
        <div className="num text-xs text-[var(--color-ink-faint)]">
          org id {org.id} · plan {org.subscription_plan} · created{" "}
          {new Date(org.created_at).toLocaleDateString("id-ID")}
        </div>
      </div>
    </section>
  );
}
