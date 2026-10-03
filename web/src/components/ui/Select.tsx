"use client";

import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useId, useRef, useState } from "react";

export interface SelectOption {
  value: string;
  label: string;
}

interface Props {
  options: SelectOption[];
  /** Controlled value; omit and use defaultValue for an uncontrolled field. */
  value?: string;
  defaultValue?: string;
  onChange?: (value: string) => void;
  /** Adds a hidden input so the value is submitted with a plain GET form. */
  name?: string;
  label: string;
  className?: string;
  /** 32px high instead of 36px, for toolbars. */
  compact?: boolean;
}

// A listbox dropdown that replaces the native <select>: the popup follows the
// theme tokens. Keyboard: Enter/Space/ArrowDown open, arrows move, Home/End
// jump, Enter picks, Escape closes, typing a letter jumps to a matching option.
export function Select({
  options,
  value,
  defaultValue,
  onChange,
  name,
  label,
  className = "",
  compact = false,
}: Props) {
  const [inner, setInner] = useState(defaultValue ?? options[0]?.value ?? "");
  const current = value ?? inner;
  const selected = Math.max(
    0,
    options.findIndex((o) => o.value === current),
  );
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(selected);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const listId = useId();

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  function openList() {
    setActive(selected);
    setOpen(true);
  }

  function pick(i: number) {
    const next = options[i];
    if (!next) return;
    setInner(next.value);
    onChange?.(next.value);
    setOpen(false);
    button.current?.focus();
  }

  function onKeyDown(e: React.KeyboardEvent) {
    const last = options.length - 1;
    if (!open) {
      if (["Enter", " ", "ArrowDown", "ArrowUp"].includes(e.key)) {
        e.preventDefault();
        openList();
      }
      return;
    }
    if (e.key === "Escape") {
      e.preventDefault();
      setOpen(false);
      button.current?.focus();
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(last, a + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(0, a - 1));
    } else if (e.key === "Home") {
      e.preventDefault();
      setActive(0);
    } else if (e.key === "End") {
      e.preventDefault();
      setActive(last);
    } else if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      pick(active);
    } else if (e.key === "Tab") {
      setOpen(false);
    } else if (e.key.length === 1) {
      const ch = e.key.toLowerCase();
      const from = options.findIndex(
        (o, i) => i > active && o.label.toLowerCase().startsWith(ch),
      );
      const at =
        from >= 0
          ? from
          : options.findIndex((o) => o.label.toLowerCase().startsWith(ch));
      if (at >= 0) setActive(at);
    }
  }

  return (
    <div ref={root} className={`relative ${className}`} onKeyDown={onKeyDown}>
      {name && <input type="hidden" name={name} value={current} />}
      <button
        ref={button}
        type="button"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        onClick={() => (open ? setOpen(false) : openList())}
        className={`bg-panel border-panel-border text-foreground hover:bg-hover flex ${compact ? "h-8" : "h-9"} w-full min-w-28 items-center justify-between gap-2 rounded-lg border px-3 text-left text-sm transition-colors`}
      >
        <span className="truncate">{options[selected]?.label}</span>
        <svg
          width="12"
          height="12"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className={`text-muted shrink-0 transition-transform ${open ? "rotate-180" : ""}`}
        >
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>
      <AnimatePresence>
        {open && (
          <motion.ul
            initial={{ opacity: 0, y: -4, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -4, scale: 0.98 }}
            transition={{ duration: 0.12 }}
            id={listId}
            role="listbox"
            aria-label={label}
            aria-activedescendant={`${listId}-${active}`}
            className="bg-panel border-panel-border absolute z-30 mt-1 max-h-60 w-full min-w-36 overflow-auto rounded-lg border py-1 shadow-lg"
          >
            {options.map((o, i) => (
              <li
                key={o.value}
                id={`${listId}-${i}`}
                role="option"
                aria-selected={o.value === current}
                onMouseEnter={() => setActive(i)}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => pick(i)}
                className={`flex cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-sm ${
                  i === active ? "bg-hover" : ""
                } ${o.value === current ? "text-link font-semibold" : ""}`}
              >
                <span className="truncate">{o.label}</span>
                {o.value === current && <span aria-hidden="true">{"✓"}</span>}
              </li>
            ))}
          </motion.ul>
        )}
      </AnimatePresence>
    </div>
  );
}
