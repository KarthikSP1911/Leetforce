"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { useUser } from "@/components/auth/AuthProvider";

export function UserMenu() {
  const { user, loading, signOut } = useUser();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (loading) return null;
  if (!user) {
    return (
      <Link
        href="/login"
        className="bg-primary rounded-md px-3 py-1.5 text-sm font-semibold text-white transition-opacity hover:opacity-90"
      >
        Sign in
      </Link>
    );
  }
  return (
    <div ref={root} className="relative">
      <button
        type="button"
        aria-label={`Account menu for ${user.username}`}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="text-foreground hover:bg-hover rounded-md px-3 py-1.5 text-sm font-semibold transition-colors"
      >
        {user.username}
      </button>
      {open && (
        <div
          role="menu"
          className="bg-panel border-panel-border absolute right-0 mt-1 min-w-36 rounded-lg border py-1"
        >
          <button
            type="button"
            role="menuitem"
            autoFocus
            onClick={async () => {
              setOpen(false);
              await signOut();
              router.refresh();
            }}
            className="hover:bg-hover w-full px-3 py-2 text-left text-sm"
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}
