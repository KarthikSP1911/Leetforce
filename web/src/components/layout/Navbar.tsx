import Image from "next/image";
import Link from "next/link";
import { ThemeToggle } from "./ThemeToggle";

const links = [
  { href: "/problems", label: "Problems" },
  { href: "/contest", label: "Contest" },
  { href: "/leaderboard", label: "Leaderboard" },
];

export function Navbar() {
  return (
    <header className="bg-panel border-panel-border sticky top-0 z-10 border-b">
      <nav className="mx-auto flex h-14 max-w-6xl items-center gap-6 px-4">
        <Link href="/problems" className="flex items-center gap-2">
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
          <span className="text-foreground text-base font-bold">LeetForce</span>
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
          <Link
            href="/login"
            className="bg-primary rounded-md px-3 py-1.5 text-sm font-semibold text-white transition-opacity hover:opacity-90"
          >
            Sign in
          </Link>
        </div>
      </nav>
    </header>
  );
}
