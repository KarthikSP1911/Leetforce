import type {
  RunResult,
  SubmissionStatus,
  Verdict,
  VerdictView,
} from "@/types/submission";

/** What the Result tab shows. Submit and Run are different shapes on purpose. */
export type ConsoleResult =
  | { kind: "idle" }
  | { kind: "pending"; action: "run" | "submit"; status: SubmissionStatus }
  | { kind: "submit"; verdict: VerdictView }
  | { kind: "run"; result: RunResult }
  | { kind: "error"; message: string };

type Label = Verdict | "OK";

const verdictLabel: Record<Label, string> = {
  AC: "Accepted",
  WA: "Wrong Answer",
  TLE: "Time Limit Exceeded",
  MLE: "Memory Limit Exceeded",
  RE: "Runtime Error",
  CE: "Compile Error",
  OLE: "Output Limit Exceeded",
  IE: "Internal Error",
  OK: "Ran Successfully",
};

// The label is always shown; color only reinforces it.
const verdictClass: Record<Label, string> = {
  AC: "text-success",
  OK: "text-success",
  TLE: "text-warning",
  MLE: "text-warning",
  OLE: "text-warning",
  WA: "text-danger",
  RE: "text-danger",
  CE: "text-danger",
  IE: "text-muted",
};

function formatMemory(kb: number): string {
  return kb >= 1024 ? `${(kb / 1024).toFixed(1)} MB` : `${kb} KB`;
}

function Block({ title, text }: { title: string; text: string }) {
  return (
    <div className="mt-2">
      <p className="text-muted text-xs">{title}</p>
      <pre className="bg-hover border-panel-border mt-1 max-h-40 overflow-auto rounded-lg border p-2 font-mono text-xs whitespace-pre-wrap">
        {text === "" ? " " : text}
      </pre>
    </div>
  );
}

function VerdictHeading({ label }: { label: Label }) {
  return (
    <h2 className={`text-2xl font-bold ${verdictClass[label]}`}>
      {verdictLabel[label] ?? label}
    </h2>
  );
}

function Stats({
  runtimeMs,
  memoryKb,
}: {
  runtimeMs: number;
  memoryKb: number;
}) {
  return (
    <p className="text-muted mt-1 font-mono text-sm">
      Runtime: {runtimeMs} ms · Memory: {formatMemory(memoryKb)}
    </p>
  );
}

// Submit: the verdict, runtime, memory and a pass count. Never the input,
// expected output or stderr of any test: hidden tests stay hidden.
function SubmitResult({ v }: { v: VerdictView }) {
  return (
    <div>
      <VerdictHeading label={v.verdict} />
      {v.verdict !== "IE" && (
        <Stats runtimeMs={v.runtime_ms} memoryKb={v.memory_kb} />
      )}
      {v.total > 0 && v.verdict !== "CE" && (
        <p className="text-muted mt-1 text-sm">
          {v.passed} / {v.total} test cases passed
        </p>
      )}
    </div>
  );
}

// Run: samples (or the user's own input) only, so details are safe to show.
function RunOutcome({ r }: { r: RunResult }) {
  return (
    <div>
      <VerdictHeading label={r.verdict} />
      {r.verdict !== "IE" && r.verdict !== "CE" && (
        <Stats runtimeMs={r.runtime_ms} memoryKb={r.memory_kb} />
      )}
      {r.compile_output && (
        <Block title="Compiler output" text={r.compile_output} />
      )}
      {r.stdout !== undefined && <Block title="Output" text={r.stdout} />}
      {r.stderr && <Block title="Stderr" text={r.stderr} />}
      {r.cases && r.cases.length > 0 && (
        <ul className="mt-3 space-y-3">
          {r.cases.map((c) => (
            <li key={c.name}>
              <p className="text-sm font-semibold">
                Sample {c.name}:{" "}
                <span className={verdictClass[c.verdict]}>
                  {verdictLabel[c.verdict] ?? c.verdict}
                </span>
              </p>
              {c.verdict !== "AC" && c.input !== undefined && (
                <>
                  <Block title="Input" text={c.input} />
                  <Block title="Expected" text={c.expected ?? ""} />
                  <Block title="Your output" text={c.actual ?? ""} />
                  {c.stderr && <Block title="Stderr" text={c.stderr} />}
                </>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function ResultPanel({ result }: { result: ConsoleResult }) {
  switch (result.kind) {
    case "idle":
      return (
        <p className="text-muted p-3 text-sm">
          Run or submit your code to see the result here.
        </p>
      );
    case "pending":
      return (
        <div className="p-3" role="status" aria-live="polite">
          <p className="flex items-center gap-2 text-lg font-semibold">
            <span
              aria-hidden
              className="bg-accent inline-block h-2.5 w-2.5 animate-pulse rounded-full"
            />
            {result.status === "judging"
              ? "Judging…"
              : result.action === "run"
                ? "Queued, running soon…"
                : "Queued…"}
          </p>
        </div>
      );
    case "error":
      return (
        <p className="text-danger p-3 text-sm" role="alert">
          {result.message}
        </p>
      );
    case "submit":
      return (
        <div className="p-3" role="status" aria-live="polite">
          <SubmitResult v={result.verdict} />
        </div>
      );
    case "run":
      return (
        <div className="p-3" role="status" aria-live="polite">
          <RunOutcome r={result.result} />
        </div>
      );
  }
}
