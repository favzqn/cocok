"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { api, type User } from "@/lib/api";

const schema = z
  .object({
    full_name: z.string().min(1, "Full name is required"),
    email: z.string().email("Enter a valid email"),
    password: z.string().min(8, "Password must be at least 8 characters"),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, {
    message: "Passwords do not match",
    path: ["confirm"],
  });

type FormValues = z.infer<typeof schema>;

export default function RegisterPage() {
  const router = useRouter();
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>();

  async function onSubmit(values: FormValues) {
    try {
      await api<User>("/auth/register", {
        method: "POST",
        body: JSON.stringify({
          full_name: values.full_name,
          email: values.email,
          password: values.password,
        }),
      });
      router.push("/dashboard");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Registration failed";
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
        <h1 className="mt-4 text-xl font-semibold">Create account</h1>
        <p className="mt-1 text-sm text-[var(--color-ink-muted)]">
          Start reconciling in minutes.
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

          <Field label="Full name" error={errors.full_name?.message}>
            <input
              type="text"
              autoComplete="name"
              {...register("full_name", { required: "Full name is required" })}
              className={inputCls}
            />
          </Field>

          <Field label="Email" error={errors.email?.message}>
            <input
              type="email"
              autoComplete="email"
              {...register("email", {
                required: "Email is required",
                pattern: { value: /\S+@\S+\.\S+/, message: "Enter a valid email" },
              })}
              className={inputCls}
            />
          </Field>

          <Field label="Password" error={errors.password?.message}>
            <input
              type="password"
              autoComplete="new-password"
              {...register("password", {
                required: "Password is required",
                minLength: { value: 8, message: "Password must be at least 8 characters" },
              })}
              className={inputCls}
            />
          </Field>

          <Field label="Confirm password" error={errors.confirm?.message}>
            <input
              type="password"
              autoComplete="new-password"
              {...register("confirm", { required: "Confirm your password" })}
              className={inputCls}
            />
          </Field>

          <button
            type="submit"
            disabled={isSubmitting}
            className="inline-flex h-10 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white transition-colors hover:bg-[var(--color-navy-800)] disabled:opacity-60"
          >
            {isSubmitting ? "Creating…" : "Create account"}
          </button>
        </form>

        <p className="mt-6 text-sm text-[var(--color-ink-muted)]">
          Already registered?{" "}
          <Link
            href="/login"
            className="font-medium text-[var(--color-ink)] underline underline-offset-2"
          >
            Sign in
          </Link>
        </p>
      </div>
    </main>
  );
}

const inputCls =
  "h-10 rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-3 text-sm outline-none focus:border-[var(--color-navy-900)] focus-visible:ring-2 focus-visible:ring-[var(--color-navy-900)]/20";

function Field({
  label,
  error,
  children,
}: {
  label: string;
  error?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="flex flex-col gap-1.5 text-sm font-medium">
      {label}
      {children}
      {error && <span className="text-xs text-[var(--color-alert)]">{error}</span>}
    </label>
  );
}
