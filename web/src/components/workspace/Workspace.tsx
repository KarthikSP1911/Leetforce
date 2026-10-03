"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import type { Difficulty, Language, ProblemDetail } from "@/types/problem";
import { useJudge } from "@/hooks/useJudge";
import { CodeEditor } from "./CodeEditor";
import { ResultPanel, type ConsoleResult } from "./ResultPanel";
import { SubmissionsTab } from "./SubmissionsTab";
import { SplitPane } from "./SplitPane";
import { Tabs } from "./Tabs";

const languages: { id: Language; label: string }[] = [
  { id: "python", label: "Python" },
  { id: "cpp", label: "C++" },
  { id: "java", label: "Java" },
  { id: "go", label: "Go" },
];

const difficultyClass: Record<Difficulty, string> = {
  easy: "text-success",
  medium: "text-warning",
  hard: "text-danger",
};

const difficultyLabel: Record<Difficulty, string> = {
  easy: "Easy",
  medium: "Medium",
  hard: "Hard",
};

function Description({ problem }: { problem: ProblemDetail }) {
  return (
    <article className="p-4">
      <h1 className="text-xl font-bold">{problem.title}</h1>
      <p className="mt-2 flex flex-wrap items-center gap-3 text-sm">
        <span
          className={`font-semibold ${difficultyClass[problem.difficulty]}`}
        >
          {difficultyLabel[problem.difficulty]}
        </span>
        {problem.tags.map((t) => (
          <span
            key={t}
            className="bg-hover border-panel-border text-muted rounded-full border px-2 py-0.5 text-xs"
          >
            {t}
          </span>
        ))}
      </p>
      <div className="statement mt-4">
        {problem.statement ? (
          <ReactMarkdown>{problem.statement}</ReactMarkdown>
        ) : (
          <p className="text-muted">No statement yet.</p>
        )}
      </div>
      {problem.samples.map((s, i) => (
        <section key={i} className="mt-4">
          <h2 className="text-sm font-semibold">Example {i + 1}</h2>
          <p className="text-muted mt-2 text-xs">Input</p>
          <pre className="bg-hover border-panel-border mt-1 overflow-x-auto rounded-lg border p-3 font-mono text-xs">
            {s.input}
          </pre>
          <p className="text-muted mt-2 text-xs">Output</p>
          <pre className="bg-hover border-panel-border mt-1 overflow-x-auto rounded-lg border p-3 font-mono text-xs">
            {s.expected}
          </pre>
        </section>
      ))}
    </article>
  );
}

type InputMode = "samples" | "custom";

function Console({
  problem,
  tab,
  onTab,
  mode,
  onMode,
  custom,
  onCustom,
  result,
}: {
  problem: ProblemDetail;
  tab: string;
  onTab: (id: string) => void;
  mode: InputMode;
  onMode: (m: InputMode) => void;
  custom: string;
  onCustom: (v: string) => void;
  result: ConsoleResult;
}) {
  const [sample, setSample] = useState(0);
  const s = problem.samples[sample];
  const pill = (active: boolean) =>
    `rounded-lg px-3 py-1 text-xs font-medium ${
      active ? "bg-hover text-foreground" : "text-muted hover:bg-hover"
    }`;
  return (
    <Tabs
      label="Console"
      active={tab}
      onChange={onTab}
      tabs={[
        {
          id: "testcase",
          label: "Testcase",
          content: (
            <div className="p-3">
              <div className="mb-2 flex flex-wrap gap-2">
                <button
                  type="button"
                  aria-pressed={mode === "samples"}
                  onClick={() => onMode("samples")}
                  className={pill(mode === "samples")}
                >
                  Samples
                </button>
                <button
                  type="button"
                  aria-pressed={mode === "custom"}
                  onClick={() => onMode("custom")}
                  className={pill(mode === "custom")}
                >
                  Custom input
                </button>
              </div>
              {mode === "custom" ? (
                <label className="block">
                  <span className="text-muted text-xs">
                    Run uses this as standard input
                  </span>
                  <textarea
                    value={custom}
                    onChange={(e) => onCustom(e.target.value)}
                    spellCheck={false}
                    rows={5}
                    className="bg-hover border-panel-border mt-1 block w-full rounded-lg border p-3 font-mono text-xs"
                  />
                </label>
              ) : (
                <>
                  {problem.samples.length > 1 && (
                    <div className="mb-2 flex gap-2">
                      {problem.samples.map((_, i) => (
                        <button
                          key={i}
                          type="button"
                          aria-pressed={i === sample}
                          onClick={() => setSample(i)}
                          className={pill(i === sample)}
                        >
                          Case {i + 1}
                        </button>
                      ))}
                    </div>
                  )}
                  <pre className="bg-hover border-panel-border overflow-auto rounded-lg border p-3 font-mono text-xs">
                    {s ? s.input : "No sample input."}
                  </pre>
                  <p className="text-muted mt-2 text-xs">
                    Run checks every sample test.
                  </p>
                </>
              )}
            </div>
          ),
        },
        {
          id: "result",
          label: "Result",
          content: <ResultPanel result={result} />,
        },
      ]}
    />
  );
}

export function Workspace({ problem }: { problem: ProblemDetail }) {
  const available = languages.filter((l) => problem.starters[l.id]);
  const options = available.length > 0 ? available : languages;
  const [language, setLanguage] = useState<Language>(options[0].id);
  const [code, setCode] = useState<Partial<Record<Language, string>>>(
    problem.starters,
  );
  const [consoleTab, setConsoleTab] = useState("testcase");
  const [mode, setMode] = useState<InputMode>(
    problem.samples.length > 0 ? "samples" : "custom",
  );
  const [custom, setCustom] = useState("");
  const [listVersion, setListVersion] = useState(0);
  const { result, busy, run, submit } = useJudge(problem.slug, () =>
    setListVersion((v) => v + 1),
  );

  const source = code[language] ?? "";
  const doRun = () => {
    if (busy) return;
    setConsoleTab("result");
    void run(language, source, mode === "custom" ? custom : undefined);
  };
  const doSubmit = () => {
    if (busy) return;
    setConsoleTab("result");
    void submit(language, source);
  };

  const editor = (
    <div className="bg-panel flex h-full min-h-0 flex-col">
      <div className="border-panel-border flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <label className="flex items-center gap-2 text-sm">
          <span className="text-muted">Language</span>
          <select
            value={language}
            onChange={(e) => setLanguage(e.target.value as Language)}
            className="bg-panel border-panel-border h-8 rounded-lg border px-2 text-sm"
          >
            {options.map((l) => (
              <option key={l.id} value={l.id}>
                {l.label}
              </option>
            ))}
          </select>
        </label>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={doRun}
            disabled={busy || source.trim() === ""}
            title="Run (Ctrl+Enter)"
            className="bg-panel border-panel-border h-8 rounded-lg border px-4 text-sm font-semibold disabled:opacity-50"
          >
            Run
          </button>
          <button
            type="button"
            onClick={doSubmit}
            disabled={busy || source.trim() === ""}
            title="Submit (Ctrl+Shift+Enter)"
            className="bg-primary h-8 rounded-lg px-4 text-sm font-semibold text-white disabled:opacity-50"
          >
            Submit
          </button>
        </div>
      </div>
      <div className="min-h-0 flex-1">
        <CodeEditor
          language={language}
          value={source}
          onChange={(v) => setCode((c) => ({ ...c, [language]: v }))}
          onRun={doRun}
          onSubmit={doSubmit}
        />
      </div>
    </div>
  );

  return (
    <div className="h-[calc(100vh-3.5rem)] p-2">
      <SplitPane
        direction="horizontal"
        label="Resize description and editor panels"
        first={
          <div className="bg-panel border-panel-border h-full rounded-lg border">
            <Tabs
              label="Problem"
              tabs={[
                {
                  id: "description",
                  label: "Description",
                  content: <Description problem={problem} />,
                },
                {
                  id: "submissions",
                  label: "Submissions",
                  content: (
                    <SubmissionsTab
                      slug={problem.slug}
                      refreshKey={listVersion}
                    />
                  ),
                },
              ]}
            />
          </div>
        }
        second={
          <SplitPane
            direction="vertical"
            initial={65}
            min={25}
            max={85}
            label="Resize editor and console panels"
            first={
              <div className="border-panel-border h-full overflow-hidden rounded-lg border">
                {editor}
              </div>
            }
            second={
              <div className="bg-panel border-panel-border h-full rounded-lg border">
                <Console
                  problem={problem}
                  tab={consoleTab}
                  onTab={setConsoleTab}
                  mode={mode}
                  onMode={setMode}
                  custom={custom}
                  onCustom={setCustom}
                  result={result}
                />
              </div>
            }
          />
        }
      />
    </div>
  );
}
