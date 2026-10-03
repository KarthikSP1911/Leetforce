import type { Difficulty } from "@/types/problem";

const fieldClass =
  "bg-panel border-panel-border text-foreground h-9 rounded-lg border px-3 text-sm";

// A plain GET form: works without JavaScript and is fully keyboard accessible.
export function ProblemFilters({
  q,
  difficulty,
  tag,
  tags,
}: {
  q: string;
  difficulty?: Difficulty;
  tag?: string;
  tags: string[];
}) {
  return (
    <form
      method="get"
      action="/problems"
      role="search"
      className="mb-4 flex flex-wrap items-end gap-3"
    >
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted">Search</span>
        <input
          type="search"
          name="q"
          defaultValue={q}
          placeholder="Search problems"
          className={`${fieldClass} w-56`}
        />
      </label>
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted">Difficulty</span>
        <select
          name="difficulty"
          defaultValue={difficulty ?? ""}
          className={fieldClass}
        >
          <option value="">All</option>
          <option value="easy">Easy</option>
          <option value="medium">Medium</option>
          <option value="hard">Hard</option>
        </select>
      </label>
      <label className="flex flex-col gap-1 text-xs">
        <span className="text-muted">Tag</span>
        <select name="tag" defaultValue={tag ?? ""} className={fieldClass}>
          <option value="">All</option>
          {tags.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </label>
      <button
        type="submit"
        className="bg-primary h-9 rounded-lg px-4 text-sm font-semibold text-white"
      >
        Apply
      </button>
      <a
        href="/problems"
        className="text-muted hover:text-primary h-9 content-center text-sm"
      >
        Reset
      </a>
    </form>
  );
}
