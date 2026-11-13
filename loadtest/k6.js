// COCOK smoke load test.
// Run:  k6 run -e BASE_URL=http://localhost:8081 loadtest/k6.js
// Auth rate limit must be relaxed for load runs:
//   AUTH_RATE_RPS=100 go run ./cmd/server   (API start env)
import http from "k6/http";
import { check, sleep } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8081";

export const options = {
  vus: 5,
  duration: "15s",
  thresholds: {
    http_req_failed: ["rate<0.01"], // <1% errors
    http_req_duration: ["p(95)<500"], // p95 under 500ms locally
  },
};

let orgId = "";

export function setup() {
  const email = `k6-setup-${Date.now()}@load.test`;
  const reg = http.post(
    `${BASE}/api/v1/auth/register`,
    JSON.stringify({ email, password: "rahasia123", full_name: "k6 setup" }),
    { headers: { "Content-Type": "application/json" } },
  );
  if (reg.status !== 201) {
    throw new Error(`setup register failed: ${reg.status}`);
  }
  // setup() runs in its own VU — export the session cookie explicitly; the
  // default VUs' cookie jars never see it.
  const detail = reg.cookies["cocok_session"];
  if (!detail || !detail[0] || !detail[0].value) {
    throw new Error("register response set no session cookie");
  }
  const org = http.post(
    `${BASE}/api/v1/organizations`,
    JSON.stringify({ name: `k6-org-${Date.now()}` }),
    {
      headers: {
        "Content-Type": "application/json",
        Cookie: `cocok_session=${detail[0].value}`,
      },
    },
  );
  if (org.status !== 201) {
    throw new Error(`setup org failed: ${org.status}`);
  }
  return { token: detail[0].value, orgId: org.json("id") };
}

export default function (data) {
  const n = __VU;
  const hdr = {
    headers: {
      "Content-Type": "application/json",
      Cookie: `cocok_session=${data.token}`,
    },
  };

  // Liveness
  const h = http.get(`${BASE}/healthz`);
  check(h, { "healthz 200": (r) => r.status === 200 });

  // Read path (session + tenant scoped)
  const orgs = http.get(`${BASE}/api/v1/organizations`, hdr);
  check(orgs, { "list orgs 200": (r) => r.status === 200 });

  const txs = http.get(`${BASE}/api/v1/organizations/${data.orgId}/transactions`, hdr);
  check(txs, { "list tx 200": (r) => r.status === 200 });

  const summary = http.get(`${BASE}/api/v1/organizations/${data.orgId}/reports/summary`, hdr);
  check(summary, { "summary 200": (r) => r.status === 200 });

  const groups = http.get(
    `${BASE}/api/v1/organizations/${data.orgId}/reconciliation/groups?status=proposed`,
    hdr,
  );
  check(groups, { "groups 200": (r) => r.status === 200 });

  // Write path: unique account per iteration (no cross-VU conflict)
  const acc = http.post(
    `${BASE}/api/v1/organizations/${data.orgId}/accounts`,
    JSON.stringify({ name: `k6-acc-${n}-${Date.now()}`, kind: "bank" }),
    hdr,
  );
  check(acc, { "create account 201": (r) => r.status === 201 });

  sleep(0.2);
}

export function teardown() {
  // Dev-load artifact; rows are harmless in the local database.
}
