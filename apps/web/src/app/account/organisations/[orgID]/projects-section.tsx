"use client";

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useId, useState, type FormEvent } from "react";
import { Banner } from "@/components/account/account-header";
import { can, client, raise, type Organisation, type Project } from "@/lib/queries";
import { projectColour, projectHref } from "@/lib/projects";

const field =
  "w-full min-w-0 rounded-2xl border-[3px] border-ink bg-paper px-4 text-base font-medium focus:ring-4 focus:ring-cobalt/40 focus:outline-none disabled:opacity-60";

export function Projects({ org }: { org: Organisation }) {
  const id = useId();
  const canCreate = can(org, "project.create");
  const isClient = org.role === "client";
  const [creating, setCreating] = useState(false);

  const projects = useInfiniteQuery({
    queryKey: ["organisation", org.id, "projects"],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects", {
        params: { path: { orgID: org.id }, query: { cursor: pageParam, limit: 30 } },
      });
      if (!data) raise(error, response.status, "Could not load projects.");
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });

  const items = projects.data?.pages.flatMap((p) => p.data) ?? [];

  return (
    <section aria-labelledby={`${id}-h`} className="space-y-5">
      <div className="flex flex-wrap items-center gap-3">
        <h2 id={`${id}-h`} className="mr-auto text-3xl font-extrabold tracking-tight">
          {isClient ? "Shared with you" : "Projects"}
        </h2>
        {canCreate && !creating && (
          <button
            type="button"
            onClick={() => setCreating(true)}
            className="btn-pop min-h-12 rounded-2xl bg-tomato px-5 font-extrabold text-white"
          >
            + New project
          </button>
        )}
      </div>

      {creating && <CreateProject org={org} onCancel={() => setCreating(false)} />}

      {projects.isPending && (
        <div aria-busy="true" className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <span className="sr-only">Loading projects…</span>
          <div className="h-40 animate-pulse rounded-[28px] bg-paper-deep" />
          <div className="h-40 animate-pulse rounded-[28px] bg-paper-deep" />
        </div>
      )}
      {projects.isError && (
        <div className="space-y-3">
          <Banner>{projects.error.message}</Banner>
          <button type="button" onClick={() => projects.refetch()} className="btn-pop min-h-11 rounded-xl bg-white px-4 font-extrabold">
            Try again
          </button>
        </div>
      )}
      {projects.isSuccess && items.length === 0 && !creating && (
        <EmptyProjects isClient={isClient} canCreate={canCreate} onCreate={() => setCreating(true)} />
      )}
      {items.length > 0 && (
        <ul className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {items.map((p) => (
            <li key={p.id}>
              <ProjectCard project={p} />
            </li>
          ))}
        </ul>
      )}
      {projects.hasNextPage && (
        <button
          type="button"
          onClick={() => projects.fetchNextPage()}
          disabled={projects.isFetchingNextPage}
          className="btn-pop min-h-12 w-full rounded-2xl bg-white px-6 font-extrabold disabled:opacity-60 sm:w-auto"
        >
          {projects.isFetchingNextPage ? "Loading…" : "Show more projects"}
        </button>
      )}
      {projects.isFetchNextPageError && <Banner>Could not load more projects. Try again.</Banner>}
    </section>
  );
}

function ProjectCard({ project }: { project: Project }) {
  return (
    <Link
      href={projectHref(project)}
      className={`tilt-hover sticker-lg block h-full rounded-[28px] p-2 focus-visible:outline-offset-4 ${projectColour(project.id)}`}
    >
      <span className="flex h-full flex-col rounded-[20px] border-[3px] border-ink bg-white p-5">
        <span className="text-xl font-extrabold tracking-tight break-words">{project.name}</span>
        {project.description ? (
          <span className="mt-2 line-clamp-2 font-medium break-words text-ink-soft">{project.description}</span>
        ) : (
          <span className="mt-2 font-medium text-ink-soft italic">No description</span>
        )}
        <span className="mt-auto flex flex-wrap items-center gap-x-2 gap-y-1 pt-4 text-sm font-semibold text-ink-soft">
          <span>{project.workspace.name}</span>
          {project.created_by && <span>· by {project.created_by.name}</span>}
          <span>· {new Date(project.created_at).toLocaleDateString()}</span>
        </span>
      </span>
    </Link>
  );
}

function EmptyProjects({ isClient, canCreate, onCreate }: { isClient: boolean; canCreate: boolean; onCreate: () => void }) {
  if (isClient) {
    return (
      <p className="rounded-2xl border-[3px] border-dashed border-ink bg-white/60 p-5 font-semibold">
        Nothing has been shared with you yet. When the team shares a project with you, it’ll show up here.
      </p>
    );
  }
  return (
    <div className="flex flex-col items-start gap-4 rounded-[28px] border-[3px] border-dashed border-ink bg-white/60 p-6">
      <p className="text-lg font-semibold">
        No projects yet.{" "}
        {canCreate
          ? "Start one for your next shoot, campaign, release or edit."
          : "When someone starts one, it’ll show up here."}
      </p>
      {canCreate && (
        <button type="button" onClick={onCreate} className="btn-pop min-h-12 rounded-2xl bg-cobalt px-5 font-extrabold text-white">
          Start your first project
        </button>
      )}
    </div>
  );
}

function CreateProject({ org, onCancel }: { org: Organisation; onCancel: () => void }) {
  const id = useId();
  const router = useRouter();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");

  const create = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects", {
        params: { path: { orgID: org.id } },
        body: { name: name.trim(), description: description.trim() },
      });
      if (!data) raise(error, response.status, "Could not create the project.");
      return data;
    },
    onSuccess: async (p) => {
      qc.setQueryData(["project", p.id], p);
      await qc.invalidateQueries({ queryKey: ["organisation", org.id, "projects"] });
      router.push(projectHref(p));
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (name.trim()) create.mutate();
  }

  return (
    <form onSubmit={onSubmit} aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-sun p-5 sm:p-7">
      <h3 id={`${id}-h`} className="text-2xl font-extrabold tracking-tight">New project</h3>
      <label htmlFor={`${id}-name`} className="mt-4 block font-extrabold">Name</label>
      <input
        id={`${id}-name`}
        value={name}
        onChange={(e) => setName(e.target.value)}
        maxLength={120}
        required
        autoFocus
        placeholder="e.g. Spring tour teasers"
        disabled={create.isPending}
        className={`${field} mt-2 min-h-12`}
      />
      <label htmlFor={`${id}-desc`} className="mt-4 block font-extrabold">
        Description <span className="font-semibold text-ink-soft">(optional)</span>
      </label>
      <textarea
        id={`${id}-desc`}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        maxLength={2000}
        rows={3}
        placeholder="What’s it for? Who’s it for? Anything the team should know."
        disabled={create.isPending}
        className={`${field} mt-2 py-3`}
      />
      {create.isError && <div className="mt-4"><Banner>{create.error.message}</Banner></div>}
      <div className="mt-5 flex flex-col gap-3 sm:flex-row">
        <button
          type="submit"
          disabled={create.isPending || !name.trim()}
          className="btn-pop min-h-12 rounded-2xl bg-cobalt px-6 font-extrabold text-white disabled:opacity-60"
        >
          {create.isPending ? "Creating…" : "Create project"}
        </button>
        <button type="button" onClick={onCancel} disabled={create.isPending} className="btn-pop min-h-12 rounded-2xl bg-white px-6 font-extrabold">
          Cancel
        </button>
      </div>
    </form>
  );
}
