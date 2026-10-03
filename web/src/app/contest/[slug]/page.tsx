import type { Metadata } from "next";
import { cookies } from "next/headers";
import { notFound } from "next/navigation";
import { ContestView } from "@/components/contest/ContestView";
import { ApiError, getContest } from "@/lib/api/client";
import type { ContestDetail } from "@/types/contest";

type Params = Promise<{ slug: string }>;

async function load(slug: string): Promise<ContestDetail> {
  const session = (await cookies()).get("lf_session");
  const cookie = session
    ? `lf_session=${encodeURIComponent(session.value)}`
    : undefined;
  try {
    return await getContest(slug, undefined, { cookie });
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
  const c = await load(slug);
  return { title: `${c.title} | LeetForce` };
}

export default async function ContestPage({ params }: { params: Params }) {
  const { slug } = await params;
  return <ContestView initial={await load(slug)} />;
}
