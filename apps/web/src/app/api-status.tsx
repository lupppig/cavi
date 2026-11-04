import { connection } from "next/server";

type Status =
  | { state: "ready" }
  | { state: "degraded"; detail: string }
  | { state: "unreachable" }
  | { state: "unconfigured" };

async function getApiStatus(): Promise<Status> {
  // Read the API URL at request time so one build can be promoted
  // across environments.
  await connection();
  const baseUrl = process.env.CAVI_API_URL;
  if (!baseUrl) return { state: "unconfigured" };

  try {
    const res = await fetch(new URL("/api/v1/health/ready", baseUrl), {
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    if (res.ok) return { state: "ready" };
    const body = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    return {
      state: "degraded",
      detail: body?.error?.message ?? `HTTP ${res.status}`,
    };
  } catch {
    return { state: "unreachable" };
  }
}

const labels: Record<Status["state"], string> = {
  ready: "API ready",
  degraded: "API degraded",
  unreachable: "API unreachable",
  unconfigured: "API URL not configured",
};

const tone: Record<Status["state"], string> = {
  ready: "bg-signal-ok",
  degraded: "bg-signal-warn",
  unreachable: "bg-signal-error",
  unconfigured: "bg-signal-warn",
};

export async function ApiStatus() {
  const status = await getApiStatus();
  return (
    <p className="flex items-center gap-2" role="status">
      <span aria-hidden className={`size-1.5 rounded-full ${tone[status.state]}`} />
      <span>{labels[status.state]}</span>
      {status.state === "degraded" && (
        <span className="text-muted">— {status.detail}</span>
      )}
    </p>
  );
}

export function ApiStatusPending() {
  return (
    <p className="flex items-center gap-2 text-muted" role="status">
      <span aria-hidden className="size-1.5 rounded-full bg-muted" />
      <span>Checking API…</span>
    </p>
  );
}
