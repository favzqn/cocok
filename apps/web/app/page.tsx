import Link from "next/link";

export default function HomePage() {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-6 bg-[var(--color-canvas)] px-6">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <div className="text-xs font-semibold tracking-[0.2em] text-[var(--color-ink-faint)] uppercase">
            COCOK
          </div>
          <h1 className="mt-3 text-2xl font-semibold text-[var(--color-ink)]">
            Financial reconciliation for Indonesian businesses
          </h1>
          <p className="mt-2 text-sm text-[var(--color-ink-muted)]">
            Import from anywhere. Reconcile everything. Investigate only what
            doesn&apos;t match.
          </p>
        </div>
        <div className="flex flex-col gap-3">
          <Link
            href="/login"
            className="inline-flex h-10 items-center justify-center rounded-md bg-[var(--color-navy-900)] px-4 text-sm font-medium text-white transition-colors hover:bg-[var(--color-navy-800)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-navy-900)]"
          >
            Sign in
          </Link>
          <Link
            href="/register"
            className="inline-flex h-10 items-center justify-center rounded-md border border-[var(--color-line)] bg-[var(--color-surface)] px-4 text-sm font-medium text-[var(--color-ink)] transition-colors hover:bg-[var(--color-canvas)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--color-navy-900)]"
          >
            Create account
          </Link>
        </div>
      </div>
    </main>
  );
}
