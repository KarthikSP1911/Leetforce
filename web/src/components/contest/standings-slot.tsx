"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getStandings } from "@/lib/api/client";
import type { StandingCell, Standings } from "@/types/leaderboard";

const POLL_MS = 10_000;

function Cell({ cell }: { cell: StandingCell | undefined }) {
  if (!cell || (!cell.solved && cell.attempts === 0)) return null;
  if (cell.solved) {
    return (
      <span className="text-success font-semibold">
        <span aria-hidden="true">{"✓ "}</span>
        <span className="sr-only">Solved: </span>
        {cell.minutes} min
        {cell.attempts > 1 && (
          <span className="text-muted font-normal"> ({cell.attempts})</span>
        )}
      </span>
    );
  }
  return (
    <span className="text-danger font-semibold">
      <span className="sr-only">Unsolved, attempts: </span>-{cell.attempts}
    </span>
  );
}

export function StandingsTable({ contestSlug }: { contestSlug: string }) {
  const [data, setData] = useState<Standings | null>(null);
  const [error, setError] = useState("");
  const [updated, setUpdated] = useState("");
  const ctrl = useRef<AbortController | null>(null);

  const load = useCallback(async () => {
    ctrl.current?.abort();
    const c = new AbortController();
    ctrl.current = c;
    try {
      const s = await getStandings(contestSlug, c.signal);
      if (c.signal.aborted) return;
      setData(s);
      setError("");
      setUpdated(new Date().toLocaleTimeString([], { hour12: false }));
    } catch (e) {
      if (c.signal.aborted) return;
      setError(e instanceof Error ? e.message : "Could not load standings.");
    }
  }, [contestSlug]);

  useEffect(() => {
    const first = setTimeout(() => void load(), 0);
    const id = setInterval(() => {
      if (!document.hidden) void load();
    }, POLL_MS);
    return () => {
      clearTimeout(first);
      clearInterval(id);
      ctrl.current?.abort();
    };
  }, [load]);

  return (
    <section aria-label="Standings">
      <div className="mb-2 flex items-center justify-between">
        <p aria-live="polite" className="text-muted text-sm">
          {updated ? `Updated ${updated}` : ""}
        </p>
        <button
          type="button"
          onClick={() => void load()}
          className="bg-panel border-panel-border hover:bg-hover rounded-lg border px-3 py-1.5 text-sm"
        >
          Refresh
        </button>
      </div>
      {error && (
        <p
          role="alert"
          className="bg-panel border-panel-border text-danger mb-2 rounded-lg border px-4 py-3"
        >
          {error}
        </p>
      )}
      {!data && !error && (
        <p className="text-muted text-sm">Loading standings…</p>
      )}
      {data && (
        <div className="bg-panel border-panel-border overflow-x-auto rounded-lg border">
          <table className="w-full text-left text-sm">
            <caption className="sr-only">
              Standings for {data.contest.title}
            </caption>
            <thead className="border-panel-border text-muted border-b text-xs tracking-wide uppercase">
              <tr>
                <th scope="col" className="px-4 py-3 font-medium">
                  Rank
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  User
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  Solved
                </th>
                <th scope="col" className="px-4 py-3 font-medium">
                  Penalty
                </th>
                {data.problems.map((p) => (
                  <th
                    key={p}
                    scope="col"
                    className="px-4 py-3 font-medium whitespace-nowrap"
                  >
                    {p}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {data.standings.length === 0 && (
                <tr>
                  <td
                    colSpan={4 + data.problems.length}
                    className="text-muted px-4 py-8 text-center"
                  >
                    No participants yet.
                  </td>
                </tr>
              )}
              {data.standings.map((r) => (
                <tr
                  key={r.user_id}
                  className="border-panel-border hover:bg-hover border-b last:border-b-0"
                >
                  <td className="px-4 py-3 font-mono">{r.rank}</td>
                  <th scope="row" className="px-4 py-3 font-medium">
                    {r.username}
                  </th>
                  <td className="px-4 py-3 font-mono">{r.solved}</td>
                  <td className="px-4 py-3 font-mono">{r.penalty_minutes}</td>
                  {data.problems.map((p) => (
                    <td
                      key={p}
                      className="px-4 py-3 font-mono whitespace-nowrap"
                    >
                      <Cell cell={r.cells[p]} />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
