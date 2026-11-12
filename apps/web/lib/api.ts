const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

type ErrorBody = {
  error?: { code?: string; message?: string };
};

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch(`${API_URL}/api/v1${path}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });

  if (!res.ok) {
    let code = "unknown";
    let message = `Request failed (${res.status})`;
    try {
      const body = (await res.json()) as ErrorBody;
      if (body.error) {
        code = body.error.code ?? code;
        message = body.error.message ?? message;
      }
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, code, message);
  }

  return (await res.json()) as T;
}

export type User = {
  id: string;
  email: string;
  full_name: string;
  status: string;
  created_at: string;
};

export type Organization = {
  id: string;
  name: string;
  slug: string;
  business_type: string | null;
  default_currency: string;
  timezone: string;
  subscription_plan: string;
  status: string;
  role?: string;
  created_at: string;
};

export type OrgSummary = Organization & { role: string };

export type Member = {
  id: string;
  user_id: string;
  email: string;
  full_name: string;
  role: string;
  status: string;
  created_at: string;
};

export type Invite = {
  id: string;
  email: string;
  role: string;
  expires_at: string;
  invite_token: string;
};

export const ROLES = [
  "owner",
  "admin",
  "finance_manager",
  "finance_staff",
  "viewer",
] as const;

export function roleLabel(role: string): string {
  return role.replace(/_/g, " ");
}

export function canManage(role: string): boolean {
  return role === "owner" || role === "admin";
}

// RBAC mirrors (backend enforces; UI only hides affordances).
export function canManageAccounts(role: string): boolean {
  return canManage(role);
}

export function canImport(role: string): boolean {
  return role !== "viewer";
}

export type FinancialAccount = {
  id: string;
  name: string;
  kind: string;
  account_number: string | null;
  currency: string;
  status: string;
  created_at: string;
  updated_at: string;
};

export type ImportBatch = {
  id: string;
  account_id: string;
  filename: string;
  status: string;
  row_count: number;
  valid_count: number;
  invalid_count: number;
  inserted_count: number;
  duplicate_count: number;
  created_at: string;
  committed_at: string | null;
};

export type ImportPreview = {
  batch_id: string;
  account_id: string;
  filename: string;
  headers: string[];
  mapping: Record<string, string>;
  row_count: number;
  valid_count: number;
  invalid_count: number;
  errors: { row: number; field?: string; message: string }[];
  sample: {
    row: number;
    date: string;
    direction: string;
    amount_minor: number;
    description?: string;
    counterparty?: string;
    external_ref?: string;
  }[];
};

export type CanonicalTransaction = {
  id: string;
  account_id: string;
  booked_at: string;
  direction: string;
  amount_minor: number;
  currency: string;
  description: string;
  counterparty: string | null;
  external_ref: string | null;
  created_at: string;
};

export function formatMinor(amountMinor: number, currency: string): string {
  const decimals = currency === "IDR" ? 0 : 2;
  const major = amountMinor / 10 ** decimals;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency,
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(major);
}
