"use client";

import { useMutation } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { api } from "@/lib/api";

function ResetForm() {
  const router = useRouter();
  const params = useSearchParams();
  const token = params.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [done, setDone] = useState(false);

  const reset = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/auth/reset-password", {
        method: "POST",
        body: JSON.stringify({ token, password }),
      }),
    onSuccess: () => {
      setDone(true);
      setTimeout(() => router.push("/login"), 1500);
    },
  });

  return (
    <div className="w-full max-w-sm rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] p-6">
      <h1 className="text-lg font-semibold">Reset password</h1>
      {!token ? (
        <p className="mt-3 text-sm text-[var(--color-alert)]">
          Missing token. Request a new link from{" "}
          <a href="/forgot-password" className="underline">
            forgot password
          </a>
          .
        </p>
      ) : done ? (
        <p className="mt-3 text-sm text-[var(--color-ink-muted)]">
          Password updated — redirecting to sign in…
        </p>
      ) : (
        <form
          className="mt-4 flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (password) reset.mutate();
          }}
        >
          <input
            type="password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="New password (min 8 chars)"
            aria-label="New password"
            className="h-10 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)]"
          />
          <button
            type="submit"
            disabled={reset.isPending}
            className="h-10 rounded-md bg-[var(--color-navy-900)] text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
          >
            {reset.isPending ? "Resetting…" : "Reset password"}
          </button>
          {reset.isError && (
            <p className="text-sm text-[var(--color-alert)]">
              {reset.error instanceof Error ? reset.error.message : "Reset failed"}
            </p>
          )}
        </form>
      )}
    </div>
  );
}

export default function ResetPasswordPage() {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-[var(--color-canvas)] px-4">
      <Suspense>
        <ResetForm />
      </Suspense>
    </main>
  );
}
