import type { ReactNode } from "react";

// A decorative, always-dark editor window for the home page hero. Colours are
// the brand tokens (never raw hex): ink for the surface, sky for keywords,
// success for strings, warning for numbers and function names.
type Kind = "kw" | "fn" | "str" | "num" | "cmt" | "op" | "txt";

const color: Record<Kind, string> = {
  kw: "text-[var(--lf-sky-400)]",
  fn: "text-[var(--lf-warning)]",
  str: "text-[var(--lf-success)]",
  num: "text-[var(--lf-warning)]",
  cmt: "text-[var(--lf-muted)] italic",
  op: "text-[var(--lf-border)]",
  txt: "text-[var(--lf-white)]",
};

type Token = [Kind, string];

const lines: Token[][] = [
  [["cmt", "# sum the numbers"]],
  [
    ["kw", "def "],
    ["fn", "solve"],
    ["op", "("],
    ["txt", "nums"],
    ["op", "):"],
  ],
  [
    ["txt", "    total "],
    ["op", "= "],
    ["num", "0"],
  ],
  [
    ["txt", "    "],
    ["kw", "for "],
    ["txt", "x "],
    ["kw", "in "],
    ["txt", "nums"],
    ["op", ":"],
  ],
  [
    ["txt", "        total "],
    ["op", "+= "],
    ["txt", "x"],
  ],
  [
    ["txt", "    "],
    ["kw", "return "],
    ["txt", "total"],
  ],
  [],
  [
    ["txt", "nums "],
    ["op", "= "],
    ["fn", "list"],
    ["op", "("],
    ["fn", "map"],
    ["op", "("],
    ["fn", "int"],
    ["op", ", "],
    ["fn", "input"],
    ["op", "()."],
    ["fn", "split"],
    ["op", "()))"],
  ],
  [
    ["fn", "print"],
    ["op", "("],
    ["fn", "solve"],
    ["op", "(nums))"],
  ],
];

function Dot({ tone }: { tone: string }) {
  return (
    <span
      className={`block h-2.5 w-2.5 rounded-full ${tone}`}
      aria-hidden="true"
    />
  );
}

export function CodeWindow(): ReactNode {
  return (
    <div
      aria-hidden="true"
      className="flex h-full w-full max-w-md flex-col overflow-hidden rounded-xl border border-[var(--lf-ink-700)] bg-[var(--lf-ink-950)] shadow-[0_12px_40px_rgb(0_0_0/0.35)]"
    >
      <div className="flex h-10 items-center justify-between border-b border-[var(--lf-ink-700)] bg-[var(--lf-ink-900)] px-4">
        <span className="font-mono text-xs text-[var(--lf-border)]">
          solution.py
        </span>
        <span className="flex items-center gap-1.5">
          <Dot tone="bg-[var(--lf-danger)]" />
          <Dot tone="bg-[var(--lf-warning)]" />
          <Dot tone="bg-[var(--lf-success)]" />
        </span>
      </div>
      <pre className="flex-1 overflow-hidden px-4 py-5 font-mono text-[13px] leading-7">
        {lines.map((tokens, i) => (
          <div key={i} className="flex">
            <span className="w-6 shrink-0 pr-4 text-right text-[var(--lf-muted)] select-none">
              {i + 1}
            </span>
            <code className="whitespace-pre">
              {tokens.length === 0
                ? " "
                : tokens.map(([kind, text], j) => (
                    <span key={j} className={color[kind]}>
                      {text}
                    </span>
                  ))}
            </code>
          </div>
        ))}
      </pre>
    </div>
  );
}
