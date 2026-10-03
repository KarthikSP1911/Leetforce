import type { Language } from "./problem";

export type SubmissionStatus = "queued" | "judging" | "judged";

/** Verdict codes the judge reports for Submit; IE means the platform failed. */
export type Verdict = "AC" | "WA" | "TLE" | "MLE" | "RE" | "CE" | "OLE" | "IE";

/** What a user may see about a judged submission: never test data or stderr. */
export interface VerdictView {
  verdict: Verdict;
  runtime_ms: number;
  memory_kb: number;
  passed: number;
  total: number;
}

export interface Submission {
  id: string;
  problem: string;
  language: Language;
  status: SubmissionStatus;
  created_at: string;
  verdict?: VerdictView;
}

export interface SubmissionCreated {
  id: string;
  status: SubmissionStatus;
}

export type RunStatus = "queued" | "judging" | "done";

/** One sample test in a Run result; detail fields appear only on failure. */
export interface RunCase {
  name: string;
  verdict: Verdict;
  input?: string;
  expected?: string;
  actual?: string;
  stderr?: string;
}

/** OK means a custom-input run ended cleanly (nothing to compare against). */
export interface RunResult {
  verdict: Verdict | "OK";
  runtime_ms: number;
  memory_kb: number;
  compile_output?: string;
  cases?: RunCase[];
  stdout?: string;
  stderr?: string;
}

export interface RunState {
  status: RunStatus;
  result?: RunResult;
}

export interface RunCreated {
  id: string;
  status: RunStatus;
}
