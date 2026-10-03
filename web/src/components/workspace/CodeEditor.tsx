"use client";

import Editor from "@monaco-editor/react";
import { useEffect, useRef, useState } from "react";
import type { Language } from "@/types/problem";

const monacoLanguage: Record<Language, string> = {
  python: "python",
  cpp: "cpp",
  java: "java",
  go: "go",
};

// Follows the same rule as globals.css: data-theme wins, else the system.
function useIsDark(): boolean {
  const [dark, setDark] = useState(false);
  useEffect(() => {
    const root = document.documentElement;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const update = () => {
      const t = root.getAttribute("data-theme");
      setDark(t ? t === "dark" : mq.matches);
    };
    update();
    const obs = new MutationObserver(update);
    obs.observe(root, { attributes: true, attributeFilter: ["data-theme"] });
    mq.addEventListener("change", update);
    return () => {
      obs.disconnect();
      mq.removeEventListener("change", update);
    };
  }, []);
  return dark;
}

export function CodeEditor({
  language,
  value,
  onChange,
  onRun,
  onSubmit,
}: {
  language: Language;
  value: string;
  onChange: (v: string) => void;
  /** Ctrl/Cmd+Enter */
  onRun?: () => void;
  /** Ctrl/Cmd+Shift+Enter */
  onSubmit?: () => void;
}) {
  const dark = useIsDark();
  // Monaco registers commands once, so they call the latest handlers via refs.
  const run = useRef(onRun);
  const submit = useRef(onSubmit);
  useEffect(() => {
    run.current = onRun;
    submit.current = onSubmit;
  });
  return (
    <Editor
      height="100%"
      language={monacoLanguage[language]}
      value={value}
      theme={dark ? "vs-dark" : "light"}
      onChange={(v) => onChange(v ?? "")}
      onMount={(editor, monaco) => {
        // Monaco cannot read CSS variables, so hand it the resolved code font.
        const mono = getComputedStyle(document.documentElement)
          .getPropertyValue("--font-mono")
          .trim();
        if (mono) {
          editor.updateOptions({ fontFamily: mono });
          monaco.editor.remeasureFonts();
        }
        // Monaco swallows these keys (Ctrl+Enter inserts a line), so they are
        // registered on the editor rather than on the page.
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () =>
          run.current?.(),
        );
        editor.addCommand(
          monaco.KeyMod.CtrlCmd | monaco.KeyMod.Shift | monaco.KeyCode.Enter,
          () => submit.current?.(),
        );
      }}
      loading={<p className="text-muted p-4 text-sm">Loading editor…</p>}
      options={{
        fontFamily: "Menlo, Monaco, Consolas, 'Courier New', monospace",
        fontSize: 14,
        minimap: { enabled: false },
        scrollBeyondLastLine: false,
        automaticLayout: true,
        tabSize: 4,
        ariaLabel: "Code editor",
      }}
    />
  );
}
