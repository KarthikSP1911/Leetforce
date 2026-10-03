import { Pagination } from "@/components/problems/Pagination";
import { ProblemFilters } from "@/components/problems/ProblemFilters";
import { ProblemTable } from "@/components/problems/ProblemTable";
import { ApiError, listProblems } from "@/lib/api/client";
import type { Difficulty, ProblemList } from "@/types/problem";

export const metadata = { title: "Problems | LeetForce" };

const PAGE_SIZE = 20;
const difficulties: readonly string[] = ["easy", "medium", "hard"];

type Search = Record<string, string | string[] | undefined>;

function first(v: string | string[] | undefined): string {
  return (Array.isArray(v) ? v[0] : v) ?? "";
}

export default async function ProblemsPage({
  searchParams,
}: {
  searchParams: Promise<Search>;
}) {
  const sp = await searchParams;
  const q = first(sp.q).trim();
  const d = first(sp.difficulty);
  const difficulty = difficulties.includes(d) ? (d as Difficulty) : undefined;
  const tag = first(sp.tag) || undefined;
  const page = Math.max(1, Number.parseInt(first(sp.page), 10) || 1);

  let list: ProblemList | null = null;
  let tags: string[] = [];
  let error = "";
  try {
    const [result, all] = await Promise.all([
      listProblems({ q, difficulty, tag, page, page_size: PAGE_SIZE }),
      listProblems({ page_size: 100 }),
    ]);
    list = result;
    tags = [...new Set(all.problems.flatMap((p) => p.tags))].sort();
  } catch (e) {
    error =
      e instanceof ApiError
        ? `The API returned an error: ${e.message}`
        : "Could not reach the API.";
  }

  const hrefFor = (p: number) => {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    if (difficulty) params.set("difficulty", difficulty);
    if (tag) params.set("tag", tag);
    params.set("page", String(p));
    return `/problems?${params.toString()}`;
  };

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <h1 className="mb-6 text-2xl font-bold">Problems</h1>
      <ProblemFilters q={q} difficulty={difficulty} tag={tag} tags={tags} />
      {error ? (
        <p
          role="alert"
          className="bg-panel border-panel-border text-danger rounded-lg border px-4 py-3"
        >
          {error}
        </p>
      ) : (
        list && (
          <>
            <ProblemTable
              problems={list.problems}
              offset={(list.page - 1) * list.page_size}
            />
            <Pagination
              page={list.page}
              pageCount={Math.max(1, Math.ceil(list.total / list.page_size))}
              hrefFor={hrefFor}
            />
          </>
        )
      )}
    </main>
  );
}
