import Link from "next/link";
import type { Difficulty, ProblemSummary } from "@/types/problem";

const difficultyClass: Record<Difficulty, string> = {
  easy: "text-success",
  medium: "text-warning",
  hard: "text-danger",
};

const difficultyLabel: Record<Difficulty, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
};

export function ProblemTable({
  problems,
  offset,
}: {
  problems: ProblemSummary[];
  offset: number;
}) {
  return (
    <div className="bg-panel border-panel-border overflow-hidden rounded-lg border">
      <table className="w-full text-left text-sm">
        <thead className="border-panel-border text-muted border-b text-xs tracking-wide uppercase">
          <tr>
            <th className="w-16 px-4 py-3 font-medium">#</th>
            <th className="px-4 py-3 font-medium">Title</th>
            <th className="hidden px-4 py-3 font-medium sm:table-cell">Tags</th>
            <th className="w-28 px-4 py-3 font-medium">Acceptance</th>
            <th className="w-28 px-4 py-3 font-medium">Difficulty</th>
          </tr>
        </thead>
        <tbody>
          {problems.length === 0 && (
            <tr>
              <td colSpan={5} className="text-muted px-4 py-8 text-center">
                No problems match these filters.
              </td>
            </tr>
          )}
          {problems.map((p, i) => (
            <tr
              key={p.slug}
              className="border-panel-border hover:bg-hover border-b transition-colors last:border-b-0"
            >
              <td className="text-muted px-4 py-3 font-mono">
                {offset + i + 1}
              </td>
              <td className="px-4 py-3 font-medium">
                <Link href={`/problems/${p.slug}`} className="hover:text-link">
                  {p.title}
                </Link>
              </td>
              <td className="text-muted hidden px-4 py-3 sm:table-cell">
                {p.tags.join(", ")}
              </td>
              <td className="px-4 py-3 font-mono">
                {p.acceptance === null ? "–" : `${p.acceptance.toFixed(1)}%`}
              </td>
              <td
                className={`px-4 py-3 font-semibold ${difficultyClass[p.difficulty]}`}
              >
                {difficultyLabel[p.difficulty]}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
