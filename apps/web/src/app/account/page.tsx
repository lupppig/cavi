"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useEffect, useSyncExternalStore } from "react";
import type { Me, OrganisationMembership } from "@cavi/api-client";
import { api, signInHref } from "@/lib/api";

const authErrors: Record<string, string> = {
  cancelled: "Sign-in was cancelled. You can try again whenever you’re ready.",
  invalid_state:
    "That sign-in link expired or was opened in a different browser. Please start again.",
  sign_in_failed: "We couldn’t sign you in. Please try again.",
};

class NotSignedIn extends Error {}

async function fetchMe(): Promise<Me> {
  if (!api) throw new Error("The CAVI API is not configured.");
  const { data, error, response } = await api.GET("/api/v1/me");
  if (response.status === 401) throw new NotSignedIn();
  if (error || !data) throw new Error(error?.error.message ?? "Could not load your account.");
  return data;
}

function Logo() {
  return (
    <svg viewBox="0 0 32 32" className="size-8" aria-hidden="true">
      <rect x="2" y="2" width="28" height="28" rx="9" fill="var(--tomato)" stroke="var(--ink)" strokeWidth="3" />
      <path d="M12 9.5 L23 16 L12 22.5 Z" fill="white" stroke="var(--ink)" strokeWidth="2.5" strokeLinejoin="round" />
    </svg>
  );
}

// The ?auth_error code captured when the page first loads. Cached so it
// survives removing it from the URL, and cleared when the page unmounts.
let capturedAuthError: string | null | undefined;
const readAuthError = () => {
  if (capturedAuthError === undefined) {
    capturedAuthError = new URLSearchParams(window.location.search).get("auth_error");
  }
  return capturedAuthError;
};
const noSubscribe = () => () => {};

export default function AccountPage() {
  const code = useSyncExternalStore(noSubscribe, readAuthError, () => null);
  const authError = code ? (authErrors[code] ?? authErrors.sign_in_failed) : null;

  // Remove ?auth_error from the URL so a refresh doesn't repeat the message.
  useEffect(() => {
    if (code) window.history.replaceState(null, "", window.location.pathname);
    return () => {
      capturedAuthError = undefined;
    };
  }, [code]);

  const me = useQuery({ queryKey: ["me"], queryFn: fetchMe, retry: (n, e) => !(e instanceof NotSignedIn) && n < 2 });

  const signOut = useMutation({
    mutationFn: async () => {
      if (!api) throw new Error("The CAVI API is not configured.");
      const { data, error } = await api.POST("/api/v1/auth/logout");
      if (error || !data) throw new Error(error?.error.message ?? "Could not sign out.");
      return data.redirect_url;
    },
    onSuccess: (url) => window.location.assign(url),
  });

  return (
    <main className="min-h-dvh px-4 py-6 sm:px-8">
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

      <div className="mx-auto mt-10 max-w-4xl sm:mt-14">
        {authError && (
          <p role="alert" className="sticker mb-8 rounded-2xl bg-tomato px-4 py-3 font-bold text-white">
            {authError}
          </p>
        )}
        {signOut.isError && (
          <p role="alert" className="sticker mb-8 rounded-2xl bg-tomato px-4 py-3 font-bold text-white">
            {signOut.error.message}
          </p>
        )}

        {me.isPending && <Loading />}
        {me.isError && me.error instanceof NotSignedIn && <SignedOut />}
        {me.isError && !(me.error instanceof NotSignedIn) && (
          <div className="sticker-lg rounded-3xl bg-white p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold tracking-tight">We couldn’t load your account</h1>
            <p className="mt-2 font-medium">{me.error.message}</p>
            <button
              type="button"
              onClick={() => me.refetch()}
              className="btn-pop mt-6 min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white"
            >
              Try again
            </button>
          </div>
        )}
        {me.data && <SignedIn me={me.data} />}
      </div>
    </main>
  );
}

function Loading() {
  return (
    <div aria-busy="true" aria-live="polite" className="space-y-6">
      <span className="sr-only">Loading your account…</span>
      <div className="h-24 w-2/3 animate-pulse rounded-3xl bg-paper-deep" />
      <div className="h-48 animate-pulse rounded-3xl bg-paper-deep" />
    </div>
  );
}

function SignedOut() {
  return (
    <div className="sticker-lg rounded-[28px] bg-sun p-6 sm:p-10">
      <h1 className="text-4xl leading-tight font-extrabold tracking-tight sm:text-5xl">You’re not signed in</h1>
      <p className="mt-3 max-w-xl text-lg font-medium">
        Sign in to see your account and workspaces. New here? Creating an account takes a few seconds.
      </p>
      <div className="mt-8 flex flex-col gap-3 sm:flex-row">
        <a
          href={signInHref("sign-in")}
          className="btn-pop flex min-h-12 items-center justify-center rounded-2xl bg-cobalt px-6 font-extrabold text-white"
        >
          Sign in
        </a>
        <a
          href={signInHref("sign-up")}
          className="btn-pop flex min-h-12 items-center justify-center rounded-2xl bg-white px-6 font-extrabold"
        >
          Create an account
        </a>
      </div>
    </div>
  );
}

function initials(me: Me) {
  const a = me.user.first_name.trim()[0] ?? me.user.email[0] ?? "?";
  const b = me.user.last_name.trim()[0] ?? "";
  return (a + b).toUpperCase();
}

function SignedIn({ me }: { me: Me }) {
  const name = [me.user.first_name, me.user.last_name].filter(Boolean).join(" ");
  return (
    <>
      <section className="flex flex-col gap-5 sm:flex-row sm:items-center">
        {me.user.avatar_url ? (
          // Avatars come from the identity provider's CDN; a plain img avoids
          // configuring every possible remote host for next/image.
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={me.user.avatar_url}
            alt=""
            referrerPolicy="no-referrer"
            className="sticker size-20 shrink-0 rounded-full object-cover"
          />
        ) : (
          <span aria-hidden="true" className="sticker grid size-20 shrink-0 place-items-center rounded-full bg-pink text-2xl font-extrabold">
            {initials(me)}
          </span>
        )}
        <div className="min-w-0">
          <h1 className="text-4xl leading-tight font-extrabold tracking-tight break-words sm:text-5xl">
            Hey{me.user.first_name ? `, ${me.user.first_name}` : ""}!
          </h1>
          <p className="mt-1 font-semibold break-all text-ink-soft">
            {name ? `${name} · ` : ""}
            {me.user.email}
          </p>
        </div>
      </section>

      <section className="mt-12" aria-labelledby="orgs-heading">
        <h2 id="orgs-heading" className="text-2xl font-extrabold tracking-tight">
          Your organisations
        </h2>
        {me.organisations.length === 0 ? (
          <p className="mt-3 font-medium">You don’t belong to any organisations yet.</p>
        ) : (
          <ul className="mt-5 grid grid-cols-1 gap-6 md:grid-cols-2">
            {me.organisations.map((o) => (
              <Organisation key={o.id} org={o} />
            ))}
          </ul>
        )}
      </section>

      <p className="mt-12 rounded-2xl border-[3px] border-dashed border-ink bg-white/60 p-5 font-semibold">
        CAVI is still being built. Projects, uploads and editing will show up here as they’re
        ready, and we’ll email you when early access opens.
      </p>
    </>
  );
}

function Organisation({ org }: { org: OrganisationMembership }) {
  return (
    <li className="sticker-lg rounded-[28px] bg-mint p-2">
      <div className="rounded-[20px] border-[3px] border-ink bg-white p-5">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="mr-auto text-xl font-extrabold tracking-tight break-words">{org.name}</h3>
          {org.personal && (
            <span className="rounded-full border-2 border-ink bg-sun px-2.5 py-0.5 text-xs font-extrabold">Personal</span>
          )}
          <span className="rounded-full border-2 border-ink bg-sky px-2.5 py-0.5 text-xs font-extrabold capitalize">
            {org.role}
          </span>
        </div>
        <p className="mt-4 text-sm font-extrabold tracking-wide text-ink-soft uppercase">Workspaces</p>
        <ul className="mt-2 flex flex-wrap gap-2">
          {org.workspaces.map((w) => (
            <li key={w.id} className="rounded-xl border-2 border-ink bg-paper px-3 py-1.5 font-bold">
              {w.name}
            </li>
          ))}
          {org.workspaces.length === 0 && <li className="font-medium">No workspaces yet.</li>}
        </ul>
      </div>
    </li>
  );
}
