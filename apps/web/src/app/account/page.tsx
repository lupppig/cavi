"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useState, useSyncExternalStore, type FormEvent } from "react";
import { AccountHeader, Banner } from "@/components/account/account-header";
import { GoogleSignIn } from "@/components/account/google-sign-in";
import {
  client,
  displayName,
  initials,
  NotSignedIn,
  raise,
  useMe,
  useMyInvitations,
  type Invitation,
  type Me,
} from "@/lib/queries";

const authErrors: Record<string, string> = {
  cancelled: "Sign-in was cancelled. You can try again whenever you’re ready.",
  invalid_state: "That sign-in link expired or was opened in a different browser. Please start again.",
  sign_in_failed: "We couldn’t sign you in with Google. Please try again.",
};

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
  useEffect(() => {
    if (code) window.history.replaceState(null, "", window.location.pathname);
    return () => {
      capturedAuthError = undefined;
    };
  }, [code]);

  const me = useMe();

  return (
    <main className="min-h-dvh px-4 py-6 sm:px-8">
      <AccountHeader />
      <div className="mx-auto mt-10 max-w-4xl space-y-8 sm:mt-14">
        {authError && <Banner>{authError}</Banner>}
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
    <div aria-busy="true" className="space-y-6">
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
        Sign in with your Google account to see your account and workspaces. New here? Your account is
        created the first time you sign in.
      </p>
      <GoogleSignIn className="mt-8 w-full sm:w-auto" />
    </div>
  );
}

function SignedIn({ me }: { me: Me }) {
  const name = displayName(me.user);
  return (
    <>
      <section className="flex flex-col gap-5 sm:flex-row sm:items-center">
        {me.user.avatar_url ? (
          // Avatars come from the identity provider's CDN; a plain img avoids
          // configuring every possible remote host for next/image.
          // eslint-disable-next-line @next/next/no-img-element
          <img src={me.user.avatar_url} alt="" referrerPolicy="no-referrer" className="sticker size-20 shrink-0 rounded-full object-cover" />
        ) : (
          <span aria-hidden="true" className="sticker grid size-20 shrink-0 place-items-center rounded-full bg-pink text-2xl font-extrabold">
            {initials(me.user)}
          </span>
        )}
        <div className="min-w-0">
          <h1 className="text-4xl leading-tight font-extrabold tracking-tight break-words sm:text-5xl">
            Hey{me.user.first_name ? `, ${me.user.first_name}` : ""}!
          </h1>
          <p className="mt-1 font-semibold break-all text-ink-soft">
            {name !== me.user.email ? `${name} · ` : ""}
            {me.user.email}
          </p>
        </div>
      </section>

      <PendingInvitations />

      <section aria-labelledby="orgs-heading">
        <h2 id="orgs-heading" className="text-2xl font-extrabold tracking-tight">
          Your organisations
        </h2>
        <ul className="mt-5 grid grid-cols-1 gap-6 md:grid-cols-2">
          {me.organisations.map((o) => (
            <li key={o.id}>
              <Link
                href={`/account/organisations/${o.id}`}
                className="tilt-hover sticker-lg block rounded-[28px] bg-mint p-2 focus-visible:outline-offset-4"
              >
                <span className="block rounded-[20px] border-[3px] border-ink bg-white p-5">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="mr-auto text-xl font-extrabold tracking-tight break-words">{o.name}</span>
                    {o.personal && <span className="rounded-full border-2 border-ink bg-sun px-2.5 py-0.5 text-xs font-extrabold">Personal</span>}
                    <span className="rounded-full border-2 border-ink bg-sky px-2.5 py-0.5 text-xs font-extrabold capitalize">{o.role}</span>
                  </span>
                  <span className="mt-3 block text-sm font-semibold text-ink-soft">
                    {o.workspaces.length} workspace{o.workspaces.length === 1 ? "" : "s"} · {o.workspaces.map((w) => w.name).join(", ")}
                  </span>
                  <span className="mt-4 block font-extrabold">Open →</span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>

      <CreateTeam />

      <p className="rounded-2xl border-[3px] border-dashed border-ink bg-white/60 p-5 font-semibold">
        Open an organisation to see its projects. CAVI is still being built: uploads, editing and renders will
        arrive in projects as they’re ready.
      </p>
    </>
  );
}

function PendingInvitations() {
  const invitations = useMyInvitations(true);
  const qc = useQueryClient();
  const router = useRouter();
  const respond = useMutation({
    mutationFn: async ({ inv, accept }: { inv: Invitation; accept: boolean }) => {
      const path = accept ? "/api/v1/invitations/{invitationID}/accept" : "/api/v1/invitations/{invitationID}/decline";
      const { error, response } = await client().POST(path, { params: { path: { invitationID: inv.id } } });
      if (!response.ok) raise(error, response.status, "Could not respond to the invitation.");
      return { inv, accept };
    },
    onSuccess: async ({ inv, accept }) => {
      await qc.invalidateQueries({ queryKey: ["invitations"] });
      await qc.invalidateQueries({ queryKey: ["me"] });
      if (accept) router.push(`/account/organisations/${inv.organisation_id}`);
    },
  });

  if (!invitations.data?.length) return null;
  return (
    <section aria-labelledby="inv-heading" className="sticker-lg rounded-[28px] bg-sun p-5 sm:p-7">
      <h2 id="inv-heading" className="text-2xl font-extrabold tracking-tight">
        You’ve been invited 🎉
      </h2>
      {respond.isError && <div className="mt-4"><Banner>{respond.error.message}</Banner></div>}
      <ul className="mt-4 space-y-3">
        {invitations.data.map((inv) => {
          const busy = respond.isPending && respond.variables?.inv.id === inv.id;
          return (
            <li key={inv.id} className="flex flex-col gap-3 rounded-2xl border-[3px] border-ink bg-white p-4 sm:flex-row sm:items-center">
              <p className="min-w-0 flex-1 font-semibold">
                <span className="font-extrabold">{inv.invited_by_name || "Someone"}</span> invited you to{" "}
                <span className="font-extrabold">{inv.organisation_name}</span> as{" "}
                <span className="capitalize">{inv.role}</span>.
              </p>
              <div className="flex gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => respond.mutate({ inv, accept: true })}
                  className="btn-pop min-h-11 flex-1 rounded-xl bg-cobalt px-4 font-extrabold text-white disabled:opacity-60 sm:flex-none"
                >
                  Join
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => respond.mutate({ inv, accept: false })}
                  className="btn-pop min-h-11 flex-1 rounded-xl bg-white px-4 font-extrabold disabled:opacity-60 sm:flex-none"
                >
                  Decline
                </button>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function CreateTeam() {
  const id = useId();
  const router = useRouter();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: async (teamName: string) => {
      const { data, error, response } = await client().POST("/api/v1/organisations", { body: { name: teamName } });
      if (!data) raise(error, response.status, "Could not create the team.");
      return data;
    },
    onSuccess: async (org) => {
      await qc.invalidateQueries({ queryKey: ["me"] });
      router.push(`/account/organisations/${org.id}`);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim()) create.mutate(name.trim());
  }

  return (
    <section aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-white p-5 sm:p-7">
      <h2 id={`${id}-h`} className="text-2xl font-extrabold tracking-tight">
        Start a team
      </h2>
      <p className="mt-1 font-medium text-ink-soft">
        Teams let you invite collaborators and clients. Your personal studio stays just yours.
      </p>
      <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-3 sm:flex-row">
        <label htmlFor={`${id}-name`} className="sr-only">
          Team name
        </label>
        <input
          id={`${id}-name`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={120}
          required
          placeholder="e.g. Night Shift Studio"
          disabled={create.isPending}
          className="min-h-12 w-full min-w-0 flex-1 rounded-2xl border-[3px] border-ink bg-paper px-4 text-base font-medium focus:ring-4 focus:ring-cobalt/40 focus:outline-none"
        />
        <button
          type="submit"
          disabled={create.isPending || !name.trim()}
          className="btn-pop min-h-12 rounded-2xl bg-tomato px-6 font-extrabold whitespace-nowrap text-white disabled:opacity-60"
        >
          {create.isPending ? "Creating…" : "Create team"}
        </button>
      </form>
      {create.isError && <div className="mt-3"><Banner>{create.error.message}</Banner></div>}
    </section>
  );
}
