import Link from "next/link";
import { cookies } from "next/headers";
import { formatTime, StatusBadge } from "@/components/contest/StatusBadge";
import { ApiError, listContests } from "@/lib/api/client";
import type { ContestSummary } from "@/types/contest";

export const metadata = { title: "Contests | LeetForce" };

export default async function ContestsPage() {
  let contests: ContestSummary[] = [];
  let error = "";
  const session = (await cookies()).get("lf_session");
  const cookie = session
    ? `lf_session=${encodeURIComponent(session.value)}`
    : undefined;
  try {
    contests = await listContests(undefined, { cookie });
  } catch (e) {
    error =
      e instanceof ApiError
        ? `The API returned an error: ${e.message}`
        : "Could not reach the API.";
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <h1 className="mb-6 text-2xl font-bold">Contests</h1>
      {error ? (
        <p
          role="alert"
          className="bg-panel border-panel-border text-danger rounded-lg border px-4 py-3"
        >
          {error}
        </p>
      ) : (
        <div className="bg-panel border-panel-border overflow-hidden rounded-lg border">
          <table className="w-full text-left text-sm">
            <thead className="border-panel-border text-muted border-b text-xs tracking-wide uppercase">
              <tr>
                <th className="px-4 py-3 font-medium">Contest</th>
                <th className="hidden px-4 py-3 font-medium sm:table-cell">
                  Starts
                </th>
                <th className="hidden px-4 py-3 font-medium md:table-cell">
                  Ends
                </th>
                <th className="w-28 px-4 py-3 font-medium">Status</th>
                <th className="w-36 px-4 py-3 font-medium">Registration</th>
              </tr>
            </thead>
            <tbody>
              {contests.length === 0 && (
                <tr>
                  <td colSpan={5} className="text-muted px-4 py-8 text-center">
                    No contests yet.
                  </td>
                </tr>
              )}
              {contests.map((c) => (
                <tr
                  key={c.slug}
                  className="border-panel-border hover:bg-hover border-b transition-colors last:border-b-0"
                >
                  <td className="px-4 py-3 font-medium">
                    <Link
                      href={`/contest/${c.slug}`}
                      className="hover:text-link"
                    >
                      {c.title}
                    </Link>
                    <span className="text-muted ml-2 text-xs">
                      {c.problem_count} problems
                    </span>
                  </td>
                  <td className="text-muted hidden px-4 py-3 sm:table-cell">
                    {formatTime(c.starts_at)}
                  </td>
                  <td className="text-muted hidden px-4 py-3 md:table-cell">
                    {formatTime(c.ends_at)}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={c.status} />
                  </td>
                  <td
                    className={`px-4 py-3 ${c.registered ? "text-success font-semibold" : "text-muted"}`}
                  >
                    {c.registered ? "Registered" : "Not registered"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
