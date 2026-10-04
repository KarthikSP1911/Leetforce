"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useUser } from "@/components/auth/AuthProvider";
import { ApiError, login, signup } from "@/lib/api/client";
import { safeNext } from "@/lib/safe-next";

const inputClass =
  "bg-background border-panel-border placeholder:text-muted h-10 w-full rounded-lg border px-3 text-sm transition-colors focus:border-link focus:outline-none";

function Field({
  id,
  label,
  type = "text",
  autoComplete,
  hint,
}: {
  id: string;
  label: string;
  type?: string;
  autoComplete: string;
  hint?: string;
}) {
  const [shown, setShown] = useState(false);
  const isPassword = type === "password";
  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-sm font-medium">
        {label}
      </label>
      <div className="relative">
        <input
          id={id}
          name={id}
          type={isPassword && shown ? "text" : type}
          required
          autoComplete={autoComplete}
          placeholder={hint}
          aria-label={hint ? `${label} (${hint})` : undefined}
          className={`${inputClass} ${isPassword ? "pr-16" : ""}`}
        />
        {isPassword && (
          <button
            type="button"
            onClick={() => setShown((v) => !v)}
            aria-label={shown ? "Hide password" : "Show password"}
            aria-pressed={shown}
            className="text-muted hover:text-foreground absolute inset-y-0 right-0 px-3 text-xs font-semibold"
          >
            {shown ? "Hide" : "Show"}
          </button>
        )}
      </div>
    </div>
  );
}

const points = [
  "Judged in an isolated sandbox with live status",
  "Python, C++, Java and Go",
  "Contests and a global leaderboard",
];

export function AuthForm({ mode }: { mode: "login" | "signup" }) {
  const router = useRouter();
  const params = useSearchParams();
  const { refresh } = useUser();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const isLogin = mode === "login";
  const rawNext = params.get("next");
  const next = safeNext(rawNext);
  const nextQuery = rawNext ? `?next=${encodeURIComponent(rawNext)}` : "";

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget);
    const get = (k: string) => String(f.get(k) ?? "");
    setError("");
    setPending(true);
    try {
      if (isLogin) {
        await login({ login: get("login").trim(), password: get("password") });
      } else {
        await signup({
          email: get("email").trim(),
          username: get("username").trim(),
          password: get("password"),
        });
      }
      await refresh();
      router.replace(next);
      router.refresh();
    } catch (err) {
      setPending(false);
      if (err instanceof ApiError) {
        setError(
          err.status === 429
            ? "Too many attempts. Wait a moment and try again."
            : err.message || "The request failed.",
        );
      } else {
        setError("Could not reach the server. Try again.");
      }
    }
  }

  return (
    <main className="mx-auto grid min-h-[calc(100dvh-3.6rem)] w-full max-w-5xl items-center gap-12 px-4 py-4 lg:grid-cols-2">
      <div className="hidden lg:block">
        <h1 className="text-3xl leading-tight font-bold tracking-tight">
          {isLogin ? "Welcome back" : "Join LeetForce"}
        </h1>
        <p className="text-muted mt-3 max-w-sm text-sm leading-6">
          {isLogin
            ? "Sign in to pick up where you left off: your submissions, your contests and your ranking."
            : "Create an account to save your submissions, register for contests and appear on the leaderboard."}
        </p>
        <ul className="mt-8 space-y-3 text-sm">
          {points.map((t) => (
            <li key={t} className="flex items-center gap-3">
              <span
                aria-hidden="true"
                className="bg-hover border-panel-border text-success flex h-6 w-6 items-center justify-center rounded-full border text-xs"
              >
                {"✓"}
              </span>
              {t}
            </li>
          ))}
        </ul>
      </div>
      <div className="w-full max-w-sm justify-self-center lg:justify-self-end">
        <h2 className="mb-1 text-2xl font-bold">
          {isLogin ? "Sign in" : "Create account"}
        </h2>
        <p className="text-muted mb-4 text-sm">
          {isLogin
            ? "Use your email or username."
            : "It takes less than a minute."}
        </p>
        <form
          onSubmit={onSubmit}
          className="bg-panel border-panel-border space-y-3 rounded-lg border p-5"
        >
          {isLogin ? (
            <Field
              id="login"
              label="Email or username"
              autoComplete="username"
            />
          ) : (
            <>
              <Field
                id="email"
                label="Email"
                type="email"
                autoComplete="email"
              />
              <Field
                id="username"
                label="Username"
                autoComplete="username"
                hint="3-32 characters: letters, digits, _ and -."
              />
            </>
          )}
          <Field
            id="password"
            label="Password"
            type="password"
            autoComplete={isLogin ? "current-password" : "new-password"}
            hint={isLogin ? undefined : "At least 8 characters."}
          />
          {error && (
            <p
              role="alert"
              className="border-danger text-danger rounded-lg border px-3 py-2 text-sm"
            >
              {error}
            </p>
          )}
          <button
            type="submit"
            disabled={pending}
            className="bg-primary h-10 w-full rounded-lg px-3 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-60"
          >
            {pending ? "Please wait…" : isLogin ? "Sign in" : "Create account"}
          </button>
        </form>
        <p className="text-muted mt-4 text-center text-sm">
          {isLogin ? "No account yet? " : "Already have an account? "}
          <Link
            href={`${isLogin ? "/signup" : "/login"}${nextQuery}`}
            className="text-link font-semibold"
          >
            {isLogin ? "Create one" : "Sign in"}
          </Link>
        </p>
      </div>
    </main>
  );
}
