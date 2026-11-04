"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useId, useState, type FormEvent } from "react";
import { AccountHeader, Banner } from "@/components/account/account-header";
import { signInHref } from "@/lib/api";
import {
  client,
  displayName,
  initials,
  NotFound,
  NotSignedIn,
  raise,
  roles,
  useMe,
  type Invitation,
  type Member,
  type Organisation,
  type Role,
} from "@/lib/queries";

const field =
  "min-h-12 w-full min-w-0 rounded-2xl border-[3px] border-ink bg-paper px-4 text-base font-medium focus:ring-4 focus:ring-cobalt/40 focus:outline-none disabled:opacity-60";

function useOrganisation(orgID: string) {
  return useQuery({
    queryKey: ["organisation", orgID],
    queryFn: async (): Promise<Organisation> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}", {
        params: { path: { orgID } },
      });
      if (!data) raise(error, response.status, "Could not load this organisation.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && !(e instanceof NotSignedIn) && n < 2,
  });
}

export function OrganisationScreen() {
  const { orgID } = useParams<{ orgID: string }>();
  const org = useOrganisation(orgID);
  useMe(); // keeps the header's sign-out button in sync

  return (
    <main className="min-h-dvh px-4 py-6 sm:px-8">
      <AccountHeader />
      <div className="mx-auto mt-8 max-w-4xl space-y-8">
        <Link href="/account" className="inline-flex min-h-11 items-center font-extrabold underline decoration-2 underline-offset-4">
          ← Your account
        </Link>

        {org.isPending && (
          <div aria-busy="true" className="space-y-6">
            <span className="sr-only">Loading…</span>
            <div className="h-20 w-2/3 animate-pulse rounded-3xl bg-paper-deep" />
            <div className="h-64 animate-pulse rounded-3xl bg-paper-deep" />
          </div>
        )}
        {org.isError && org.error instanceof NotSignedIn && (
          <div className="sticker-lg rounded-[28px] bg-sun p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Sign in to see this organisation</h1>
            <a href={signInHref()} className="btn-pop mt-6 inline-flex min-h-12 items-center rounded-2xl bg-cobalt px-6 font-extrabold text-white">
              Sign in
            </a>
          </div>
        )}
        {org.isError && org.error instanceof NotFound && (
          <div className="sticker-lg rounded-[28px] bg-white p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Organisation not found</h1>
            <p className="mt-2 font-medium">It doesn’t exist, or you’re not a member of it.</p>
          </div>
        )}
        {org.isError && !(org.error instanceof NotFound) && !(org.error instanceof NotSignedIn) && (
          <div className="sticker-lg rounded-[28px] bg-white p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Something went wrong</h1>
            <p className="mt-2 font-medium">{org.error.message}</p>
            <button type="button" onClick={() => org.refetch()} className="btn-pop mt-6 min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white">
              Try again
            </button>
          </div>
        )}
        {org.data && <OrganisationView org={org.data} />}
      </div>
    </main>
  );
}

function can(org: Organisation, permission: string) {
  return org.permissions.includes(permission);
}

function OrganisationView({ org }: { org: Organisation }) {
  return (
    <>
      <Title org={org} />
      {org.personal && (
        <p className="sticker rounded-2xl bg-sky p-5 font-semibold">
          This is your personal studio, just for you. To work with collaborators or clients,{" "}
          <Link href="/account" className="font-extrabold underline decoration-2 underline-offset-4">
            start a team
          </Link>
          .
        </p>
      )}
      {can(org, "organisation.members.read") && <Members org={org} />}
      {!org.personal && can(org, "organisation.members.manage") && <Invite org={org} />}
    </>
  );
}

function Title({ org }: { org: Organisation }) {
  const id = useId();
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(org.name);
  const rename = useMutation({
    mutationFn: async (newName: string) => {
      const { data, error, response } = await client().PATCH("/api/v1/organisations/{orgID}", {
        params: { path: { orgID: org.id } },
        body: { name: newName },
      });
      if (!data) raise(error, response.status, "Could not rename the organisation.");
      return data;
    },
    onSuccess: async (updated) => {
      qc.setQueryData(["organisation", org.id], updated);
      await qc.invalidateQueries({ queryKey: ["me"] });
      setEditing(false);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim()) rename.mutate(name.trim());
  }

  return (
    <section>
      {editing ? (
        <form onSubmit={onSubmit} className="flex flex-col gap-3 sm:flex-row">
          <label htmlFor={`${id}-name`} className="sr-only">Organisation name</label>
          <input id={`${id}-name`} value={name} onChange={(e) => setName(e.target.value)} maxLength={120} required autoFocus className={field} disabled={rename.isPending} />
          <div className="flex gap-2">
            <button type="submit" disabled={rename.isPending} className="btn-pop min-h-12 flex-1 rounded-2xl bg-cobalt px-5 font-extrabold text-white disabled:opacity-60">
              Save
            </button>
            <button type="button" onClick={() => { setEditing(false); setName(org.name); }} className="btn-pop min-h-12 flex-1 rounded-2xl bg-white px-5 font-extrabold">
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="text-4xl leading-tight font-extrabold tracking-tight break-words sm:text-5xl">{org.name}</h1>
          {can(org, "organisation.manage") && (
            <button type="button" onClick={() => setEditing(true)} className="btn-pop min-h-11 rounded-full bg-white px-4 text-sm font-extrabold">
              Rename
            </button>
          )}
        </div>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        {org.personal && <span className="rounded-full border-2 border-ink bg-sun px-3 py-1 text-sm font-extrabold">Personal</span>}
        <span className="rounded-full border-2 border-ink bg-sky px-3 py-1 text-sm font-extrabold">
          You’re {/^[aeiou]/.test(org.role) ? "an" : "a"} <span className="capitalize">{org.role}</span>
        </span>
      </div>
      {rename.isError && <div className="mt-4"><Banner>{rename.error.message}</Banner></div>}
    </section>
  );
}

function Members({ org }: { org: Organisation }) {
  const qc = useQueryClient();
  const router = useRouter();
  const me = useMe();
  const members = useQuery({
    queryKey: ["organisation", org.id, "members"],
    queryFn: async (): Promise<Member[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/members", {
        params: { path: { orgID: org.id } },
      });
      if (!data) raise(error, response.status, "Could not load members.");
      return data.data;
    },
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ["organisation", org.id] });

  const changeRole = useMutation({
    mutationFn: async ({ userID, role }: { userID: string; role: Role }) => {
      const { error, response } = await client().PATCH("/api/v1/organisations/{orgID}/members/{userID}", {
        params: { path: { orgID: org.id, userID } },
        body: { role },
      });
      if (!response.ok) raise(error, response.status, "Could not change the role.");
    },
    onSettled: refresh,
  });

  const remove = useMutation({
    mutationFn: async (userID: string) => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/members/{userID}", {
        params: { path: { orgID: org.id, userID } },
      });
      if (!response.ok) raise(error, response.status, "Could not remove that person.");
      return userID;
    },
    onSuccess: async (userID) => {
      if (userID === me.data?.user.id) {
        await qc.invalidateQueries({ queryKey: ["me"] });
        router.push("/account");
        return;
      }
      await refresh();
    },
  });

  const manage = can(org, "organisation.members.manage");
  const iAmOwner = org.role === "owner";
  const error = changeRole.error ?? remove.error;

  return (
    <section aria-labelledby="members-h" className="sticker-lg rounded-[28px] bg-white p-5 sm:p-7">
      <h2 id="members-h" className="text-2xl font-extrabold tracking-tight">
        People {members.data && <span className="text-ink-soft">· {members.data.length}</span>}
      </h2>
      {error && <div className="mt-4"><Banner>{error.message}</Banner></div>}
      {members.isPending && <div className="mt-4 h-24 animate-pulse rounded-2xl bg-paper-deep" />}
      {members.isError && <div className="mt-4"><Banner>{members.error.message}</Banner></div>}
      {members.data && (
        <ul className="mt-4 divide-y-2 divide-ink/10">
          {members.data.map((m) => {
            const isMe = m.user_id === me.data?.user.id;
            const touchesOwner = m.role === "owner";
            const canEditThis = manage && !isMe && (!touchesOwner || iAmOwner) && !org.personal;
            const busy = (changeRole.isPending && changeRole.variables?.userID === m.user_id) || (remove.isPending && remove.variables === m.user_id);
            return (
              <li key={m.user_id} className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center">
                <div className="flex min-w-0 flex-1 items-center gap-3">
                  {m.avatar_url ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={m.avatar_url} alt="" referrerPolicy="no-referrer" className="size-11 shrink-0 rounded-full border-[3px] border-ink object-cover" />
                  ) : (
                    <span aria-hidden="true" className="grid size-11 shrink-0 place-items-center rounded-full border-[3px] border-ink bg-pink text-sm font-extrabold">
                      {initials(m)}
                    </span>
                  )}
                  <div className="min-w-0">
                    <p className="truncate font-extrabold">
                      {displayName(m)} {isMe && <span className="ml-1 rounded-full bg-sun px-2 py-0.5 text-xs">You</span>}
                    </p>
                    <p className="truncate text-sm font-medium text-ink-soft">{m.email}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {canEditThis ? (
                    <>
                      <label htmlFor={`role-${m.user_id}`} className="sr-only">Role for {displayName(m)}</label>
                      <select
                        id={`role-${m.user_id}`}
                        value={m.role}
                        disabled={busy}
                        onChange={(e) => changeRole.mutate({ userID: m.user_id, role: e.target.value as Role })}
                        className="min-h-11 rounded-xl border-[3px] border-ink bg-white px-3 font-bold disabled:opacity-60"
                      >
                        {roles.filter((r) => iAmOwner || r.value !== "owner").map((r) => (
                          <option key={r.value} value={r.value}>{r.label}</option>
                        ))}
                      </select>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => {
                          if (window.confirm(`Remove ${displayName(m)} from ${org.name}?`)) remove.mutate(m.user_id);
                        }}
                        className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold disabled:opacity-60"
                      >
                        Remove
                      </button>
                    </>
                  ) : (
                    <span className="rounded-full border-2 border-ink bg-sky px-3 py-1 text-sm font-extrabold capitalize">{m.role}</span>
                  )}
                  {isMe && !org.personal && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        if (window.confirm(`Leave ${org.name}? You’ll lose access straight away.`)) remove.mutate(m.user_id);
                      }}
                      className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold disabled:opacity-60"
                    >
                      Leave
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function Invite({ org }: { org: Organisation }) {
  const id = useId();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("editor");
  const [sentTo, setSentTo] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const iAmOwner = org.role === "owner";

  const invitations = useQuery({
    queryKey: ["organisation", org.id, "invitations"],
    queryFn: async (): Promise<Invitation[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/invitations", {
        params: { path: { orgID: org.id } },
      });
      if (!data) raise(error, response.status, "Could not load invitations.");
      return data.data;
    },
  });

  const invite = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/invitations", {
        params: { path: { orgID: org.id } },
        body: { email: email.trim(), role },
      });
      if (!data) raise(error, response.status, "Could not send the invitation.");
      return data;
    },
    onSuccess: async (inv) => {
      setSentTo(inv.email);
      setEmail("");
      setCopied(false);
      await qc.invalidateQueries({ queryKey: ["organisation", org.id, "invitations"] });
    },
  });

  const revoke = useMutation({
    mutationFn: async (invitationID: string) => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/invitations/{invitationID}", {
        params: { path: { orgID: org.id, invitationID } },
      });
      if (!response.ok) raise(error, response.status, "Could not revoke the invitation.");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["organisation", org.id, "invitations"] }),
  });

  const accountLink = typeof window === "undefined" ? "/account" : `${window.location.origin}/account`;

  return (
    <section aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-pink p-2">
      <div className="rounded-[20px] border-[3px] border-ink bg-white p-5 sm:p-6">
        <h2 id={`${id}-h`} className="text-2xl font-extrabold tracking-tight">Invite people</h2>
        <p className="mt-1 font-medium text-ink-soft">
          They’ll see the invitation on their account page after signing in to CAVI with this email.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            invite.mutate();
          }}
          className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_auto]"
        >
          <label htmlFor={`${id}-email`} className="sr-only">Email address</label>
          <input
            id={`${id}-email`}
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="teammate@studio.com"
            autoComplete="off"
            disabled={invite.isPending}
            className={field}
          />
          <label htmlFor={`${id}-role`} className="sr-only">Role</label>
          <select id={`${id}-role`} value={role} onChange={(e) => setRole(e.target.value as Role)} disabled={invite.isPending} className={`${field} font-bold`}>
            {roles.filter((r) => iAmOwner || r.value !== "owner").map((r) => (
              <option key={r.value} value={r.value}>{r.label} — {r.hint}</option>
            ))}
          </select>
          <button type="submit" disabled={invite.isPending} className="btn-pop min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold whitespace-nowrap text-white disabled:opacity-60">
            {invite.isPending ? "Inviting…" : "Invite"}
          </button>
        </form>

        {invite.isError && <div className="mt-4"><Banner>{invite.error.message}</Banner></div>}
        {sentTo && !invite.isError && (
          <div className="mt-4 rounded-2xl border-[3px] border-ink bg-mint p-4 font-semibold">
            <p>
              Invitation created for <span className="font-extrabold break-all">{sentTo}</span>. We don’t send emails
              yet, so let them know: they can sign in at
            </p>
            <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
              <code className="min-w-0 flex-1 truncate rounded-lg bg-white px-3 py-2 font-mono text-sm">{accountLink}</code>
              <button
                type="button"
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(accountLink);
                    setCopied(true);
                  } catch {
                    setCopied(false);
                  }
                }}
                className="btn-pop min-h-11 rounded-xl bg-white px-4 text-sm font-extrabold"
              >
                {copied ? "Copied!" : "Copy link"}
              </button>
            </div>
          </div>
        )}

        <h3 className="mt-6 text-lg font-extrabold">Pending invitations</h3>
        {revoke.isError && <div className="mt-3"><Banner>{revoke.error.message}</Banner></div>}
        {invitations.isPending && <div className="mt-3 h-14 animate-pulse rounded-2xl bg-paper-deep" />}
        {invitations.isError && <div className="mt-3"><Banner>{invitations.error.message}</Banner></div>}
        {invitations.data?.length === 0 && <p className="mt-2 font-medium text-ink-soft">No pending invitations.</p>}
        {invitations.data && invitations.data.length > 0 && (
          <ul className="mt-3 space-y-2">
            {invitations.data.map((inv) => (
              <li key={inv.id} className="flex flex-col gap-2 rounded-2xl border-2 border-ink bg-paper p-3 sm:flex-row sm:items-center">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-extrabold">{inv.email}</p>
                  <p className="text-sm font-medium text-ink-soft">
                    <span className="capitalize">{inv.role}</span> · expires {new Date(inv.expires_at).toLocaleDateString()}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={revoke.isPending && revoke.variables === inv.id}
                  onClick={() => revoke.mutate(inv.id)}
                  className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold disabled:opacity-60"
                >
                  Revoke
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
