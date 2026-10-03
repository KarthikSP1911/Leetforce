import { Pagination } from "@/components/problems/Pagination";
import { ApiError, getLeaderboard } from "@/lib/api/client";
import type { Leaderboard } from "@/types/leaderboard";

export const metadata = { title: "Leaderboard | LeetForce" };

const PER_PAGE = 50;

type Search = Record<string, string | string[] | undefined>;

export default async function LeaderboardPage({
  searchParams,
}: {
  searchParams: Promise<Search>;
}) {
  const sp = await searchParams;
  const raw = Array.isArray(sp.page) ? sp.page[0] : sp.page;
  const page = Math.max(1, Number.parseInt(raw ?? "", 10) || 1);

  let board: Leaderboard | null = null;
  let error = "";
  try {
    board = await getLeaderboard(page, PER_PAGE);
  } catch (e) {
    error =
      e instanceof ApiError
        ? `The API returned an error: ${e.message}`
        : "Could not reach the API.";
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <h1 className="mb-6 text-2xl font-bold">Leaderboard</h1>
      {error ? (
        <p
          role="alert"
          className="bg-panel border-panel-border text-danger rounded-lg border px-4 py-3"
        >
          {error}
        </p>
      ) : (
        board && (
          <>
            <div className="bg-panel border-panel-border overflow-hidden rounded-lg border">
              <table className="w-full text-left text-sm">
                <caption className="sr-only">Ranked users by score</caption>
                <thead className="border-panel-border text-muted border-b text-xs tracking-wide uppercase">
                  <tr>
                    <th scope="col" className="w-20 px-4 py-3 font-medium">
                      Rank
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      User
                    </th>
                    <th scope="col" className="px-4 py-3 font-medium">
                      Solved
                    </th>
                    <th
                      scope="col"
                      className="hidden px-4 py-3 font-medium sm:table-cell"
                    >
                      Easy / Medium / Hard
                    </th>
                    <th scope="col" className="w-24 px-4 py-3 font-medium">
                      Score
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {board.entries.length === 0 && (
                    <tr>
                      <td
                        colSpan={5}
                        className="text-muted px-4 py-8 text-center"
                      >
                        No one has solved a problem yet.
                      </td>
                    </tr>
                  )}
                  {board.entries.map((e) => (
                    <tr
                      key={e.user_id}
                      className="border-panel-border hover:bg-hover border-b transition-colors last:border-b-0"
                    >
                      <td className="px-4 py-3 font-mono">{e.rank}</td>
                      <td className="px-4 py-3 font-medium">{e.username}</td>
                      <td className="px-4 py-3 font-mono">{e.solved}</td>
                      <td className="hidden px-4 py-3 font-mono sm:table-cell">
                        <span className="text-success font-semibold">
                          E {e.easy}
                        </span>
                        {" / "}
                        <span className="text-warning font-semibold">
                          M {e.medium}
                        </span>
                        {" / "}
                        <span className="text-danger font-semibold">
                          H {e.hard}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-mono">{e.score}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination
              page={board.page}
              pageCount={Math.max(1, Math.ceil(board.total / PER_PAGE))}
              hrefFor={(p) => `/leaderboard?page=${p}`}
            />
          </>
        )
      )}
    </main>
  );
}
