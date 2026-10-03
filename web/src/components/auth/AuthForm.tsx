"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useUser } from "@/components/auth/AuthProvider";
import { ApiError, login, signup } from "@/lib/api/client";

const inputClass =
  "bg-background border-panel-border w-full rounded-md border px-3 py-2 text-sm";

function safeNext(next: string | null): string {
  return next && next.startsWith("/") && !next.startsWith("//")
    ? next
    : "/problems";
}

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
  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-sm font-medium">
        {label}
      </label>
      <input
        id={id}
        name={id}
        type={type}
        required
        autoComplete={autoComplete}
        className={inputClass}
      />
      {hint && <p className="text-muted mt-1 text-xs">{hint}</p>}
    </div>
  );
}

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
    <main className="mx-auto w-full max-w-sm flex-1 px-4 py-12">
      <h1 className="mb-6 text-2xl font-bold">
        {isLogin ? "Sign in" : "Create account"}
      </h1>
      <form
        onSubmit={onSubmit}
        className="bg-panel border-panel-border space-y-4 rounded-lg border p-5"
      >
        {isLogin ? (
          <Field id="login" label="Email or username" autoComplete="username" />
        ) : (
          <>
            <Field id="email" label="Email" type="email" autoComplete="email" />
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
          <p role="alert" className="text-danger text-sm">
            {error}
          </p>
        )}
        <button
          type="submit"
          disabled={pending}
          className="bg-primary w-full rounded-md px-3 py-2 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-60"
        >
          {pending ? "Please wait…" : isLogin ? "Sign in" : "Create account"}
        </button>
      </form>
      <p className="text-muted mt-4 text-sm">
        {isLogin ? "No account yet? " : "Already have an account? "}
        <Link
          href={`${isLogin ? "/signup" : "/login"}${nextQuery}`}
          className="text-link font-semibold"
        >
          {isLogin ? "Create one" : "Sign in"}
        </Link>
      </p>
    </main>
  );
}
