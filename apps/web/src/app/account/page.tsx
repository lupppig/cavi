"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useState, useSyncExternalStore, type FormEvent } from "react";
import { Banner } from "@/components/account/account-header";
import { PageHeader, StatePanel } from "@/components/account/page-header";
import { GoogleSignIn } from "@/components/account/google-sign-in";
import {
  client,
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
    <div className="mx-auto max-w-5xl space-y-10">
      {authError && <Banner>{authError}</Banner>}
      {me.isPending && <Loading />}
      {me.isError && me.error instanceof NotSignedIn && <SignedOut />}
      {me.isError && !(me.error instanceof NotSignedIn) && (
        <StatePanel title="We couldn’t load your account">
          <p>{me.error.message}</p>
          <button type="button" onClick={() => me.refetch()} className="btn-pop mt-6 min-h-12 bg-ink px-6 font-extrabold text-white">
            Try again
          </button>
        </StatePanel>
      )}
      {me.data && <SignedIn me={me.data} />}
    </div>
  );
}

function Loading() {
  return (
    <div aria-busy="true" className="space-y-6">
      <span className="sr-only">Loading your account…</span>
      <div className="h-24 w-2/3 animate-pulse bg-paper-deep" />
      <div className="h-48 animate-pulse bg-paper-deep" />
    </div>
  );
}

function SignedOut() {
  return (
    <StatePanel title="Sign in to CAVI">
      <p>Use your Google account. New here? Your account and personal studio are created the first time you sign in.</p>
      <GoogleSignIn className="mt-8 w-full sm:w-auto" />
    </StatePanel>
  );
}

function SignedIn({ me }: { me: Me }) {
  return (
    <>
      <PageHeader title={`Welcome back${me.user.first_name ? `, ${me.user.first_name}` : ""}.`} subtitle="Pick up where you left off." />
      <PendingInvitations />
      <section aria-labelledby="orgs-heading">
        <h2 id="orgs-heading" className="text-2xl">
          Your organisations
        </h2>
        <ul className="mt-4 border-b-2 border-ink">
          {me.organisations.map((o) => (
            <li key={o.id}>
              <Link
                href={`/account/organisations/${o.id}`}
                className="group flex min-h-16 items-center gap-4 border-t-2 border-ink px-2 py-3 transition-colors hover:bg-ink hover:text-paper sm:px-4"
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-lg font-extrabold">{o.name}</span>
                  <span className="block truncate font-mono text-xs font-semibold uppercase opacity-80">
                    {o.personal ? "Personal studio" : "Team"} · {o.role}
                  </span>
                </span>
                <span aria-hidden="true" className="text-xl font-black transition-transform group-hover:translate-x-1">
                  →
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
      <CreateTeam />
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
    <section aria-labelledby="inv-heading" className="border-2 border-ink bg-white p-5 sm:p-7">
      <h2 id="inv-heading" className="text-2xl">
        You’ve been invited
      </h2>
      {respond.isError && <div className="mt-4"><Banner>{respond.error.message}</Banner></div>}
      <ul className="mt-4 space-y-3">
        {invitations.data.map((inv) => {
          const busy = respond.isPending && respond.variables?.inv.id === inv.id;
          return (
            <li key={inv.id} className="flex flex-col gap-3 border-2 border-ink bg-white p-4 sm:flex-row sm:items-center">
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
                  className="btn-pop min-h-11 flex-1 bg-ink px-4 font-extrabold text-white disabled:opacity-60 sm:flex-none"
                >
                  Join
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => respond.mutate({ inv, accept: false })}
                  className="btn-pop min-h-11 flex-1 bg-white px-4 font-extrabold disabled:opacity-60 sm:flex-none"
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
    <section aria-labelledby={`${id}-h`} className="border-2 border-ink bg-white p-5 sm:p-7">
      <h2 id={`${id}-h`} className="text-2xl">
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
          className="min-h-12 w-full min-w-0 flex-1 border-2 border-ink bg-paper px-4 text-base font-medium focus:ring-4 focus:ring-ink/30 focus:outline-none"
        />
        <button
          type="submit"
          disabled={create.isPending || !name.trim()}
          className="btn-pop min-h-12 bg-ink px-6 font-extrabold whitespace-nowrap text-white disabled:opacity-60"
        >
          {create.isPending ? "Creating…" : "Create team"}
        </button>
      </form>
      {create.isError && <div className="mt-3"><Banner>{create.error.message}</Banner></div>}
    </section>
  );
}
