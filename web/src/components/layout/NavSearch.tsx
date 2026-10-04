"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

// Search box for the navbar: Enter opens the problem list filtered by the text.
// It is hidden on narrow screens, where the problem list has its own search.
export function NavSearch() {
  const router = useRouter();
  const [q, setQ] = useState("");

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const text = q.trim();
    router.push(text ? `/problems?q=${encodeURIComponent(text)}` : "/problems");
  }

  return (
    <form role="search" onSubmit={onSubmit} className="hidden sm:block">
      <label className="relative block">
        <span className="sr-only">Search problems</span>
        <svg
          width="14"
          height="14"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
          className="text-muted pointer-events-none absolute top-1/2 left-3 -translate-y-1/2"
        >
          <circle cx="11" cy="11" r="7" />
          <path d="m20 20-3.5-3.5" />
        </svg>
        <input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Search problems"
          className="bg-hover border-panel-border placeholder:text-muted focus:border-link h-8 w-44 rounded-full border pr-3 pl-8 text-sm transition-[width,border-color] focus:w-56 lg:w-52 lg:focus:w-64"
        />
      </label>
    </form>
  );
}
