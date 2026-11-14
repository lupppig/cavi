import { signInHref } from "@/lib/api";

const links = [
  { href: "#game", label: "Play" },
  { href: "#workflow", label: "How it works" },
  { href: "#who", label: "Who it’s for" },
];

/** Logo mark: a frame with a play head cut through it. */
export function Mark({ className = "size-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} aria-hidden="true">
      <rect width="64" height="64" rx="16" fill="#FF5A1F" />
      <rect x="14" y="16" width="6" height="32" rx="3" fill="#FFFFFF" />
      <path d="M27 17.5c0-2.3 2.5-3.7 4.5-2.5l19 12.5c1.8 1.2 1.8 3.8 0 5L31.5 49c-2 1.2-4.5-.2-4.5-2.5z" fill="#FFFFFF" />
    </svg>
  );
}

/** A light bar that stays over every section. */
export function SiteHeader() {
  return (
    <header className="site-header fixed inset-x-0 top-0 z-40">
      <div className="mx-auto flex h-16 max-w-[90rem] items-center justify-between gap-3 px-4 sm:h-20 sm:px-8">
        <a href="#top" className="flex min-h-11 items-center gap-2.5 text-lg font-semibold tracking-tight">
          <Mark />
          CAVI
        </a>
        <nav aria-label="Primary" className="hidden md:block">
          <ul className="flex gap-1 text-sm font-medium text-[var(--subtle)]">
            {links.map((l) => (
              <li key={l.href}>
                <a href={l.href} className="flex min-h-11 items-center rounded-lg px-3 transition-colors hover:bg-black/5 hover:text-[var(--fg)]">
                  {l.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="flex items-center gap-1 sm:gap-2">
          <a href={signInHref()} className="hidden min-h-11 items-center rounded-lg px-3 text-sm font-medium text-[var(--subtle)] transition-colors hover:bg-black/5 hover:text-[var(--fg)] sm:flex">
            Sign in
          </a>
          <a
            href="#waitlist"
            className="btn btn-sm"
          >
            Join the waitlist
          </a>
        </div>
      </div>
    </header>
  );
}
