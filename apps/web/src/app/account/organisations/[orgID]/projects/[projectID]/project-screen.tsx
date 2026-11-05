"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useId, useState, type FormEvent } from "react";
import { AccountHeader, Banner } from "@/components/account/account-header";
import { GoogleSignIn } from "@/components/account/google-sign-in";
import { projectColour } from "@/lib/projects";
import { MediaSection } from "./media-section";
import {
  can,
  client,
  displayName,
  initials,
  NotFound,
  NotSignedIn,
  raise,
  useMe,
  useMembers,
  useOrganisation,
  type Organisation,
  type Project,
  type ProjectAccess,
} from "@/lib/queries";

const field =
  "w-full min-w-0 rounded-2xl border-[3px] border-ink bg-paper px-4 text-base font-medium focus:ring-4 focus:ring-cobalt/40 focus:outline-none disabled:opacity-60";

function useProject(orgID: string, projectID: string) {
  return useQuery({
    queryKey: ["project", projectID],
    queryFn: async (): Promise<Project> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}", {
        params: { path: { orgID, projectID } },
      });
      if (!data) raise(error, response.status, "Could not load this project.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && !(e instanceof NotSignedIn) && n < 2,
  });
}

export function ProjectScreen() {
  const { orgID, projectID } = useParams<{ orgID: string; projectID: string }>();
  const project = useProject(orgID, projectID);
  const org = useOrganisation(orgID);
  useMe(); // keeps the header's sign-out button in sync

  return (
    <main className="min-h-dvh px-4 py-6 sm:px-8">
      <AccountHeader />
      <div className="mx-auto mt-8 max-w-4xl space-y-8">
        <Link
          href={`/account/organisations/${orgID}`}
          className="inline-flex min-h-11 max-w-full items-center font-extrabold underline decoration-2 underline-offset-4"
        >
          <span className="truncate">← {org.data?.name ?? "Back to organisation"}</span>
        </Link>

        {project.isPending && (
          <div aria-busy="true" className="space-y-6">
            <span className="sr-only">Loading…</span>
            <div className="h-48 animate-pulse rounded-[28px] bg-paper-deep" />
            <div className="h-32 animate-pulse rounded-[28px] bg-paper-deep" />
          </div>
        )}
        {project.isError && project.error instanceof NotSignedIn && (
          <div className="sticker-lg rounded-[28px] bg-sun p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Sign in to see this project</h1>
            <GoogleSignIn className="mt-6 w-full sm:w-auto" />
          </div>
        )}
        {project.isError && project.error instanceof NotFound && (
          <div className="sticker-lg rounded-[28px] bg-white p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Project not found</h1>
            <p className="mt-2 font-medium">It doesn’t exist, or it hasn’t been shared with you.</p>
          </div>
        )}
        {project.isError && !(project.error instanceof NotFound) && !(project.error instanceof NotSignedIn) && (
          <div className="sticker-lg rounded-[28px] bg-white p-6 sm:p-8">
            <h1 className="text-3xl font-extrabold">Something went wrong</h1>
            <p className="mt-2 font-medium">{project.error.message}</p>
            <button type="button" onClick={() => project.refetch()} className="btn-pop mt-6 min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white">
              Try again
            </button>
          </div>
        )}
        {project.data && <ProjectView project={project.data} org={org.data} />}
      </div>
    </main>
  );
}

function ProjectView({ project, org }: { project: Project; org?: Organisation }) {
  return (
    <>
      <Details project={project} />
      <MediaSection project={project} canUpload={!!org && can(org, "media.upload")} />
      <p className="rounded-2xl border-[3px] border-dashed border-ink bg-white/60 p-5 font-semibold">
        Editing and renders aren’t available yet. They’re next, and they’ll work from the media in this project.
      </p>
      {org && !org.personal && can(project, "project.access.manage") && <ClientAccess project={project} />}
      {can(project, "project.delete") && <DeleteProject project={project} />}
    </>
  );
}

function Details({ project }: { project: Project }) {
  const id = useId();
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState(project.name);
  const [description, setDescription] = useState(project.description);

  const save = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().PATCH("/api/v1/organisations/{orgID}/projects/{projectID}", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { name: name.trim(), description: description.trim() },
      });
      if (!data) raise(error, response.status, "Could not save the project.");
      return data;
    },
    onSuccess: async (updated) => {
      qc.setQueryData(["project", project.id], updated);
      await qc.invalidateQueries({ queryKey: ["organisation", project.organisation_id, "projects"] });
      setEditing(false);
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim()) save.mutate();
  }

  function cancel() {
    setEditing(false);
    setName(project.name);
    setDescription(project.description);
    save.reset();
  }

  return (
    <section className={`sticker-lg rounded-[28px] p-2 ${projectColour(project.id)}`}>
      <div className="rounded-[20px] border-[3px] border-ink bg-white p-5 sm:p-7">
        {editing ? (
          <form onSubmit={onSubmit}>
            <label htmlFor={`${id}-name`} className="block font-extrabold">Name</label>
            <input
              id={`${id}-name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={120}
              required
              autoFocus
              disabled={save.isPending}
              className={`${field} mt-2 min-h-12`}
            />
            <label htmlFor={`${id}-desc`} className="mt-4 block font-extrabold">Description</label>
            <textarea
              id={`${id}-desc`}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              maxLength={2000}
              rows={4}
              disabled={save.isPending}
              className={`${field} mt-2 py-3`}
            />
            {save.isError && <div className="mt-4"><Banner>{save.error.message}</Banner></div>}
            <div className="mt-5 flex flex-col gap-3 sm:flex-row">
              <button type="submit" disabled={save.isPending || !name.trim()} className="btn-pop min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white disabled:opacity-60">
                {save.isPending ? "Saving…" : "Save"}
              </button>
              <button type="button" onClick={cancel} disabled={save.isPending} className="btn-pop min-h-12 rounded-2xl bg-white px-6 font-extrabold">
                Cancel
              </button>
            </div>
          </form>
        ) : (
          <>
            <div className="flex flex-wrap items-start gap-3">
              <h1 className="mr-auto min-w-0 text-4xl leading-tight font-extrabold tracking-tight break-words sm:text-5xl">
                {project.name}
              </h1>
              {can(project, "project.update") && (
                <button
                  type="button"
                  onClick={() => {
                    setName(project.name);
                    setDescription(project.description);
                    setEditing(true);
                  }}
                  className="btn-pop min-h-11 rounded-full bg-white px-4 text-sm font-extrabold"
                >
                  Edit
                </button>
              )}
            </div>
            {project.description ? (
              <p className="mt-3 max-w-2xl text-lg font-medium break-words whitespace-pre-line">{project.description}</p>
            ) : (
              <p className="mt-3 font-medium text-ink-soft italic">No description yet.</p>
            )}
            <div className="mt-5 flex flex-wrap gap-2 text-sm font-extrabold">
              <span className="rounded-full border-2 border-ink bg-sky px-3 py-1">{project.workspace.name}</span>
              {project.created_by && (
                <span className="rounded-full border-2 border-ink bg-paper px-3 py-1">Started by {project.created_by.name}</span>
              )}
              <span className="rounded-full border-2 border-ink bg-paper px-3 py-1">
                {new Date(project.created_at).toLocaleDateString()}
              </span>
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function ClientAccess({ project }: { project: Project }) {
  const id = useId();
  const qc = useQueryClient();
  const orgID = project.organisation_id;
  const [selected, setSelected] = useState("");
  const accessKey = ["project", project.id, "access"];

  const access = useQuery({
    queryKey: accessKey,
    queryFn: async (): Promise<ProjectAccess[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/access", {
        params: { path: { orgID, projectID: project.id } },
      });
      if (!data) raise(error, response.status, "Could not load who this project is shared with.");
      return data.data;
    },
  });
  const members = useMembers(orgID);

  const grant = useMutation({
    mutationFn: async (userID: string) => {
      const { error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/access", {
        params: { path: { orgID, projectID: project.id } },
        body: { user_id: userID },
      });
      if (!response.ok) raise(error, response.status, "Could not share the project.");
    },
    onSuccess: () => setSelected(""),
    onSettled: () => qc.invalidateQueries({ queryKey: accessKey }),
  });

  const revoke = useMutation({
    mutationFn: async (userID: string) => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/projects/{projectID}/access/{userID}", {
        params: { path: { orgID, projectID: project.id, userID } },
      });
      if (!response.ok) raise(error, response.status, "Could not stop sharing.");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: accessKey }),
  });

  const sharedIDs = new Set(access.data?.map((a) => a.user_id));
  const clients = (members.data ?? []).filter((m) => m.role === "client" && !sharedIDs.has(m.user_id));
  const error = grant.error ?? revoke.error;

  return (
    <section aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-white p-5 sm:p-7">
      <h2 id={`${id}-h`} className="text-2xl font-extrabold tracking-tight">Client access</h2>
      <p className="mt-1 font-medium text-ink-soft">
        Your team sees every project. Clients only see the projects you share with them here.
      </p>
      {error && <div className="mt-4"><Banner>{error.message}</Banner></div>}

      {(access.isPending || members.isPending) && <div className="mt-4 h-14 animate-pulse rounded-2xl bg-paper-deep" />}
      {access.isError && <div className="mt-4"><Banner>{access.error.message}</Banner></div>}
      {members.isError && <div className="mt-4"><Banner>{members.error.message}</Banner></div>}

      {access.data && members.data && (
        <>
          {clients.length > 0 ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                if (selected) grant.mutate(selected);
              }}
              className="mt-4 flex flex-col gap-3 sm:flex-row"
            >
              <label htmlFor={`${id}-client`} className="sr-only">Client to share with</label>
              <select
                id={`${id}-client`}
                value={selected}
                onChange={(e) => setSelected(e.target.value)}
                disabled={grant.isPending}
                className={`${field} min-h-12 font-bold sm:flex-1`}
              >
                <option value="">Choose a client…</option>
                {clients.map((m) => (
                  <option key={m.user_id} value={m.user_id}>
                    {displayName(m)} ({m.email})
                  </option>
                ))}
              </select>
              <button type="submit" disabled={!selected || grant.isPending} className="btn-pop min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white disabled:opacity-60">
                {grant.isPending ? "Sharing…" : "Share"}
              </button>
            </form>
          ) : (
            access.data.length === 0 && (
              <p className="mt-4 rounded-2xl border-2 border-ink bg-paper p-4 font-semibold">
                There are no clients in this team yet. Invite someone with the Client role from the{" "}
                <Link href={`/account/organisations/${orgID}`} className="font-extrabold underline decoration-2 underline-offset-4">
                  team page
                </Link>
                , then share this project with them.
              </p>
            )
          )}

          {access.data.length > 0 && (
            <ul className="mt-4 divide-y-2 divide-ink/10">
              {access.data.map((a) => (
                <li key={a.user_id} className="flex flex-col gap-3 py-3 sm:flex-row sm:items-center">
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    {a.avatar_url ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={a.avatar_url} alt="" referrerPolicy="no-referrer" className="size-11 shrink-0 rounded-full border-[3px] border-ink object-cover" />
                    ) : (
                      <span aria-hidden="true" className="grid size-11 shrink-0 place-items-center rounded-full border-[3px] border-ink bg-pink text-sm font-extrabold">
                        {initials(a)}
                      </span>
                    )}
                    <div className="min-w-0">
                      <p className="truncate font-extrabold">{displayName(a)}</p>
                      <p className="truncate text-sm font-medium text-ink-soft">{a.email}</p>
                    </div>
                  </div>
                  <button
                    type="button"
                    disabled={revoke.isPending && revoke.variables === a.user_id}
                    onClick={() => revoke.mutate(a.user_id)}
                    className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold disabled:opacity-60"
                  >
                    Stop sharing
                  </button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </section>
  );
}

function DeleteProject({ project }: { project: Project }) {
  const router = useRouter();
  const qc = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/projects/{projectID}", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
      });
      if (!response.ok) raise(error, response.status, "Could not delete the project.");
    },
    onSuccess: async () => {
      qc.removeQueries({ queryKey: ["project", project.id] });
      await qc.invalidateQueries({ queryKey: ["organisation", project.organisation_id, "projects"] });
      router.push(`/account/organisations/${project.organisation_id}`);
    },
  });

  return (
    <section className="rounded-[28px] border-[3px] border-ink bg-white/60 p-5 sm:p-7">
      <h2 className="text-xl font-extrabold">Delete project</h2>
      <p className="mt-1 font-medium text-ink-soft">This removes the project for everyone. It can’t be undone.</p>
      {remove.isError && <div className="mt-4"><Banner>{remove.error.message}</Banner></div>}
      <button
        type="button"
        disabled={remove.isPending}
        onClick={() => {
          if (window.confirm(`Delete “${project.name}”? This can’t be undone.`)) remove.mutate();
        }}
        className="btn-pop mt-4 min-h-12 rounded-2xl bg-tomato px-6 font-extrabold text-white disabled:opacity-60"
      >
        {remove.isPending ? "Deleting…" : "Delete project"}
      </button>
    </section>
  );
}
