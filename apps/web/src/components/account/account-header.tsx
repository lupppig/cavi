"use client";

import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { client, raise, useMe } from "@/lib/queries";

function Logo() {
  return (
    <svg viewBox="0 0 32 32" className="size-8" aria-hidden="true">
      <rect x="2" y="2" width="28" height="28" rx="9" fill="var(--tomato)" stroke="var(--ink)" strokeWidth="3" />
      <path d="M12 9.5 L23 16 L12 22.5 Z" fill="white" stroke="var(--ink)" strokeWidth="2.5" strokeLinejoin="round" />
    </svg>
  );
}

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
      <header className="sticker mx-auto flex h-16 max-w-4xl items-center justify-between gap-3 rounded-full bg-white pr-2 pl-3 sm:pl-4">
        <Link href="/" className="flex min-h-11 items-center gap-2 text-2xl font-extrabold tracking-tight">
          <Logo />
          CAVI
        </Link>
        {me.data && (
          <button
            type="button"
            onClick={() => signOut.mutate()}
            disabled={signOut.isPending}
            className="btn-pop min-h-11 rounded-full bg-white px-4 text-sm font-extrabold disabled:opacity-60 sm:px-5 sm:text-base"
          >
            {signOut.isPending ? "Signing out…" : "Sign out"}
          </button>
        )}
      </header>
      {signOut.isError && (
        <p role="alert" className="sticker mx-auto mt-6 max-w-4xl rounded-2xl bg-tomato px-4 py-3 font-bold text-white">
          {signOut.error.message}
        </p>
      )}
    </>
  );
}

export function Banner({ tone = "error", children }: { tone?: "error" | "ok"; children: React.ReactNode }) {
  return (
    <p
      role={tone === "error" ? "alert" : "status"}
      className={`sticker rounded-2xl px-4 py-3 font-bold ${tone === "error" ? "bg-tomato text-white" : "bg-mint"}`}
    >
      {children}
    </p>
  );
}
