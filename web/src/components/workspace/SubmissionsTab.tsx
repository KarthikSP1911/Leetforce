"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { useUser } from "@/components/auth/AuthProvider";
import { listSubmissions } from "@/lib/api/client";
import type { Submission, Verdict } from "@/types/submission";

const verdictLabel: Record<Verdict, string> = {
  AC: "Accepted",
  WA: "Wrong Answer",
  TLE: "Time Limit Exceeded",
  MLE: "Memory Limit Exceeded",
  RE: "Runtime Error",
  CE: "Compile Error",
  OLE: "Output Limit Exceeded",
  IE: "Internal Error",
};

const verdictClass: Record<Verdict, string> = {
  AC: "text-success",
  TLE: "text-warning",
  MLE: "text-warning",
  OLE: "text-warning",
  WA: "text-danger",
  RE: "text-danger",
  CE: "text-danger",
  IE: "text-muted",
};

const languageLabel: Record<string, string> = {
  python: "Python",
  cpp: "C++",
  java: "Java",
  go: "Go",
};

const POLL_MS = 2000;

function when(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleString();
}

function memory(kb: number): string {
  return kb >= 1024 ? `${(kb / 1024).toFixed(1)} MB` : `${kb} KB`;
}

function Status({ s }: { s: Submission }) {
  if (s.status === "judged" && s.verdict) {
    return (
      <span className={`font-semibold ${verdictClass[s.verdict.verdict]}`}>
        {verdictLabel[s.verdict.verdict] ?? s.verdict.verdict}
      </span>
    );
  }
  return (
    <span className="text-muted font-semibold">
      {s.status === "judging" ? "Judging…" : "Queued…"}
    </span>
  );
}

/**
 * Your submissions to one problem, newest first. `refreshKey` changes
 * when the page submits something; while any row is unfinished the list is
 * re-read every couple of seconds, so a verdict appears without a reload.
 */
export function SubmissionsTab({
  slug,
  refreshKey,
}: {
  slug: string;
  refreshKey: number;
}) {
  const { user, loading } = useUser();
  const userId = user?.id;
  const pathname = usePathname();
  const [rows, setRows] = useState<Submission[] | null>(null);
  const [failed, setFailed] = useState(false);
  const pending = rows?.some((r) => r.status !== "judged") ?? false;

  useEffect(() => {
    if (loading || !userId) return;
    const ctl = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = async () => {
      try {
        const next = await listSubmissions(slug, ctl.signal);
        setRows(next);
        setFailed(false);
        if (next.some((r) => r.status !== "judged")) {
          timer = setTimeout(load, POLL_MS);
        }
      } catch {
        if (ctl.signal.aborted) return;
        setFailed(true);
        timer = setTimeout(load, POLL_MS * 3);
      }
    };
    void load();
    return () => {
      ctl.abort();
      if (timer) clearTimeout(timer);
    };
    // `pending` is not a dependency: the loop keeps itself going.
  }, [slug, refreshKey, userId, loading]);

  if (!loading && !user) {
    return (
      <p className="text-muted p-4 text-sm">
        <Link
          href={`/login?next=${encodeURIComponent(pathname)}`}
          className="text-link font-semibold underline"
        >
          Sign in
        </Link>{" "}
        to see your submissions.
      </p>
    );
  }
  if (rows === null) {
    return (
      <p className="text-muted p-4 text-sm">
        {failed ? "Could not load submissions." : "Loading…"}
      </p>
    );
  }
  if (rows.length === 0) {
    return (
      <p className="text-muted p-4 text-sm">
        No submissions yet. Submit your code to see it here.
      </p>
    );
  }
  return (
    <div className="p-2" aria-busy={pending}>
      {failed && (
        <p className="text-danger px-2 pb-2 text-xs" role="alert">
          Could not refresh. Retrying…
        </p>
      )}
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="text-muted text-xs uppercase">
            <th className="px-2 py-2 font-medium">Status</th>
            <th className="px-2 py-2 font-medium">Language</th>
            <th className="px-2 py-2 font-medium">Runtime</th>
            <th className="px-2 py-2 font-medium">Memory</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="border-panel-border border-t">
              <td className="px-2 py-2">
                <Status s={r} />
                <div className="text-muted text-xs">{when(r.created_at)}</div>
              </td>
              <td className="px-2 py-2">
                {languageLabel[r.language] ?? r.language}
              </td>
              <td className="px-2 py-2 font-mono">
                {r.verdict && r.verdict.verdict !== "IE"
                  ? `${r.verdict.runtime_ms} ms`
                  : "N/A"}
              </td>
              <td className="px-2 py-2 font-mono">
                {r.verdict && r.verdict.verdict !== "IE"
                  ? memory(r.verdict.memory_kb)
                  : "N/A"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
