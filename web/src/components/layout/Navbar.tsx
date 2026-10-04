"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ThemeToggle } from "./ThemeToggle";
import { UserMenu } from "./UserMenu";

const links = [
  { href: "/problems", label: "Problems" },
  { href: "/contest", label: "Contest" },
  { href: "/leaderboard", label: "Leaderboard" },
];

// The problem workspace uses the full window width; every other page sits in
// a centred column, and the navbar follows the same column so edges line up.
const workspacePath = /^\/problems\/[^/]+\/?$/;

export function Navbar() {
  const wide = workspacePath.test(usePathname());
  return (
    <header className="bg-panel border-panel-border sticky top-0 z-10 border-b">
      <nav
        className={`mx-auto flex h-14 items-center gap-6 px-4 ${
          wide ? "" : "max-w-6xl"
        }`}
      >
        <Link href="/" className="flex items-center gap-2">
          <Image
            src="/brand/logo-mark.svg"
            alt=""
            width={28}
            height={28}
            className="logo-on-dark"
            priority
          />
          <Image
            src="/brand/logo-mark-light.svg"
            alt=""
            width={28}
            height={28}
            className="logo-on-light"
            priority
          />
          <span className="text-foreground text-lg leading-none font-bold tracking-wider uppercase">
            LeetForce
          </span>
        </Link>
        <ul className="flex items-center gap-1">
          {links.map((link) => (
            <li key={link.href}>
              <Link
                href={link.href}
                className="text-muted hover:text-foreground hover:bg-hover rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
              >
                {link.label}
              </Link>
            </li>
          ))}
        </ul>
        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          <UserMenu />
        </div>
      </nav>
    </header>
  );
}
