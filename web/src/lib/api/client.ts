import type {
  Language,
  ProblemDetail,
  ProblemList,
  ProblemQuery,
} from "@/types/problem";
import type {
  RunCreated,
  RunState,
  Submission,
  SubmissionCreated,
} from "@/types/submission";

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

async function getJSON<T>(
  path: string,
  signal?: AbortSignal,
  headers?: Record<string, string>,
): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    cache: "no-store",
    signal,
    headers,
  });
  return readJSON<T>(res);
}

async function postJSON<T>(
  path: string,
  body: unknown,
  headers?: Record<string, string>,
  signal?: AbortSignal,
): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    method: "POST",
    cache: "no-store",
    signal,
    headers: { "Content-Type": "application/json", ...headers },
    body: JSON.stringify(body),
  });
  return readJSON<T>(res);
}

async function readJSON<T>(res: Response): Promise<T> {
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

// Anonymous browser id for "my submissions" until accounts exist (Phase 9).
// It is kept in localStorage; if storage is blocked, a per-page id is used.
const CLIENT_KEY = "lf-client";
let memoryClient: string | undefined;

export function clientId(): string {
  try {
    const saved = window.localStorage.getItem(CLIENT_KEY);
    if (saved) return saved;
    const fresh = crypto.randomUUID();
    window.localStorage.setItem(CLIENT_KEY, fresh);
    return fresh;
  } catch {
    memoryClient ??= crypto.randomUUID();
    return memoryClient;
  }
}

const clientHeaders = () => ({ "X-LeetForce-Client": clientId() });

export function createSubmission(
  req: { problem: string; language: Language; source: string },
  signal?: AbortSignal,
): Promise<SubmissionCreated> {
  return postJSON<SubmissionCreated>(
    "/submissions",
    req,
    clientHeaders(),
    signal,
  );
}

export function getSubmission(
  id: string,
  signal?: AbortSignal,
): Promise<Submission> {
  return getJSON<Submission>(`/submissions/${encodeURIComponent(id)}`, signal);
}

export async function listSubmissions(
  slug: string,
  signal?: AbortSignal,
): Promise<Submission[]> {
  const body = await getJSON<{ submissions: Submission[] }>(
    `/problems/${encodeURIComponent(slug)}/submissions`,
    signal,
    clientHeaders(),
  );
  return body.submissions;
}

/** Run on the sample tests, or once on `input` when it is given. */
export function createRun(
  req: { problem: string; language: Language; source: string; input?: string },
  signal?: AbortSignal,
): Promise<RunCreated> {
  return postJSON<RunCreated>("/runs", req, undefined, signal);
}

export function getRun(id: string, signal?: AbortSignal): Promise<RunState> {
  return getJSON<RunState>(`/runs/${encodeURIComponent(id)}`, signal);
}
