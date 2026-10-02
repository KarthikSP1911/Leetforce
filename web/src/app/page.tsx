import Image from "next/image";

export default function Home() {
  return (
    <main className="flex flex-1 flex-col items-center justify-center gap-4">
      <Image
        src="/brand/logo.svg"
        alt="LeetForce logo"
        width={96}
        height={96}
        className="rounded-lg"
        priority
      />
      <h1 className="text-3xl font-bold">LeetForce</h1>
      <p className="text-muted">Distributed, sandboxed code execution.</p>
    </main>
  );
}
