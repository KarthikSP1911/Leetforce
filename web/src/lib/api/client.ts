import type {
  Language,
  ProblemDetail,
  ProblemList,
  ProblemQuery,
} from "@/types/problem";
import type { User } from "@/types/auth";
import type { Leaderboard, Standings } from "@/types/leaderboard";
import type {
  ContestDetail,
  ContestProblem,
  ContestSummary,
} from "@/types/contest";
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
    readonly retryAfter?: number,
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
  cookie?: string,
): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    cache: "no-store",
    signal,
    headers: cookie ? { Cookie: cookie } : undefined,
  });
  return readJSON<T>(res);
}

async function postJSON<T>(
  path: string,
  body?: unknown,
  signal?: AbortSignal,
  cookie?: string,
): Promise<T> {
  const res = await fetch(`${baseUrl()}${path}`, {
    method: "POST",
    cache: "no-store",
    signal,
    headers: {
      "Content-Type": "application/json",
      ...(cookie ? { Cookie: cookie } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
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
    const retry = Number.parseInt(res.headers.get("Retry-After") ?? "", 10);
    throw new ApiError(
      res.status,
      message,
      Number.isFinite(retry) ? retry : undefined,
    );
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function listProblems(
  query: ProblemQuery = {},
  signal?: AbortSignal,
  opts: { cookie?: string } = {},
): Promise<ProblemList> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== "") params.set(key, String(value));
  }
  const qs = params.toString();
  return getJSON<ProblemList>(
    `/problems${qs ? `?${qs}` : ""}`,
    signal,
    opts.cookie,
  );
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

export async function signup(req: {
  email: string;
  username: string;
  password: string;
}): Promise<User> {
  return (await postJSON<{ user: User }>("/auth/signup", req)).user;
}

export async function login(req: {
  login: string;
  password: string;
}): Promise<User> {
  return (await postJSON<{ user: User }>("/auth/login", req)).user;
}

export async function logout(): Promise<void> {
  await postJSON<void>("/auth/logout");
}

/** The signed-in user, or null when anonymous. */
export async function getMe(signal?: AbortSignal): Promise<User | null> {
  try {
    return (await getJSON<{ user: User }>("/me", signal)).user;
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null;
    throw e;
  }
}

export function createSubmission(
  req: {
    problem: string;
    language: Language;
    source: string;
    /** Contest slug; the API answers 409 when the window is closed. */
    contest_id?: string;
  },
  signal?: AbortSignal,
): Promise<SubmissionCreated> {
  return postJSON<SubmissionCreated>("/submissions", req, signal);
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
  );
  return body.submissions;
}

/** Run on the sample tests, or once on `input` when it is given. */
export function createRun(
  req: { problem: string; language: Language; source: string; input?: string },
  signal?: AbortSignal,
): Promise<RunCreated> {
  return postJSON<RunCreated>("/runs", req, signal);
}

export function getRun(id: string, signal?: AbortSignal): Promise<RunState> {
  return getJSON<RunState>(`/runs/${encodeURIComponent(id)}`, signal);
}

export async function listContests(
  signal?: AbortSignal,
  opts: { cookie?: string } = {},
): Promise<ContestSummary[]> {
  const body = await getJSON<{ contests: ContestSummary[] }>(
    "/contests",
    signal,
    opts.cookie,
  );
  return body.contests;
}

export function getContest(
  slug: string,
  signal?: AbortSignal,
  opts: { cookie?: string } = {},
): Promise<ContestDetail> {
  return getJSON<ContestDetail>(
    `/contests/${encodeURIComponent(slug)}`,
    signal,
    opts.cookie,
  );
}

export function registerContest(
  slug: string,
  signal?: AbortSignal,
): Promise<{ registered: boolean }> {
  return postJSON<{ registered: boolean }>(
    `/contests/${encodeURIComponent(slug)}/register`,
    undefined,
    signal,
  );
}

/** Problems of a contest; the API answers 404 until the user may see them. */
export async function listContestProblems(
  slug: string,
  signal?: AbortSignal,
): Promise<ContestProblem[]> {
  const body = await getJSON<{ problems: ContestProblem[] }>(
    `/contests/${encodeURIComponent(slug)}/problems`,
    signal,
  );
  return body.problems;
}

export function getLeaderboard(
  page: number,
  perPage: number,
  signal?: AbortSignal,
): Promise<Leaderboard> {
  return getJSON<Leaderboard>(
    `/leaderboard?page=${page}&per_page=${perPage}`,
    signal,
  );
}

export function getStandings(
  slug: string,
  signal?: AbortSignal,
): Promise<Standings> {
  return getJSON<Standings>(
    `/contests/${encodeURIComponent(slug)}/standings`,
    signal,
  );
}
