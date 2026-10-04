import Link from "next/link";
import { Select } from "@/components/ui/Select";
import type { Difficulty } from "@/types/problem";

const fieldClass =
  "bg-panel border-panel-border text-foreground h-9 rounded-lg border px-3 text-sm";

// A plain GET form: the custom dropdowns keep a hidden input, so the filters
// are still submitted as query parameters.
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
      <div className="flex flex-col gap-1 text-xs">
        <span className="text-muted">Difficulty</span>
        <Select
          name="difficulty"
          label="Difficulty"
          defaultValue={difficulty ?? ""}
          className="w-36"
          options={[
            { value: "", label: "All" },
            { value: "easy", label: "Easy" },
            { value: "medium", label: "Medium" },
            { value: "hard", label: "Hard" },
          ]}
        />
      </div>
      <div className="flex flex-col gap-1 text-xs">
        <span className="text-muted">Tag</span>
        <Select
          name="tag"
          label="Tag"
          defaultValue={tag ?? ""}
          className="w-40"
          options={[
            { value: "", label: "All" },
            ...tags.map((t) => ({ value: t, label: t })),
          ]}
        />
      </div>
      <button
        type="submit"
        className="bg-primary h-9 rounded-lg px-4 text-sm font-semibold text-white"
      >
        Apply
      </button>
      <Link
        href="/problems"
        className="text-muted hover:text-link h-9 content-center text-sm"
      >
        Reset
      </Link>
    </form>
  );
}
