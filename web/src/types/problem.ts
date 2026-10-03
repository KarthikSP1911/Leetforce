export type Difficulty = "easy" | "medium" | "hard";

export type Language = "python" | "cpp" | "java" | "go";

export interface ProblemSummary {
  slug: string;
  title: string;
  difficulty: Difficulty;
  tags: string[];
  /** Percent 0-100, or null when nobody has submitted yet. */
  acceptance: number | null;
}

export interface ProblemList {
  problems: ProblemSummary[];
  total: number;
  page: number;
  page_size: number;
}

export interface Sample {
  name?: string;
  input: string;
  output: string;
}

export interface ProblemDetail extends ProblemSummary {
  statement: string;
  starters: Partial<Record<Language, string>>;
  samples: Sample[];
}

export interface ProblemQuery {
  q?: string;
  difficulty?: Difficulty;
  tag?: string;
  page?: number;
  page_size?: number;
}
