import type { ProblemDetail, ProblemList, ProblemQuery } from "@/types/problem";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// On the server, call the API directly; in the browser, use the /api proxy
// (see next.config.ts).
function baseUrl(): string {
  if (typeof window === "undefined") {
    return process.env.LEETFORCE_API_URL ?? "http://127.0.0.1:8080";
  }
  return "/api";
}

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    cache: "no-store",
    signal,
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      // keep statusText
    }
    throw new ApiError(res.status, message);
  }
  return (await res.json()) as T;
}

export function listProblems(
  query: ProblemQuery = {},
  signal?: AbortSignal,
): Promise<ProblemList> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") params.set(key, String(value));
  }
  const qs = params.toString();
  return getJSON<ProblemList>(`/problems${qs ? `?${qs}` : ""}`, signal);
}

export function getProblem(
  slug: string,
  signal?: AbortSignal,
): Promise<ProblemDetail> {
  return getJSON<ProblemDetail>(
    `/problems/${encodeURIComponent(slug)}`,
    signal,
  );
}
