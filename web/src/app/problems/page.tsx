import { ProblemTable } from "@/components/problems/ProblemTable";
import { sampleProblems } from "@/lib/sample-problems";

export const metadata = { title: "Problems | LeetForce" };

export default function ProblemsPage() {
  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <h1 className="text-2xl font-bold">Problems</h1>
          <p className="text-muted mt-1">
            Sample data. Real problems arrive with the API.
          </p>
        </div>
      </div>
      <ProblemTable problems={sampleProblems} />
    </main>
  );
}
