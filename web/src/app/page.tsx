import Link from "next/link";
import type { ReactNode } from "react";
import { CodeWindow } from "@/components/home/CodeWindow";
import { HeroGrid, PipelineDiagram } from "@/components/home/HomeArt";
import { ButtonLink, Enter, Reveal } from "@/components/motion/Motion";

export const metadata = {
  title: "LeetForce | Practice code, get a verdict in seconds",
};

const primaryBtn =
  "bg-primary inline-flex h-11 items-center justify-center rounded-lg px-6 text-sm font-semibold text-white transition-opacity hover:opacity-90";
const secondaryBtn =
  "bg-panel border-panel-border hover:bg-hover inline-flex h-11 items-center justify-center rounded-lg border px-6 text-sm font-semibold transition-colors";
// The hero uses slightly smaller buttons than the rest of the page.
const heroPrimaryBtn = primaryBtn
  .replace("h-11", "h-10")
  .replace("px-6", "px-5");
const heroSecondaryBtn = secondaryBtn
  .replace("h-11", "h-10")
  .replace("px-6", "px-5");

function Icon({ children }: { children: ReactNode }) {
  return (
    <span className="bg-hover border-panel-border text-link flex h-10 w-10 items-center justify-center rounded-lg border">
      <svg
        width="20"
        height="20"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        {children}
      </svg>
    </span>
  );
}

const features: { title: string; text: string; icon: ReactNode }[] = [
  {
    title: "Isolated sandbox",
    text: "Every submission runs in its own sandbox with CPU, memory and process limits. Runaway code is killed, never trusted.",
    icon: <path d="M12 3 4 6v6c0 5 3.4 8 8 9 4.6-1 8-4 8-9V6l-8-3Z" />,
  },
  {
    title: "Live verdicts",
    text: "Watch a submission move from queued to judging to a final verdict without refreshing the page.",
    icon: <path d="M3 12h4l3-8 4 16 3-8h4" />,
  },
  {
    title: "Python, C++, Java and Go",
    text: "Write in the language you like. Each one has its own compiler setup and starter code.",
    icon: (
      <>
        <path d="m8 7-5 5 5 5" />
        <path d="m16 7 5 5-5 5" />
        <path d="m14 4-4 16" />
      </>
    ),
  },
  {
    title: "Hidden tests stay hidden",
    text: "Run shows you sample results. Submit reports only the verdict, runtime and memory, never the secret test data.",
    icon: (
      <>
        <rect x="5" y="11" width="14" height="9" rx="2" />
        <path d="M8 11V8a4 4 0 0 1 8 0v3" />
      </>
    ),
  },
  {
    title: "Timed contests",
    text: "Register, race the clock and get ranked ICPC style: most problems solved, then the lowest penalty.",
    icon: (
      <>
        <circle cx="12" cy="13" r="8" />
        <path d="M12 9v4l2 2M9 2h6" />
      </>
    ),
  },
  {
    title: "Global leaderboard",
    text: "Rankings update as verdicts arrive, so you can see where you stand against everyone else.",
    icon: <path d="M6 20V10M12 20V4M18 20v-7" />,
  },
];

const steps = [
  { title: "Write", text: "Pick a problem and code in the browser editor." },
  {
    title: "Submit",
    text: "Your code is queued and sent to a judging runner.",
  },
  {
    title: "Judge",
    text: "It compiles and runs against every test in a sandbox.",
  },
  {
    title: "Verdict",
    text: "You get Accepted or the reason it failed, with time and memory.",
  },
];

export default function Home() {
  return (
    <main className="flex-1">
      <section className="border-panel-border relative overflow-hidden border-b">
        <HeroGrid />
        <div className="relative mx-auto grid w-full max-w-6xl items-stretch gap-12 px-4 pt-10 pb-16 lg:grid-cols-2 lg:pt-12 lg:pb-24">
          <Enter>
            <p className="text-link mb-4 text-sm font-semibold tracking-wide uppercase">
              Online judge
            </p>
            <h1 className="text-3xl leading-tight font-bold tracking-tight sm:text-4xl lg:text-[2.5rem]">
              Write code.
              <br />
              Get a verdict in seconds.
            </h1>
            <p className="text-muted mt-4 max-w-lg text-[15px] leading-7">
              LeetForce runs your solutions in isolated sandboxes and tells you
              exactly how they did: Accepted, Wrong Answer, Time Limit and more,
              with runtime and memory.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <ButtonLink href="/problems" className={heroPrimaryBtn}>
                Start solving
              </ButtonLink>
              <ButtonLink href="/contest" className={heroSecondaryBtn}>
                View contests
              </ButtonLink>
            </div>
            <ul className="text-muted mt-6 flex flex-wrap gap-x-6 gap-y-2 text-sm">
              <li>Python</li>
              <li>C++</li>
              <li>Java</li>
              <li>Go</li>
            </ul>
          </Enter>
          <Enter
            x={24}
            delay={0.15}
            className="flex lg:justify-end lg:self-stretch"
          >
            <CodeWindow />
          </Enter>
        </div>
      </section>

      <section className="mx-auto w-full max-w-6xl px-4 py-16">
        <h2 className="text-2xl font-bold">From submit to verdict</h2>
        <p className="text-muted mt-2 max-w-2xl text-sm leading-6">
          Your code never runs next to the website or the database. It travels
          through a queue to a runner and executes only inside a sandbox.
        </p>
        <Reveal className="bg-panel border-panel-border mt-8 rounded-lg border p-6">
          <PipelineDiagram />
        </Reveal>
      </section>

      <section className="border-panel-border border-t">
        <div className="mx-auto w-full max-w-6xl px-4 py-16">
          <h2 className="text-2xl font-bold">Built for fair, fast judging</h2>
          <p className="text-muted mt-2 max-w-2xl text-sm leading-6">
            Everything you need to practice and compete, with safety and speed
            built in.
          </p>
          <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {features.map((f, i) => (
              <Reveal
                key={f.title}
                delay={(i % 3) * 0.07}
                className="bg-panel border-panel-border hover:border-link h-full rounded-lg border p-5 transition-colors"
              >
                <Icon>{f.icon}</Icon>
                <h3 className="mt-4 text-base font-semibold">{f.title}</h3>
                <p className="text-muted mt-2 text-sm leading-6">{f.text}</p>
              </Reveal>
            ))}
          </div>
        </div>
      </section>

      <section className="border-panel-border border-y">
        <div className="mx-auto w-full max-w-6xl px-4 py-16">
          <h2 className="text-2xl font-bold">How it works</h2>
          <ol className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {steps.map((s, i) => (
              <li key={s.title}>
                <Reveal
                  delay={i * 0.08}
                  className="bg-panel border-panel-border h-full rounded-lg border p-5"
                >
                  <span className="text-link font-mono text-sm font-semibold">
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <h3 className="mt-2 text-base font-semibold">{s.title}</h3>
                  <p className="text-muted mt-2 text-sm leading-6">{s.text}</p>
                </Reveal>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section className="mx-auto w-full max-w-6xl px-4 py-16">
        <Reveal className="bg-panel border-panel-border flex flex-col items-start justify-between gap-6 rounded-lg border p-8 sm:flex-row sm:items-center">
          <div>
            <h2 className="text-xl font-bold">Ready to start?</h2>
            <p className="text-muted mt-1 text-sm">
              Create a free account to save your submissions and join contests.
            </p>
          </div>
          <div className="flex gap-3">
            <ButtonLink href="/signup" className={primaryBtn}>
              Create account
            </ButtonLink>
            <ButtonLink href="/login" className={secondaryBtn}>
              Sign in
            </ButtonLink>
          </div>
        </Reveal>
      </section>

      <footer className="border-panel-border text-muted border-t">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-2 px-4 py-6 text-sm">
          <span>LeetForce</span>
          <nav aria-label="Footer" className="flex gap-4">
            <Link href="/problems" className="hover:text-foreground">
              Problems
            </Link>
            <Link href="/contest" className="hover:text-foreground">
              Contest
            </Link>
            <Link href="/leaderboard" className="hover:text-foreground">
              Leaderboard
            </Link>
          </nav>
        </div>
      </footer>
    </main>
  );
}
