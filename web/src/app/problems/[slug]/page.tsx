import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { Workspace } from "@/components/workspace/Workspace";
import { ApiError, getProblem } from "@/lib/api/client";
import type { ProblemDetail } from "@/types/problem";

type Params = Promise<{ slug: string }>;

async function load(slug: string): Promise<ProblemDetail> {
  try {
    return await getProblem(slug);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) notFound();
    throw e;
  }
}

export async function generateMetadata({
  params,
}: {
  params: Params;
}): Promise<Metadata> {
  const { slug } = await params;
  const p = await load(slug);
  return { title: `${p.title} | LeetForce` };
}

export default async function ProblemPage({
  params,
  searchParams,
}: {
  params: Params;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { slug } = await params;
  const c = (await searchParams).contest;
  const contest = (Array.isArray(c) ? c[0] : c) || undefined;
  return <Workspace problem={await load(slug)} contest={contest} />;
}
