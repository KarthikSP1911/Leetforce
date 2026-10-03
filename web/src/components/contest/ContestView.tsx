"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useUser } from "@/components/auth/AuthProvider";
import {
  ApiError,
  getContest,
  listContestProblems,
  registerContest,
} from "@/lib/api/client";
import type {
  ContestDetail,
  ContestProblem,
  ContestStatus,
} from "@/types/contest";
import type { Difficulty } from "@/types/problem";
import { StandingsTable } from "./standings-slot";
import { formatTime, StatusBadge } from "./StatusBadge";

const difficultyClass: Record<Difficulty, string> = {
  easy: "text-success",
  medium: "text-warning",
  hard: "text-danger",
};
const difficultyLabel: Record<Difficulty, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
};

function pad(n: number): string {
  return String(n).padStart(2, "0");
}

function formatRemaining(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const clock = `${pad(h)}:${pad(m)}:${pad(s % 60)}`;
  return d > 0 ? `${d}d ${clock}` : clock;
}

export function ContestView({ initial }: { initial: ContestDetail }) {
  const { user } = useUser();
  const [contest, setContest] = useState(initial);
  // Server clock minus client clock, measured once when the page loaded.
  const [offset] = useState(
    () => new Date(initial.server_time).getTime() - Date.now(),
  );
  const [now, setNow] = useState(() => Date.now() + offset);
  const [problems, setProblems] = useState<ContestProblem[] | null>(null);
  const [active, setActive] = useState(0);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  const start = new Date(contest.starts_at).getTime();
  const end = new Date(contest.ends_at).getTime();

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now() + offset), 1000);
    return () => clearInterval(id);
  }, [offset]);

  // The status follows the corrected clock, so the page flips at the boundary.
  const status: ContestStatus = useMemo(() => {
    if (now >= end) return "ended";
    if (now >= start) return "running";
    return "upcoming";
  }, [now, start, end]);

  const slug = contest.slug;
  const registered = contest.registered;
  const userId = user?.id;

  useEffect(() => {
    const ctl = new AbortController();
    listContestProblems(slug, ctl.signal)
      .then((p) => setProblems(p))
      .catch((e: unknown) => {
        if (ctl.signal.aborted) return;
        // 404 means the user may not see the problems yet.
        setProblems(e instanceof ApiError && e.status === 404 ? [] : null);
      });
    return () => ctl.abort();
  }, [slug, status, registered, userId]);

  // Signed-in state is only known on the client, so re-read the registration.
  useEffect(() => {
    if (!userId) return;
    const ctl = new AbortController();
    getContest(slug, ctl.signal)
      .then((c) => setContest(c))
      .catch(() => {});
    return () => ctl.abort();
  }, [userId, slug]);

  const register = useCallback(async () => {
    setError("");
    setPending(true);
    try {
      await registerContest(slug);
      setContest((c) => ({ ...c, registered: true }));
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 409
          ? "This contest has ended; registration is closed."
          : e instanceof ApiError
            ? e.message
            : "Could not reach the API.",
      );
    } finally {
      setPending(false);
    }
  }, [slug]);

  const countdown =
    status === "upcoming"
      ? { label: "Starts in", ms: start - now }
      : status === "running"
        ? { label: "Ends in", ms: end - now }
        : null;
  const tab = problems && problems.length > 0 ? problems[active] : undefined;

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <header className="bg-panel border-panel-border mb-6 flex flex-wrap items-center justify-between gap-4 rounded-lg border px-4 py-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold">{contest.title}</h1>
            <StatusBadge status={status} />
          </div>
          <p className="text-muted mt-1 text-sm">
            {formatTime(contest.starts_at)} to {formatTime(contest.ends_at)}
          </p>
        </div>
        <div className="flex items-center gap-4">
          {countdown && (
            <div className="text-right">
              <div className="text-muted text-xs uppercase">
                {countdown.label}
              </div>
              <div
                className="font-mono text-xl font-semibold"
                role="timer"
                aria-label={countdown.label}
              >
                {formatRemaining(countdown.ms)}
              </div>
            </div>
          )}
          {contest.registered ? (
            <span className="text-success text-sm font-semibold">
              Registered
            </span>
          ) : status === "ended" ? null : user ? (
            <button
              type="button"
              onClick={register}
              disabled={pending}
              className="bg-primary rounded-md px-4 py-2 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-60"
            >
              {pending ? "Registering..." : "Register"}
            </button>
          ) : (
            <Link
              href={`/login?next=${encodeURIComponent(`/contest/${slug}`)}`}
              className="text-link text-sm font-semibold hover:underline"
            >
              Sign in to register
            </Link>
          )}
        </div>
      </header>
      {error && (
        <p
          role="alert"
          className="bg-panel border-panel-border text-danger mb-4 rounded-lg border px-4 py-3 text-sm"
        >
          {error}
        </p>
      )}

      <section aria-labelledby="problems-h" className="mb-8">
        <h2 id="problems-h" className="mb-3 text-lg font-bold">
          Problems
        </h2>
        {tab ? (
          <div className="bg-panel border-panel-border rounded-lg border">
            <div
              role="tablist"
              className="border-panel-border flex gap-1 border-b px-2"
            >
              {problems?.map((p, i) => (
                <button
                  key={p.slug}
                  role="tab"
                  type="button"
                  aria-selected={i === active}
                  onClick={() => setActive(i)}
                  className={`px-4 py-2 text-sm font-semibold ${
                    i === active
                      ? "text-link border-primary border-b-2"
                      : "text-muted hover:bg-hover"
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
            <div
              role="tabpanel"
              className="flex flex-wrap items-center justify-between gap-3 px-4 py-4"
            >
              <div>
                <div className="font-medium">
                  {tab.label}. {tab.title}
                </div>
                <div className="mt-1 text-sm">
                  <span
                    className={`font-semibold ${difficultyClass[tab.difficulty]}`}
                  >
                    {difficultyLabel[tab.difficulty]}
                  </span>
                  <span className="text-muted ml-3 font-mono">
                    {tab.points} points
                  </span>
                </div>
              </div>
              <Link
                href={`/problems/${tab.slug}?contest=${encodeURIComponent(slug)}`}
                className="bg-primary rounded-md px-4 py-2 text-sm font-semibold text-white transition-opacity hover:opacity-90"
              >
                Open problem
              </Link>
            </div>
          </div>
        ) : (
          <p className="bg-panel border-panel-border text-muted rounded-lg border px-4 py-6 text-center text-sm">
            {problems === null
              ? "Could not load the problems."
              : status === "upcoming"
                ? "Problems are revealed when the contest starts."
                : "Register to see the problems."}
          </p>
        )}
      </section>

      <section aria-labelledby="standings-h">
        <h2 id="standings-h" className="mb-3 text-lg font-bold">
          Standings
        </h2>
        <StandingsTable contestSlug={slug} />
      </section>
    </main>
  );
}
