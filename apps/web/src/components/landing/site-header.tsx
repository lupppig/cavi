import { signInHref } from "@/lib/api";

const links = [
  { href: "#workflow", label: "How it works" },
  { href: "#reframe", label: "Reframing" },
  { href: "#who", label: "Who it's for" },
];

function Logo() {
  return (
    <svg viewBox="0 0 32 32" className="size-8" aria-hidden="true">
      <rect x="2" y="2" width="28" height="28" rx="9" fill="var(--tomato)" stroke="var(--ink)" strokeWidth="3" />
      <path d="M12 9.5 L23 16 L12 22.5 Z" fill="white" stroke="var(--ink)" strokeWidth="2.5" strokeLinejoin="round" />
    </svg>
  );
}

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 px-3 pt-3 sm:px-6">
      <div className="sticker mx-auto flex h-16 max-w-7xl items-center justify-between gap-3 rounded-full bg-white pr-2 pl-3 sm:pl-4">
        <a href="#top" className="flex min-h-11 items-center gap-2 text-2xl font-extrabold tracking-tight">
          <Logo />
          CAVI
        </a>
        <nav aria-label="Primary" className="hidden md:block">
          <ul className="flex gap-1 text-[15px] font-bold">
            {links.map((l) => (
              <li key={l.href}>
                <a
                  href={l.href}
                  className="flex min-h-11 items-center rounded-full px-4 transition-colors hover:bg-sun"
                >
                  {l.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="flex items-center gap-1">
        <a
          href={signInHref()}
          className="hidden min-h-11 items-center rounded-full px-4 text-[15px] font-bold transition-colors hover:bg-sun sm:flex"
        >
          Sign in
        </a>
        <a
          href="#waitlist"
          className="btn-pop flex min-h-11 items-center rounded-full bg-tomato px-4 text-sm font-extrabold text-white sm:px-5 sm:text-base"
        >
          Join the waitlist
        </a>
        </div>
      </div>
    </header>
  );
}
