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

export type MatchGroup = {
  id: string;
  status: string;
  kind: string;
  score?: number;
  weights_version?: number;
  components?: Record<string, number>;
  credit_tx_ids: string[];
  debit_tx_ids: string[];
  created_at: string;
  decided_at?: string;
};

// match:approve = owner/admin/finance_manager (mirrors backend RBAC).
export function canApproveMatches(role: string): boolean {
  return role === "owner" || role === "admin" || role === "finance_manager";
}

// reconcile:run = everyone but viewer.
export function canRunRecon(role: string): boolean {
  return role !== "viewer";
}

// exception:resolve = owner/admin/finance_manager/finance_staff.
export function canResolveExceptions(role: string): boolean {
  return role !== "viewer";
}

export type ReportSummary = {
  transactions: { total_count: number; total_credit_minor: number; total_debit_minor: number };
  matched: {
    count: number;
    credit_count: number;
    debit_count: number;
    credit_minor: number;
    debit_minor: number;
    match_rate: number;
  };
  groups_by_status: Record<string, number>;
  exceptions_by_status: Record<string, number>;
  settlements_by_status: Record<
    string,
    { count: number; expected_minor: number; settled_minor: number }
  >;
  accounts: {
    id: string;
    name: string;
    currency: string;
    tx_count: number;
    credit_minor: number;
    debit_minor: number;
    matched_count: number;
  }[];
};

export const REPORT_KINDS = [
  { kind: "transactions", label: "Transactions" },
  { kind: "matches", label: "Match groups" },
  { kind: "exceptions", label: "Exceptions" },
  { kind: "settlements", label: "Settlements" },
] as const;


// integration:manage = owner/admin only (mirrors backend RBAC).
export function canManageIntegrations(role: string): boolean {
  return canManage(role);
}

export type Integration = {
  id: string;
  provider: string;
  mode: string;
  default_account_id?: string;
  status: string;
  webhook_url: string;
  created_at: string;
};

export type Exception = {
  id: string;
  type: string;
  status: string;
  subject_tx_ids: string[];
  detail?: Record<string, unknown>;
  note?: string;
  detected_at: string;
  resolved_at?: string;
};

export type Settlement = {
  id: string;
  account_id?: string;
  counterparty?: string;
  reference?: string;
  expected_amount_minor: number;
  currency: string;
  expected_date: string;
  status: string;
  settled_amount_minor: number;
  settled_at?: string;
  notes?: string;
  linked_group_id?: string;
  created_at: string;
};

export const SETTLEMENT_STATUSES = ["pending", "partial", "settled", "failed"] as const;


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

export type AuditLog = {
  id: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  actor_id?: string;
  reason?: string;
  new_state?: Record<string, unknown>;
  created_at: string;
};

// audit:view = everyone but viewer.
export function canViewAudit(role: string): boolean {
  return role !== "viewer";
}

export const TIMEZONES = [
  "Asia/Jakarta",
  "Asia/Makassar",
  "Asia/Jayapura",
  "UTC",
] as const;
