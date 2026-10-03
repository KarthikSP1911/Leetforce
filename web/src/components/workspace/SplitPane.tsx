"use client";

import { useCallback, useRef, useState, type ReactNode } from "react";

interface Props {
  direction: "horizontal" | "vertical";
  first: ReactNode;
  second: ReactNode;
  /** Initial size of the first pane, percent. */
  initial?: number;
  min?: number;
  max?: number;
  label: string;
}

const STEP = 2;

// Resizable two-pane layout. The divider is a keyboard-focusable separator:
// arrow keys resize, Home/End jump to the limits.
export function SplitPane({
  direction,
  first,
  second,
  initial = 50,
  min = 20,
  max = 80,
  label,
}: Props) {
  const [size, setSize] = useState(initial);
  const box = useRef<HTMLDivElement>(null);
  const horizontal = direction === "horizontal";
  const clamp = useCallback(
    (v: number) => Math.min(max, Math.max(min, v)),
    [min, max],
  );

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
  };
  const onPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId) || !box.current) return;
    const r = box.current.getBoundingClientRect();
    const pct = horizontal
      ? ((e.clientX - r.left) / r.width) * 100
      : ((e.clientY - r.top) / r.height) * 100;
    setSize(clamp(pct));
  };
  const onKeyDown = (e: React.KeyboardEvent) => {
    const dec = horizontal ? "ArrowLeft" : "ArrowUp";
    const inc = horizontal ? "ArrowRight" : "ArrowDown";
    if (e.key === dec) setSize((s) => clamp(s - STEP));
    else if (e.key === inc) setSize((s) => clamp(s + STEP));
    else if (e.key === "Home") setSize(min);
    else if (e.key === "End") setSize(max);
    else return;
    e.preventDefault();
  };

  return (
    <div
      ref={box}
      className={`flex h-full min-h-0 w-full ${horizontal ? "flex-row" : "flex-col"}`}
    >
      <div
        className="min-h-0 min-w-0 overflow-hidden"
        style={{ flexBasis: `${size}%` }}
      >
        {first}
      </div>
      <div
        role="separator"
        tabIndex={0}
        aria-label={label}
        aria-orientation={horizontal ? "vertical" : "horizontal"}
        aria-valuenow={Math.round(size)}
        aria-valuemin={min}
        aria-valuemax={max}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onKeyDown={onKeyDown}
        className={`hover:bg-primary focus-visible:bg-primary shrink-0 touch-none bg-transparent transition-colors ${
          horizontal
            ? "mx-0.5 w-1 cursor-col-resize"
            : "my-0.5 h-1 cursor-row-resize"
        } rounded`}
      />
      <div className="min-h-0 min-w-0 flex-1 overflow-hidden">{second}</div>
    </div>
  );
}
