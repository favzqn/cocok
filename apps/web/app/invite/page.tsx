"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { api, type Organization } from "@/lib/api";

function InviteContent() {
  const router = useRouter();
  const params = useSearchParams();
  const qc = useQueryClient();
  const token = params.get("token") ?? "";
  const [done, setDone] = useState(false);

  const accept = useMutation({
    mutationFn: () =>
      api<{ organization: Organization; role: string }>("/invitations/accept", {
        method: "POST",
        body: JSON.stringify({ token }),
      }),
    onSuccess: () => {
      setDone(true);
      qc.invalidateQueries({ queryKey: ["organizations"] });
      setTimeout(() => router.push("/dashboard"), 600);
    },
  });

  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-4 bg-[var(--color-canvas)] px-6 py-12">
      <div className="text-xs font-semibold tracking-[0.2em] text-[var(--color-ink-faint)] uppercase">
        COCOK
      </div>
      <div className="w-full max-w-sm rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] px-6 py-7">
        {!token ? (
          <p className="text-sm text-[var(--color-alert)]">
            Missing invite token. Ask the sender for a new link.
          </p>
        ) : done ? (
          <p className="text-sm">
            Joined{" "}
            <span className="font-medium">
              {accept.data?.organization.name}
            </span>{" "}
            as {accept.data?.role}. Redirecting…
          </p>
        ) : (
          <>
            <h1 className="text-xl font-semibold">Join organization</h1>
            <p className="mt-1 text-sm text-[var(--color-ink-muted)]">
              Accept this invitation with your current account.
            </p>
            {accept.isError && (
              <p className="mt-3 text-sm text-[var(--color-alert)]">
                {accept.error instanceof Error
                  ? accept.error.message
                  : "Failed to accept invite"}
              </p>
            )}
            <button
              onClick={() => accept.mutate()}
              disabled={accept.isPending}
              className="mt-5 inline-flex h-10 w-full items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
            >
              {accept.isPending ? "Accepting…" : "Accept invitation"}
            </button>
            <button
              onClick={() => router.push("/dashboard")}
              className="mt-3 w-full text-center text-sm text-[var(--color-ink-muted)] underline underline-offset-2"
            >
              Back to dashboard
            </button>
          </>
        )}
      </div>
    </main>
  );
}

export default function InvitePage() {
  return (
    <Suspense>
      <InviteContent />
    </Suspense>
  );
}