"use client";

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/lib/api";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [done, setDone] = useState(false);

  const forgot = useMutation({
    mutationFn: () =>
      api<{ ok: boolean }>("/auth/forgot-password", {
        method: "POST",
        body: JSON.stringify({ email }),
      }),
    onSuccess: () => setDone(true),
  });

  return (
    <main className="flex min-h-dvh items-center justify-center bg-[var(--color-canvas)] px-4">
      <div className="w-full max-w-sm rounded-lg border border-[var(--color-line)] bg-[var(--color-surface)] p-6">
        <h1 className="text-lg font-semibold">Forgot password</h1>
        {done ? (
          <p className="mt-3 text-sm text-[var(--color-ink-muted)]">
            If that email exists, a reset link is on its way. (Sandbox: check the
            API <code className="num">email_outbox</code> / log.)
          </p>
        ) : (
          <form
            className="mt-4 flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (email.trim()) forgot.mutate();
            }}
          >
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@company.id"
              aria-label="Email"
              className="h-10 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)]"
            />
            <button
              type="submit"
              disabled={forgot.isPending}
              className="h-10 rounded-md bg-[var(--color-navy-900)] text-sm font-medium text-white hover:bg-[var(--color-navy-800)] disabled:opacity-60"
            >
              {forgot.isPending ? "Sending…" : "Send reset link"}
            </button>
            {forgot.isError && (
              <p className="text-sm text-[var(--color-alert)]">Something went wrong.</p>
            )}
            <a href="/login" className="text-xs text-[var(--color-ink-muted)] underline">
              Back to sign in
            </a>
          </form>
        )}
      </div>
    </main>
  );
}
