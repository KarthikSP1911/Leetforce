import Link from "next/link";

export type Difficulty = "Easy" | "Medium" | "Hard";

export interface ProblemRow {
  slug: string;
  title: string;
  difficulty: Difficulty;
  acceptance: number;
  tags: string[];
}

const difficultyClass: Record<Difficulty, string> = {
  Easy: "text-success",
  Medium: "text-warning",
  Hard: "text-danger",
};

export function ProblemTable({ problems }: { problems: ProblemRow[] }) {
  return (
    <div className="bg-panel border-panel-border overflow-hidden rounded-lg border">
      <table className="w-full text-left text-sm">
        <thead className="border-panel-border text-muted border-b text-xs tracking-wide uppercase">
          <tr>
            <th className="w-12 px-4 py-3 font-medium">Status</th>
            <th className="px-4 py-3 font-medium">Title</th>
            <th className="hidden px-4 py-3 font-medium sm:table-cell">Tags</th>
            <th className="w-28 px-4 py-3 font-medium">Acceptance</th>
            <th className="w-28 px-4 py-3 font-medium">Difficulty</th>
          </tr>
        </thead>
        <tbody>
          {problems.map((p, i) => (
            <tr
              key={p.slug}
              className="border-panel-border hover:bg-hover border-b transition-colors last:border-b-0"
            >
              <td className="text-muted px-4 py-3" aria-label="Not attempted">
                {"–"}
              </td>
              <td className="px-4 py-3 font-medium">
                <Link
                  href={`/problems/${p.slug}`}
                  className="hover:text-primary"
                >
                  {i + 1}. {p.title}
                </Link>
              </td>
              <td className="text-muted hidden px-4 py-3 sm:table-cell">
                {p.tags.join(", ")}
              </td>
              <td className="px-4 py-3 font-mono">
                {p.acceptance.toFixed(1)}%
              </td>
              <td
                className={`px-4 py-3 font-semibold ${difficultyClass[p.difficulty]}`}
              >
                {p.difficulty}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
