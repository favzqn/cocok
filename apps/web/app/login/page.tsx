"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { api, type User } from "@/lib/api";

const schema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(1, "Password is required"),
});

type FormValues = z.infer<typeof schema>;

export default function LoginPage() {
  const router = useRouter();
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>();

  async function onSubmit(values: FormValues) {
    try {
      await api<User>("/auth/login", {
        method: "POST",
        body: JSON.stringify(values),
      });
      router.push("/dashboard");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Login failed";
      setError("root", { message: msg });
    }
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-[var(--color-canvas)] px-6 py-12">
      <div className="w-full max-w-sm">
        <Link
          href="/"
          className="text-xs font-semibold tracking-[0.2em] text-[var(--color-ink-faint)] uppercase"
        >
          COCOK
        </Link>
        <h1 className="mt-4 text-xl font-semibold">Sign in</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-muted)]">
          Access your reconciliation workspace.
        </p>

        <form
          onSubmit={handleSubmit(onSubmit)}
          className="mt-6 flex flex-col gap-4"
          noValidate
        >
          {errors.root && (
            <div
              role="alert"
              className="rounded-md border border-[var(--color-alert)]/30 bg-[var(--color-alert)]/5 px-3 py-2 text-sm text-[var(--color-alert)]"
            >
              {errors.root.message}
            </div>
          )}

          <label className="flex flex-col gap-1.5 text-sm font-medium">
            Email
            <input
              type="email"
              autoComplete="email"
              required
              {...register("email", {
                required: "Email is required",
                pattern: { value: /\S+@\S+\.\S+/, message: "Enter a valid email" },
              })}
              className="h-10 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)] focus-visible:ring-2 focus-visible:ring-[var(--color-navy-900)]/20"
            />
            {errors.email && (
              <span className="text-xs text-[var(--color-alert)]">
                {errors.email.message}
              </span>
            )}
          </label>

          <label className="flex flex-col gap-1.5 text-sm font-medium">
            Password
            <input
              type="password"
              autoComplete="current-password"
              required
              {...register("password", {
                required: "Password is required",
              })}
              className="h-10 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)] focus-visible:ring-2 focus-visible:ring-[var(--color-navy-900)]/20"
            />
            {errors.password && (
              <span className="text-xs text-[var(--color-alert)]">
                {errors.password.message}
              </span>
            )}
          </label>

          <button
            type="submit"
            disabled={isSubmitting}
            className="inline-flex h-10 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white transition-colors hover:bg-[var(--color-navy-800)] disabled:opacity-60"
          >
            {isSubmitting ? "Signing in…" : "Sign in"}
          </button>
          <a
            href="/forgot-password"
            className="text-right text-xs text-[var(--color-ink-muted)] underline underline-offset-2"
          >
            Forgot password?
          </a>
        </form>

        <p className="mt-6 text-sm text-[var(--color-ink-muted)]">
          No account?{" "}
          <Link
            href="/register"
            className="font-medium text-[var(--color-ink)] underline underline-offset-2"
          >
            Create one
          </Link>
        </p>
      </div>
    </main>
  );
}
