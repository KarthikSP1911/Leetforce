"use client";

import Editor from "@monaco-editor/react";
import { useEffect, useState } from "react";
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
}: {
  language: Language;
  value: string;
  onChange: (v: string) => void;
}) {
  const dark = useIsDark();
  return (
    <Editor
      height="100%"
      language={monacoLanguage[language]}
      value={value}
      theme={dark ? "vs-dark" : "light"}
      onChange={(v) => onChange(v ?? "")}
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
