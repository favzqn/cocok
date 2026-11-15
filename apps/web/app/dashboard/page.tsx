"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, type OrgSummary, type User } from "@/lib/api";
import { Shell } from "@/components/shell";
import { CreateOrgForm, OrgList } from "@/components/orgs";
import { MembersPanel } from "@/components/members";
import { IngestPanel } from "@/components/ingest";
import { ReconPanel } from "@/components/recon";
import { ExceptionsPanel } from "@/components/exceptions";
import { SettlementsPanel } from "@/components/settlements";
import { ReportsPanel } from "@/components/reports";
import { IntegrationsPanel } from "@/components/integrations";
import { AuditPanel } from "@/components/audit";
import { SettingsPanel } from "@/components/settings";

const ACTIVE_ORG_KEY = "cocok.activeOrgId";

export default function DashboardPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [orgName, setOrgName] = useState("");
  const [activeOrgId, setActiveOrgId] = useState<string | undefined>(undefined);

  const me = useQuery({
    queryKey: ["me"],
    queryFn: () => api<User>("/auth/me"),
    retry: false,
  });

  const orgs = useQuery({
    queryKey: ["organizations"],
    queryFn: () => api<{ data: OrgSummary[] }>("/organizations"),
    enabled: me.isSuccess,
  });

  const items = orgs.data?.data;

  useEffect(() => {
    if (!items || items.length === 0) return;
    const saved = localStorage.getItem(ACTIVE_ORG_KEY);
    const valid = saved && items.some((o) => o.id === saved) ? saved : items[0].id;
    setActiveOrgId(valid);
  }, [items]);

  function selectOrg(id: string) {
    setActiveOrgId(id);
    localStorage.setItem(ACTIVE_ORG_KEY, id);
  }

  const createOrg = useMutation({
    mutationFn: (name: string) =>
      api<OrgSummary>("/organizations", {
        method: "POST",
        body: JSON.stringify({ name }),
      }),
    onSuccess: (org) => {
      setOrgName("");
      qc.invalidateQueries({ queryKey: ["organizations"] });
      selectOrg(org.id);
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

  const activeOrg = items?.find((o) => o.id === activeOrgId);

  return (
    <Shell
      user={me.data}
      orgs={items}
      activeOrgId={activeOrgId}
      onSwitchOrg={selectOrg}
      onLogout={logout}
      main={
        <div className="flex flex-col gap-8">
          <header id="overview" className="scroll-mt-8">
            <h1 className="text-lg font-semibold">Overview</h1>
            <p className="mt-1 text-sm text-[var(--color-ink-muted)]">
              Your organizations
            </p>
          </header>
          <OrgList state={orgs} activeOrgId={activeOrgId} onSelect={selectOrg} />
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
          {activeOrg && <MembersPanel org={activeOrg} />}
          {activeOrg && <IngestPanel org={activeOrg} />}
          {activeOrg && <ReconPanel org={activeOrg} />}
          {activeOrg && <ExceptionsPanel org={activeOrg} />}
          {activeOrg && <SettlementsPanel org={activeOrg} />}
          {activeOrg && <ReportsPanel org={activeOrg} />}
          {activeOrg && <IntegrationsPanel org={activeOrg} />}
          {activeOrg && <AuditPanel org={activeOrg} />}
          {activeOrg && <SettingsPanel org={activeOrg} />}
        </div>
      }
    />
  );
}