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
