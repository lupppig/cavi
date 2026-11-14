"use client";

import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { Avatar } from "@/components/account/app-shell";
import { Banner } from "@/components/account/account-header";
import { GoogleSignIn } from "@/components/account/google-sign-in";
import { PageHeader, StatePanel } from "@/components/account/page-header";
import { client, displayName, NotSignedIn, raise, roles, useMe } from "@/lib/queries";

/** Who you are in CAVI, how you sign in, and where you belong. */
export default function ProfilePage() {
  const me = useMe();
  const signOut = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/auth/logout");
      if (!data) raise(error, response.status, "Could not sign out.");
      return data.redirect_url;
    },
    onSuccess: (url) => window.location.assign(url),
  });

  if (me.isPending) return <div aria-busy="true" className="mx-auto h-64 max-w-5xl animate-pulse bg-paper-deep" />;
  if (me.isError && me.error instanceof NotSignedIn) {
    return (
      <StatePanel title="Sign in to see your profile">
        <GoogleSignIn className="mt-6 w-full sm:w-auto" />
      </StatePanel>
    );
  }
  if (!me.data) return <Banner>{me.error?.message ?? "We couldn’t load your profile."}</Banner>;
  const { user, organisations } = me.data;

  return (
    <div className="mx-auto max-w-5xl space-y-10">
      <PageHeader crumbs={[{ label: "Home", href: "/account" }]} title="Profile" />

      <section aria-labelledby="you-h" className="grid gap-6 border-2 border-ink bg-white p-5 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-center sm:p-8">
        <span className="text-3xl">
          <Avatar user={user} size="size-24" onDark={false} />
        </span>
        <div className="min-w-0">
          <h2 id="you-h" className="text-3xl break-words">
            {displayName(user)}
          </h2>
          <p className="mt-1 font-semibold break-all text-ink-soft">{user.email}</p>
        </div>
        <dl className="grid gap-4 border-t-2 border-ink pt-5 sm:col-span-2 sm:grid-cols-2">
          <div>
            <dt className="font-mono text-xs font-semibold uppercase text-ink-soft">Sign-in</dt>
            <dd className="mt-1 font-extrabold">Google</dd>
          </div>
          <div>
            <dt className="font-mono text-xs font-semibold uppercase text-ink-soft">Name, email and photo</dt>
            <dd className="mt-1 font-medium">
              Come from your Google account and refresh each time you sign in. Change them in Google.
            </dd>
          </div>
        </dl>
      </section>

      <section aria-labelledby="orgs-h">
        <h2 id="orgs-h" className="text-2xl">
          Organisations
        </h2>
        <div className="mt-4 overflow-x-auto border-2 border-ink bg-white">
          <table className="w-full min-w-[28rem] text-left">
            <thead className="bg-ink font-mono text-xs uppercase text-paper">
              <tr>
                <th scope="col" className="px-4 py-3 font-semibold">Organisation</th>
                <th scope="col" className="px-4 py-3 font-semibold">Your role</th>
                <th scope="col" className="px-4 py-3 font-semibold">Can</th>
              </tr>
            </thead>
            <tbody>
              {organisations.map((o) => (
                <tr key={o.id} className="border-t-2 border-ink">
                  <td className="px-4 py-3">
                    <Link href={`/account/organisations/${o.id}`} className="inline-flex min-h-11 items-center font-extrabold underline-offset-4 hover:underline">
                      {o.name}
                    </Link>
                    {o.personal && <span className="ml-2 font-mono text-xs font-semibold uppercase text-ink-soft">Personal</span>}
                  </td>
                  <td className="px-4 py-3 font-semibold capitalize">{o.role}</td>
                  <td className="px-4 py-3 text-sm font-medium text-ink-soft">{roles.find((r) => r.value === o.role)?.hint}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      <section aria-labelledby="session-h" className="border-2 border-ink bg-white p-5 sm:p-8">
        <h2 id="session-h" className="text-2xl">
          Sign out
        </h2>
        <p className="mt-1 font-medium text-ink-soft">Signs you out of CAVI on this device.</p>
        <button
          type="button"
          onClick={() => signOut.mutate()}
          disabled={signOut.isPending}
          className="btn-pop mt-4 min-h-12 bg-ink px-6 font-extrabold text-white disabled:opacity-60"
        >
          {signOut.isPending ? "Signing out…" : "Sign out"}
        </button>
        {signOut.isError && <div className="mt-3"><Banner>{signOut.error.message}</Banner></div>}
      </section>
    </div>
  );
}
