import type { ProblemRow } from "@/components/problems/ProblemTable";

// Placeholder rows until the problems API exists (Phase 4). Original titles only.
export const sampleProblems: ProblemRow[] = [
  {
    slug: "pair-sum",
    title: "Pair Sum",
    difficulty: "Easy",
    acceptance: 52.4,
    tags: ["Array", "Hash Table"],
  },
  {
    slug: "balanced-brackets",
    title: "Balanced Brackets",
    difficulty: "Easy",
    acceptance: 44.1,
    tags: ["Stack", "String"],
  },
  {
    slug: "longest-unique-run",
    title: "Longest Unique Run",
    difficulty: "Medium",
    acceptance: 36.8,
    tags: ["String", "Sliding Window"],
  },
  {
    slug: "meeting-rooms-needed",
    title: "Meeting Rooms Needed",
    difficulty: "Medium",
    acceptance: 48.3,
    tags: ["Heap", "Sorting"],
  },
  {
    slug: "shortest-signal-path",
    title: "Shortest Signal Path",
    difficulty: "Hard",
    acceptance: 28.9,
    tags: ["Graph", "BFS"],
  },
];
