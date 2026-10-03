import type { Difficulty } from "./problem";

export type ContestStatus = "upcoming" | "running" | "ended";

export interface ContestSummary {
  slug: string;
  title: string;
  starts_at: string;
  ends_at: string;
  status: ContestStatus;
  registered: boolean;
  problem_count: number;
}

export interface ContestDetail {
  slug: string;
  title: string;
  starts_at: string;
  ends_at: string;
  status: ContestStatus;
  registered: boolean;
  /** RFC 3339 server clock, used to correct client clock skew. */
  server_time: string;
}

export interface ContestProblem {
  label: string;
  position: number;
  points: number;
  slug: string;
  title: string;
  difficulty: Difficulty;
}
