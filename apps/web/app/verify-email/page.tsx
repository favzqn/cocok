"use client";

import { useMutation } from "@tanstack/react-query";
import { useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";
import { api } from "@/lib/api";

function VerifyForm() {
  const params = useSearchParams();
  const token = params.get("token") ?? "";

  const verify = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/auth/verify-email", {
        method: "POST",
        body: JSON.stringify({ token }),
      }),
    retry: false,
  });

  useEffect(() => {
    if (token && !verify.isPending && !verify.isSuccess && !verify.isError) {
      verify.mutate();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  return (
    <div className="w-full max-w-sm rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] p-6 text-center">
      <h1 className="text-lg font-semibold">Email verification</h1>
      {!token ? (
        <p className="mt-3 text-sm text-[var(--color-alert)]">Missing token.</p>
      ) : verify.isPending ? (
        <p className="mt-3 text-sm text-[var(--color-ink-muted)]">Verifying…</p>
      ) : verify.isSuccess ? (
        <p className="mt-3 text-sm text-[var(--color-ink-muted)]">
          Verified! You can now{" "}
          <a href="/login" className="underline">
            sign in
          </a>
          .
        </p>
      ) : (
        <p className="mt-3 text-sm text-[var(--color-alert)]">
          Invalid or expired token — request a new verification email.
        </p>
      )}
    </div>
  );
}

export default function VerifyEmailPage() {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-[var(--color-canvas)] px-4">
      <Suspense>
        <VerifyForm />
      </Suspense>
    </main>
  );
}
