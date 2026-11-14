"use client";

import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { Mark } from "@/components/home/site-header";
import { client, displayName, raise, useMe } from "@/lib/queries";

/** The signed-in app's header: a black bar matching the homepage. */
export function AccountHeader() {
  const me = useMe();
  const signOut = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/auth/logout");
      if (!data) raise(error, response.status, "Could not sign out.");
      return data.redirect_url;
    },
    onSuccess: (url) => window.location.assign(url),
  });

  return (
    <>
      <header className="mx-auto flex min-h-16 max-w-4xl items-center justify-between gap-3 border-2 border-ink bg-ink px-3 text-paper sm:px-4">
        <Link href="/" className="flex min-h-11 items-center gap-2.5 text-xl font-black tracking-tight [font-stretch:125%]">
          <Mark />
          CAVI
        </Link>
        {me.data && (
          <div className="flex min-w-0 items-center gap-3">
            <span className="hidden truncate font-mono text-xs uppercase sm:block">{displayName(me.data.user)}</span>
            <button
              type="button"
              onClick={() => signOut.mutate()}
              disabled={signOut.isPending}
              className="min-h-11 border-2 border-paper px-4 text-xs font-extrabold tracking-wider uppercase transition-colors hover:bg-paper hover:text-ink disabled:opacity-60"
            >
              {signOut.isPending ? "Signing out…" : "Sign out"}
            </button>
          </div>
        )}
      </header>
      {signOut.isError && (
        <div className="mx-auto mt-6 max-w-4xl">
          <Banner>{signOut.error.message}</Banner>
        </div>
      )}
    </>
  );
}

/**
 * A message strip. Errors are solid black with a "!" marker so they stand
 * out without colour; confirmations are a light panel with a tick.
 */
export function Banner({ tone = "error", children }: { tone?: "error" | "ok"; children: React.ReactNode }) {
  const error = tone === "error";
  return (
    <p
      role={error ? "alert" : "status"}
      className={`flex items-start gap-3 border-2 border-ink px-4 py-3 font-bold ${error ? "bg-ink text-paper" : "bg-paper-deep"}`}
    >
      <span
        aria-hidden="true"
        className={`grid size-6 shrink-0 place-items-center font-mono text-sm font-black ${error ? "bg-paper text-ink" : "bg-ink text-paper"}`}
      >
        {error ? "!" : "✓"}
      </span>
      <span className="min-w-0 pt-px">{children}</span>
    </p>
  );
}
