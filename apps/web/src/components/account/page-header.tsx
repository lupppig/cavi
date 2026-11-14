import Link from "next/link";
import type { ReactNode } from "react";

/** Breadcrumb, title and actions: how every signed-in page starts. */
export function PageHeader({
  crumbs = [],
  title,
  subtitle,
  actions,
}: {
  crumbs?: { label: string; href: string }[];
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="flex flex-col gap-4 border-b border-line pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0">
        {crumbs.length > 0 && (
          <nav aria-label="Breadcrumb">
            <ol className="flex flex-wrap items-center gap-1 text-sm font-medium text-muted">
              {crumbs.map((c, i) => (
                <li key={c.href} className="flex min-w-0 items-center gap-1">
                  {i > 0 && <span aria-hidden="true">/</span>}
                  <Link href={c.href} className="inline-flex min-h-11 min-w-11 max-w-[14rem] items-center truncate hover:text-ink">
                    <span className="truncate">{c.label}</span>
                  </Link>
                </li>
              ))}
            </ol>
          </nav>
        )}
        <h1 className="text-2xl break-words sm:text-4xl">{title}</h1>
        {subtitle && <div className="mt-2 font-medium text-muted">{subtitle}</div>}
      </div>
      {actions && <div className="flex shrink-0 flex-wrap gap-2">{actions}</div>}
    </header>
  );
}

/** The panel used for sign-in, not-found and error states. */
export function StatePanel({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="card p-6 sm:p-10">
      <h1 className="text-3xl sm:text-4xl">{title}</h1>
      <div className="mt-3 max-w-xl text-lg font-medium">{children}</div>
    </div>
  );
}
