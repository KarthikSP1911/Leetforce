"use client";

import { AnimatePresence, motion } from "framer-motion";
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
        className="hover:ring-panel-border rounded-full transition-shadow hover:ring-2"
      >
        <span
          aria-hidden="true"
          className="bg-primary flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold text-white uppercase"
        >
          {user.username.charAt(0)}
        </span>
      </button>
      <AnimatePresence>
        {open && (
          <motion.div
            initial={{ opacity: 0, y: -4, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: -4, scale: 0.98 }}
            transition={{ duration: 0.12 }}
            role="menu"
            className="bg-panel border-panel-border absolute right-0 mt-2 w-56 rounded-lg border py-1 shadow-lg"
          >
            <div className="border-panel-border flex items-center gap-3 border-b px-3 py-3">
              <span
                aria-hidden="true"
                className="bg-primary flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-sm font-bold text-white uppercase"
              >
                {user.username.charAt(0)}
              </span>
              <span className="truncate text-sm font-semibold">
                {user.username}
              </span>
            </div>
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
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
