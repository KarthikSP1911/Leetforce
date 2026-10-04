"use client";

import { useId, useRef, useState, type ReactNode } from "react";

export interface TabDef {
  id: string;
  label: string;
  content: ReactNode;
}

// Accessible tab list: roving tabindex, arrow keys move between tabs.
// Uncontrolled by default; pass active and onChange to drive it from outside
// (the console switches to Result when a run starts).
export function Tabs({
  tabs,
  label,
  active: controlled,
  onChange,
}: {
  tabs: TabDef[];
  label: string;
  active?: string;
  onChange?: (id: string) => void;
}) {
  const [inner, setInner] = useState(tabs[0].id);
  const active = controlled ?? inner;
  const setActive = (id: string) => {
    setInner(id);
    onChange?.(id);
  };
  const base = useId();
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});

  const onKeyDown = (e: React.KeyboardEvent) => {
    const i = tabs.findIndex((t) => t.id === active);
    let next = i;
    if (e.key === "ArrowRight") next = (i + 1) % tabs.length;
    else if (e.key === "ArrowLeft") next = (i - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = tabs.length - 1;
    else return;
    e.preventDefault();
    setActive(tabs[next].id);
    refs.current[tabs[next].id]?.focus();
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        role="tablist"
        aria-label={label}
        onKeyDown={onKeyDown}
        className="border-panel-border flex h-11 shrink-0 items-stretch gap-1 border-b px-2"
      >
        {tabs.map((t) => {
          const selected = t.id === active;
          return (
            <button
              key={t.id}
              ref={(el) => {
                refs.current[t.id] = el;
              }}
              role="tab"
              id={`${base}-tab-${t.id}`}
              aria-selected={selected}
              aria-controls={`${base}-panel-${t.id}`}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActive(t.id)}
              className={`-mb-px flex items-center border-b-2 px-3 text-sm font-medium transition-colors ${
                selected
                  ? "border-link text-link"
                  : "text-muted hover:text-foreground border-transparent"
              }`}
            >
              {t.label}
            </button>
          );
        })}
      </div>
      {tabs.map((t) => (
        <div
          key={t.id}
          role="tabpanel"
          id={`${base}-panel-${t.id}`}
          aria-labelledby={`${base}-tab-${t.id}`}
          hidden={t.id !== active}
          className="min-h-0 flex-1 overflow-auto"
        >
          {t.content}
        </div>
      ))}
    </div>
  );
}
