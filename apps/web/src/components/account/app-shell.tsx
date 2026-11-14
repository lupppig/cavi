"use client";

import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { useParams, usePathname, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Mark } from "@/components/home/site-header";
import { client, displayName, initials, raise, useMe, type Me } from "@/lib/queries";

/** Organisation sections, shown under the current organisation. */
export const orgTabs = [
  { tab: "projects", label: "Projects" },
  { tab: "people", label: "People" },
  { tab: "settings", label: "Settings" },
] as const;

function useSignOut() {
  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/auth/logout");
      if (!data) raise(error, response.status, "Could not sign out.");
      return data.redirect_url;
    },
    onSuccess: (url) => window.location.assign(url),
  });
}

export function Avatar({ user, size = "size-9", onDark = true }: { user: Me["user"]; size?: string; onDark?: boolean }) {
  return user.avatar_url ? (
    // Avatars come from the identity provider's CDN; a plain img avoids
    // configuring every possible remote host for next/image.
    // eslint-disable-next-line @next/next/no-img-element
    <img src={user.avatar_url} alt="" referrerPolicy="no-referrer" className={`${size} shrink-0 rounded-full object-cover`} />
  ) : (
    <span
      aria-hidden="true"
      className={`${size} grid shrink-0 place-items-center rounded-full font-extrabold ${onDark ? "bg-paper text-ink" : "bg-ink text-paper"}`}
    >
      {initials(user)}
    </span>
  );
}

const navLink = (active: boolean) =>
  `flex min-h-11 items-center gap-2 px-3 text-sm font-extrabold transition-colors ${
    active ? "bg-paper text-ink" : "text-paper hover:bg-paper/15"
  }`;

/** Where you can go: home, each organisation and its sections, and you. */
function Nav({ me }: { me: Me }) {
  const pathname = usePathname();
  const params = useParams<{ orgID?: string }>();
  const search = useSearchParams();
  const signOut = useSignOut();
  const tab = search.get("tab") ?? "projects";
  const inProject = pathname.includes("/projects/");

  return (
    <nav aria-label="App" className="flex h-full flex-col">
      <Link href="/account" className="flex min-h-16 items-center gap-2.5 px-3 text-xl font-black tracking-tight [font-stretch:125%]">
        <Mark />
        CAVI
      </Link>
      <div className="mt-2 flex-1 overflow-y-auto">
        <Link href="/account" aria-current={pathname === "/account" ? "page" : undefined} className={navLink(pathname === "/account")}>
          Home
        </Link>
        <p className="mt-5 px-3 font-mono text-[11px] font-semibold tracking-wider uppercase opacity-80">Organisations</p>
        <ul className="mt-1">
          {me.organisations.map((o) => {
            const current = params.orgID === o.id;
            return (
              <li key={o.id}>
                <Link
                  href={`/account/organisations/${o.id}`}
                  aria-current={current && !inProject && tab === "projects" ? "page" : undefined}
                  className={`${navLink(current && !inProject && tab === "projects")} justify-between`}
                >
                  <span className="truncate">{o.name}</span>
                  {o.personal && <span className="font-mono text-[11px] font-semibold opacity-80">YOU</span>}
                </Link>
                {current && (
                  <ul className="mb-2 ml-3 border-l-2 border-paper/30">
                    {orgTabs.map((t) => {
                      const active = !inProject && tab === t.tab;
                      return (
                        <li key={t.tab}>
                          <Link
                            href={t.tab === "projects" ? `/account/organisations/${o.id}` : `/account/organisations/${o.id}?tab=${t.tab}`}
                            aria-current={active ? "page" : undefined}
                            className={navLink(active)}
                          >
                            {t.label}
                          </Link>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </li>
            );
          })}
        </ul>
      </div>
      <div className="border-t-2 border-paper/30 p-2">
        <Link
          href="/account/profile"
          aria-current={pathname === "/account/profile" ? "page" : undefined}
          className={`${navLink(pathname === "/account/profile")} min-h-14`}
        >
          <Avatar user={me.user} size="size-8" onDark={pathname !== "/account/profile"} />
          <span className="min-w-0">
            <span className="block truncate">{displayName(me.user)}</span>
            <span className="block truncate font-mono text-[11px] font-semibold opacity-80">Profile</span>
          </span>
        </Link>
        <button
          type="button"
          onClick={() => signOut.mutate()}
          disabled={signOut.isPending}
          className="mt-1 flex min-h-11 w-full items-center px-3 text-sm font-extrabold text-paper hover:bg-paper/15 disabled:opacity-60"
        >
          {signOut.isPending ? "Signing out…" : "Sign out"}
        </button>
        {signOut.isError && <p className="px-3 pb-2 text-sm font-bold">{signOut.error.message}</p>}
      </div>
    </nav>
  );
}

/**
 * The signed-in app's frame: a sidebar on wide screens, a top bar with a
 * menu on phones. Signed-out visitors see the page alone (it shows sign-in).
 */
export function AppShell({ children }: { children: ReactNode }) {
  const me = useMe();
  const pathname = usePathname();
  // The menu remembers the page it was opened on, so going anywhere closes it.
  const [openOn, setOpenOn] = useState<string | null>(null);
  const open = openOn === pathname;
  const setOpen = (o: boolean) => setOpenOn(o ? pathname : null);
  const menuButton = useRef<HTMLButtonElement>(null);
  const drawer = useRef<HTMLDivElement>(null);

  // Focus the menu when it opens; Escape closes it.
  useEffect(() => {
    if (!open) return;
    drawer.current?.querySelector<HTMLElement>("a, button")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpenOn(null);
        menuButton.current?.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open]);

  if (!me.data) {
    return (
      <div className="app min-h-dvh">
        <header className="flex min-h-16 items-center bg-ink px-4 text-paper sm:px-8">
          <Link href="/" className="flex min-h-11 items-center gap-2.5 text-xl font-black tracking-tight [font-stretch:125%]">
            <Mark />
            CAVI
          </Link>
        </header>
        <main id="main" className="mx-auto max-w-3xl px-4 py-10 sm:px-8">
          {children}
        </main>
      </div>
    );
  }

  return (
    <div className="app min-h-dvh lg:grid lg:grid-cols-[15rem_minmax(0,1fr)]">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:bg-paper focus:px-4 focus:py-3 focus:font-extrabold focus:text-ink"
      >
        Skip to content
      </a>
      <aside className="sticky top-0 hidden h-dvh bg-ink text-paper lg:block">
        <Nav me={me.data} />
      </aside>

      <header className="sticky top-0 z-30 flex min-h-14 items-center justify-between bg-ink px-3 text-paper lg:hidden">
        <Link href="/account" className="flex min-h-11 items-center gap-2 text-lg font-black tracking-tight [font-stretch:125%]">
          <Mark className="size-6" />
          CAVI
        </Link>
        <button
          ref={menuButton}
          type="button"
          onClick={() => setOpen(true)}
          aria-expanded={open}
          aria-controls="app-menu"
          className="flex min-h-11 items-center gap-2 border-2 border-paper px-3 text-xs font-extrabold tracking-wider uppercase"
        >
          Menu
        </button>
      </header>
      {open && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <button type="button" aria-label="Close menu" onClick={() => setOpen(false)} className="absolute inset-0 bg-ink/60" />
          <div
            id="app-menu"
            ref={drawer}
            role="dialog"
            aria-modal="true"
            aria-label="Menu"
            className="absolute inset-y-0 left-0 flex w-[min(20rem,85vw)] flex-col bg-ink text-paper"
          >
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                menuButton.current?.focus();
              }}
              className="absolute top-2.5 right-2.5 z-10 min-h-11 border-2 border-paper px-3 text-xs font-extrabold tracking-wider uppercase"
            >
              Close
            </button>
            <Nav me={me.data} />
          </div>
        </div>
      )}

      <main id="main" className="min-w-0 px-4 py-6 sm:px-8 lg:py-8">
        {children}
      </main>
    </div>
  );
}
