import { signInHref } from "@/lib/api";

const links = [
  { href: "#game", label: "Play" },
  { href: "#workflow", label: "How it works" },
  { href: "#who", label: "Who it’s for" },
];

/** Logo mark: a frame with a play head cut through it. */
export function Mark({ className = "size-7" }: { className?: string }) {
  return (
    <svg viewBox="0 0 32 32" className={className} aria-hidden="true">
      <rect x="2" y="2" width="28" height="28" fill="none" stroke="currentColor" strokeWidth="3" />
      <path d="M12 9 L23 16 L12 23 Z" fill="currentColor" />
      <path d="M2 16 H8" stroke="currentColor" strokeWidth="3" />
    </svg>
  );
}

/** Sits over every section and inverts against it (mix-blend-mode: difference). */
export function SiteHeader() {
  return (
    <header className="site-header fixed inset-x-0 top-0 z-40">
      <div className="mx-auto flex h-16 max-w-[90rem] items-center justify-between gap-3 px-4 sm:h-20 sm:px-8">
        <a href="#top" className="flex min-h-11 items-center gap-2.5 text-xl font-black tracking-tight [font-stretch:125%]">
          <Mark />
          CAVI
        </a>
        <nav aria-label="Primary" className="hidden md:block">
          <ul className="mono flex gap-1 text-xs font-medium uppercase">
            {links.map((l) => (
              <li key={l.href}>
                <a href={l.href} className="flex min-h-11 items-center px-3 underline-offset-4 hover:underline">
                  {l.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="flex items-center gap-1 sm:gap-2">
          <a href={signInHref()} className="mono hidden min-h-11 items-center px-3 text-xs font-medium uppercase underline-offset-4 hover:underline sm:flex">
            Sign in
          </a>
          <a
            href="#waitlist"
            className="flex min-h-11 items-center border-2 border-current px-3 text-xs font-extrabold tracking-wider uppercase transition-colors hover:bg-white hover:text-black sm:px-4"
          >
            Join the waitlist
          </a>
        </div>
      </div>
    </header>
  );
}
