const links = [
  { href: "#workflow", label: "Workflow" },
  { href: "#reframe", label: "Reframing" },
  { href: "#direction", label: "Direction" },
  { href: "#who", label: "Who it's for" },
];

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b border-line/60 bg-ink/90 backdrop-blur-sm">
      <div className="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-4 sm:px-8">
        <a
          href="#top"
          className="flex min-h-11 items-center text-lg font-extrabold tracking-tight font-stretch-[75%]"
        >
          CAVI
        </a>
        <nav aria-label="Primary" className="hidden md:block">
          <ul className="flex gap-6 text-sm text-muted">
            {links.map((l) => (
              <li key={l.href}>
                <a href={l.href} className="flex min-h-11 items-center px-1 transition-colors hover:text-paper">
                  {l.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <a
          href="#waitlist"
          className="flex min-h-11 items-center border border-paper px-4 text-sm font-semibold transition-colors hover:bg-paper hover:text-ink"
        >
          Join the waitlist
        </a>
      </div>
    </header>
  );
}
