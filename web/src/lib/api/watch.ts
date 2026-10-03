import { ApiError, getRun, getSubmission } from "./client";
import type { RunState, Submission } from "@/types/submission";

const POLL_MS = 1000;
// A submission or run still unfinished after this long is reported as an error
// instead of spinning forever (the API's own stream ends at 10 minutes).
const GIVE_UP_MS = 2 * 60 * 1000;

export interface Watch {
  /** Called each time the status changes, and once with the final state. */
  onUpdate: (s: Submission) => void;
  onError: (message: string) => void;
}

/**
 * Follows a submission: queued, judging, then the verdict. It uses the API's
 * Server-Sent Events stream and falls back to polling GET /submissions/:id if
 * the stream cannot be opened or breaks (a proxy that buffers, a dropped
 * connection). Returns a function that stops watching.
 */
export function watchSubmission(id: string, w: Watch): () => void {
  let stopped = false;
  let source: EventSource | undefined;
  let pollTimer: ReturnType<typeof setTimeout> | undefined;
  let polling = false;
  const started = Date.now();

  const finish = () => {
    stopped = true;
    source?.close();
    if (pollTimer) clearTimeout(pollTimer);
  };

  const poll = async () => {
    if (stopped) return;
    try {
      const s = await getSubmission(id);
      if (stopped) return;
      w.onUpdate(s);
      if (s.status === "judged") return finish();
    } catch {
      // transient: try again until the deadline
    }
    if (Date.now() - started > GIVE_UP_MS) {
      finish();
      w.onError("Timed out waiting for the verdict. Check Submissions later.");
      return;
    }
    pollTimer = setTimeout(poll, POLL_MS);
  };

  const fallBack = () => {
    source?.close();
    source = undefined;
    if (!stopped && !polling) {
      polling = true;
      void poll();
    }
  };

  if (typeof EventSource === "undefined") {
    fallBack();
    return finish;
  }
  source = new EventSource(`/api/submissions/${encodeURIComponent(id)}/events`);
  // The event payload is {status, verdict?}; the caller already knows the rest.
  const apply = (e: Event, final: boolean) => {
    try {
      const body = JSON.parse((e as MessageEvent<string>).data) as Pick<
        Submission,
        "status" | "verdict"
      >;
      w.onUpdate({ id, ...body } as Submission);
    } catch {
      return fallBack();
    }
    if (final) finish();
  };
  source.addEventListener("status", (e) => apply(e, false));
  source.addEventListener("verdict", (e) => apply(e, true));
  // "timeout" and "error" are named server events; a network drop fires the
  // built-in error event. Either way, polling takes over.
  source.addEventListener("timeout", fallBack);
  source.addEventListener("error", fallBack);
  return finish;
}

export interface RunWatch {
  onUpdate: (s: RunState) => void;
  onError: (message: string) => void;
}

/** Polls a Run until it is done. Runs are short, so there is no stream. */
export function watchRun(id: string, w: RunWatch): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const started = Date.now();
  const tick = async () => {
    if (stopped) return;
    try {
      const s = await getRun(id);
      if (stopped) return;
      w.onUpdate(s);
      if (s.status === "done") {
        stopped = true;
        return;
      }
    } catch (e) {
      // 404 means the run expired or never existed: stop, do not poll forever.
      if (e instanceof ApiError && e.status === 404) {
        stopped = true;
        w.onError("The run expired. Run again.");
        return;
      }
    }
    if (Date.now() - started > GIVE_UP_MS) {
      stopped = true;
      w.onError("Timed out waiting for the run. Try again.");
      return;
    }
    timer = setTimeout(tick, 500);
  };
  void tick();
  return () => {
    stopped = true;
    if (timer) clearTimeout(timer);
  };
}
