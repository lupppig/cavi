"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useId, useState, type FormEvent } from "react";
import { Banner } from "@/components/account/account-header";
import { orgTabs } from "@/components/account/app-shell";
import { PageHeader, StatePanel } from "@/components/account/page-header";
import { GoogleSignIn } from "@/components/account/google-sign-in";
import {
  can,
  client,
  displayName,
  initials,
  NotFound,
  NotSignedIn,
  raise,
  roles,
  useMe,
  useMembers,
  useOrganisation,
  type Invitation,
  type Organisation,
  type Role,
} from "@/lib/queries";
import { AIModels } from "./ai-models-section";
import { Projects } from "./projects-section";

const field = "field";

export function OrganisationScreen() {
  const { orgID } = useParams<{ orgID: string }>();
  const org = useOrganisation(orgID);

  if (org.isPending) {
    return (
      <div aria-busy="true" className="mx-auto max-w-6xl space-y-6">
        <span className="sr-only">Loading…</span>
        <div className="h-24 w-2/3 animate-pulse bg-surface-2" />
        <div className="h-64 animate-pulse bg-surface-2" />
      </div>
    );
  }
  if (org.isError && org.error instanceof NotSignedIn) {
    return (
      <StatePanel title="Sign in to see this organisation">
        <GoogleSignIn className="mt-6 w-full sm:w-auto" />
      </StatePanel>
    );
  }
  if (org.isError && org.error instanceof NotFound) {
    return <StatePanel title="Organisation not found">It doesn’t exist, or you’re not a member of it.</StatePanel>;
  }
  if (org.isError) {
    return (
      <StatePanel title="Something went wrong">
        <p>{org.error.message}</p>
        <button type="button" onClick={() => org.refetch()} className="btn btn-primary mt-6 min-h-12 px-6">
          Try again
        </button>
      </StatePanel>
    );
  }
  return <OrganisationView org={org.data} />;
}

function OrganisationView({ org }: { org: Organisation }) {
  const search = useSearchParams();
  const tabs = orgTabs.filter(
    (t) =>
      t.tab === "projects" ||
      (t.tab === "people" && can(org, "organisation.members.read")) ||
      (t.tab === "settings" && can(org, "organisation.manage")),
  );
  const tab = tabs.find((t) => t.tab === search.get("tab"))?.tab ?? "projects";
  const role = `${/^[aeiou]/.test(org.role) ? "an" : "a"} ${org.role}`;

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <PageHeader
        crumbs={[{ label: "Home", href: "/account" }]}
        title={org.name}
        subtitle={org.personal ? `Your personal studio · you’re ${role}` : `Team · you’re ${role}`}
      />
      {tabs.length > 1 && (
        <nav aria-label="Organisation sections" className="flex gap-1 overflow-x-auto border-b border-line">
          {tabs.map((t) => (
            <Link
              key={t.tab}
              href={t.tab === "projects" ? `/account/organisations/${org.id}` : `/account/organisations/${org.id}?tab=${t.tab}`}
              aria-current={tab === t.tab ? "page" : undefined}
              className={`flex min-h-11 shrink-0 items-center px-3 text-sm font-semibold transition-colors ${
                tab === t.tab ? "text-ink shadow-[inset_0_-2px_0_var(--accent)]" : "text-muted hover:text-ink"
              }`}
            >
              {t.label}
            </Link>
          ))}
        </nav>
      )}

      {tab === "projects" && (
        <>
          {org.personal && (
            <p className="rounded-xl border border-accent-line bg-accent-soft p-4 text-sm font-medium">
              Your personal studio is just for you. To work with collaborators or clients,{" "}
              <Link href="/account" className="font-semibold underline decoration-2 underline-offset-4">
                start a team
              </Link>
              .
            </p>
          )}
          <Projects org={org} />
        </>
      )}
      {tab === "people" && (
        <>
          <Members org={org} />
          {!org.personal && can(org, "organisation.members.manage") && <Invite org={org} />}
        </>
      )}
      {tab === "settings" && (
        <>
          <Rename org={org} />
          <AIModels org={org} />
        </>
      )}
    </div>
  );
}

function Rename({ org }: { org: Organisation }) {
  const id = useId();
  const qc = useQueryClient();
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
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim()) rename.mutate(name.trim());
  }

  return (
    <section aria-labelledby={`${id}-h`} className="card p-5 sm:p-7">
      <h2 id={`${id}-h`} className="text-2xl">
        Name
      </h2>
      <form onSubmit={onSubmit} className="mt-4 flex flex-col gap-3 sm:flex-row">
        <label htmlFor={`${id}-name`} className="sr-only">
          Organisation name
        </label>
        <input id={`${id}-name`} value={name} onChange={(e) => setName(e.target.value)} maxLength={120} required className={field} disabled={rename.isPending} />
        <button
          type="submit"
          disabled={rename.isPending || name.trim() === org.name || !name.trim()}
          className="btn btn-primary min-h-12 px-6 whitespace-nowrap disabled:opacity-60"
        >
          {rename.isPending ? "Saving…" : "Save name"}
        </button>
      </form>
      {rename.isSuccess && name.trim() === org.name && <div className="mt-3"><Banner tone="ok">Saved.</Banner></div>}
      {rename.isError && <div className="mt-3"><Banner>{rename.error.message}</Banner></div>}
    </section>
  );
}

function Members({ org }: { org: Organisation }) {
  const qc = useQueryClient();
  const router = useRouter();
  const me = useMe();
  const members = useMembers(org.id);

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
    <section aria-labelledby="members-h" className="card bg-white p-5 sm:p-7">
      <h2 id="members-h" className="text-2xl font-semibold tracking-tight">
        People {members.data && <span className="text-muted">· {members.data.length}</span>}
      </h2>
      {error && <div className="mt-4"><Banner>{error.message}</Banner></div>}
      {members.isPending && <div className="mt-4 h-24 animate-pulse bg-surface-2" />}
      {members.isError && <div className="mt-4"><Banner>{members.error.message}</Banner></div>}
      {members.data && (
        <ul className="mt-4 divide-y-2 divide-line">
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
                    <img src={m.avatar_url} alt="" referrerPolicy="no-referrer" className="size-11 shrink-0 rounded-full border border-line object-cover" />
                  ) : (
                    <span aria-hidden="true" className="grid size-11 shrink-0 place-items-center rounded-full bg-accent-soft text-sm font-semibold text-accent-text">
                      {initials(m)}
                    </span>
                  )}
                  <div className="min-w-0">
                    <p className="truncate font-semibold">
                      {displayName(m)} {isMe && <span className="ml-1 bg-surface-2 px-2 py-0.5 text-xs">You</span>}
                    </p>
                    <p className="truncate text-sm font-medium text-muted">{m.email}</p>
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
                        className="min-h-11 border border-line bg-white px-3 font-bold disabled:opacity-60"
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
                        className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
                      >
                        Remove
                      </button>
                    </>
                  ) : (
                    <span className="border border-line bg-surface-2 px-3 py-1 text-sm font-semibold capitalize">{m.role}</span>
                  )}
                  {isMe && !org.personal && (
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        if (window.confirm(`Leave ${org.name}? You’ll lose access straight away.`)) remove.mutate(m.user_id);
                      }}
                      className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
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
    <section aria-labelledby={`${id}-h`} className="card p-5 sm:p-6">
      <div>
        <h2 id={`${id}-h`} className="text-2xl font-semibold tracking-tight">Invite people</h2>
        <p className="mt-1 font-medium text-muted">
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
          <button type="submit" disabled={invite.isPending} className="btn btn-primary min-h-12 px-6 whitespace-nowrap disabled:opacity-60">
            {invite.isPending ? "Inviting…" : "Invite"}
          </button>
        </form>

        {invite.isError && <div className="mt-4"><Banner>{invite.error.message}</Banner></div>}
        {sentTo && !invite.isError && (
          <div className="mt-4 border border-line bg-surface-2 p-4 font-semibold">
            <p>
              Invitation created for <span className="font-semibold break-all">{sentTo}</span>. We don’t send emails
              yet, so let them know: they can sign in at
            </p>
            <div className="mt-2 flex flex-col gap-2 sm:flex-row sm:items-center">
              <code className="min-w-0 flex-1 truncate bg-white px-3 py-2 font-mono text-sm">{accountLink}</code>
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
                className="btn btn-secondary min-h-11 px-4 text-sm"
              >
                {copied ? "Copied!" : "Copy link"}
              </button>
            </div>
          </div>
        )}

        <h3 className="mt-6 text-lg font-semibold">Pending invitations</h3>
        {revoke.isError && <div className="mt-3"><Banner>{revoke.error.message}</Banner></div>}
        {invitations.isPending && <div className="mt-3 h-14 animate-pulse bg-surface-2" />}
        {invitations.isError && <div className="mt-3"><Banner>{invitations.error.message}</Banner></div>}
        {invitations.data?.length === 0 && <p className="mt-2 font-medium text-muted">No pending invitations.</p>}
        {invitations.data && invitations.data.length > 0 && (
          <ul className="mt-3 space-y-2">
            {invitations.data.map((inv) => (
              <li key={inv.id} className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2 p-3 sm:flex-row sm:items-center">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-semibold">{inv.email}</p>
                  <p className="text-sm font-medium text-muted">
                    <span className="capitalize">{inv.role}</span> · expires {new Date(inv.expires_at).toLocaleDateString()}
                  </p>
                </div>
                <button
                  type="button"
                  disabled={revoke.isPending && revoke.variables === inv.id}
                  onClick={() => revoke.mutate(inv.id)}
                  className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
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
