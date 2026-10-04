export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username: string;
  solved: number;
  easy: number;
  medium: number;
  hard: number;
  score: number;
  /** RFC3339, or null when the user has no accepted submission. */
  last_ac_at: string | null;
}

export interface Leaderboard {
  page: number;
  per_page: number;
  total: number;
  generated_at: string;
  entries: LeaderboardEntry[];
}

export interface StandingCell {
  solved: boolean;
  attempts: number;
  minutes: number;
}

export interface StandingRow {
  rank: number;
  user_id: string;
  username: string;
  solved: number;
  penalty_minutes: number;
  last_ac_at: string | null;
  /** Keyed by problem slug; a missing cell means untouched. */
  cells: Record<string, StandingCell | undefined>;
}

export interface Standings {
  contest: {
    slug: string;
    title: string;
    starts_at: string;
    ends_at: string;
  };
  generated_at: string;
  problems: string[];
  standings: StandingRow[];
}
