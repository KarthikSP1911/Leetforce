"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { ConsoleResult } from "@/components/workspace/ResultPanel";
import { ApiError, createRun, createSubmission } from "@/lib/api/client";
import { watchRun, watchSubmission } from "@/lib/api/watch";
import type { Language } from "@/types/problem";

function message(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401) return "Sign in to run or submit code.";
    if (e.status === 413) return "Your code or input is too large.";
    if (e.status === 429) {
      return e.retryAfter
        ? `Too many requests. Try again in ${e.retryAfter} seconds.`
        : "Too many requests. Wait a moment.";
    }
    return e.message || "The request failed.";
  }
  return "Could not reach the server. Try again.";
}

/**
 * Drives Run and Submit for one problem. At most one is active at a time: a new
 * one stops watching the old one. `onSubmission` fires when a submission is
 * created and again when it gets its verdict, so the Submissions tab can refresh.
 */
export function useJudge(problem: string, onSubmission?: () => void) {
  const [result, setResult] = useState<ConsoleResult>({ kind: "idle" });
  const [busy, setBusy] = useState(false);
  const stop = useRef<(() => void) | undefined>(undefined);
  const changed = useRef(onSubmission);
  useEffect(() => {
    changed.current = onSubmission;
  });
  useEffect(() => () => stop.current?.(), []);

  const begin = useCallback((action: "run" | "submit") => {
    stop.current?.();
    stop.current = undefined;
    setBusy(true);
    setResult({ kind: "pending", action, status: "queued" });
  }, []);

  const fail = useCallback((msg: string, signIn?: boolean) => {
    setBusy(false);
    setResult({ kind: "error", message: msg, signIn });
  }, []);

  const run = useCallback(
    async (language: Language, source: string, input?: string) => {
      begin("run");
      try {
        const { id } = await createRun({ problem, language, source, input });
        stop.current = watchRun(id, {
          onUpdate: (s) => {
            if (s.status === "done" && s.result) {
              setBusy(false);
              setResult({ kind: "run", result: s.result });
            } else {
              setResult({
                kind: "pending",
                action: "run",
                status: s.status === "judging" ? "judging" : "queued",
              });
            }
          },
          onError: (m) => fail(m),
        });
      } catch (e) {
        fail(message(e), e instanceof ApiError && e.status === 401);
      }
    },
    [begin, fail, problem],
  );

  const submit = useCallback(
    async (language: Language, source: string) => {
      begin("submit");
      try {
        const { id } = await createSubmission({ problem, language, source });
        changed.current?.();
        stop.current = watchSubmission(id, {
          onUpdate: (s) => {
            if (s.status === "judged" && s.verdict) {
              setBusy(false);
              setResult({ kind: "submit", verdict: s.verdict });
              changed.current?.();
            } else {
              setResult({
                kind: "pending",
                action: "submit",
                status: s.status,
              });
            }
          },
          onError: (m) => fail(m),
        });
      } catch (e) {
        fail(message(e), e instanceof ApiError && e.status === 401);
      }
    },
    [begin, fail, problem],
  );

  return { result, busy, run, submit };
}
