"use client";

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Banner } from "@/components/account/account-header";
import { client, raise, type CaptionOptions, type Composition, type Project, type Render } from "@/lib/queries";
import { FlagList } from "./brief-section";
import { length } from "./clips";

const isMontage = (r: Render) => r.composition.version === 3;
const busy = (r: Render) => r.status === "queued" || r.status === "running";

/** All of a project's renders, newest first, in pages. Polls while any are rendering. */
export function useMontages(project: Project) {
  const q = useInfiniteQuery({
    queryKey: ["project", project.id, "renders", "all"],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: { path: { orgID: project.organisation_id, projectID: project.id }, query: { cursor: pageParam, limit: 100 } },
      });
      if (!data) raise(error, response.status, "Could not load montages.");
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: (q) => (q.state.data?.pages.some((p) => p.data.some((r) => isMontage(r) && busy(r))) ? 3000 : false),
  });
  return { ...q, montages: (q.data?.pages.flatMap((p) => p.data) ?? []).filter(isMontage) };
}

/** The stored composition as a new render request: the API fills in captions and voiceover lengths itself. */
function again(r: Render): { name: string; composition: Composition; captions?: CaptionOptions } {
  const c = r.composition;
  const composition: Composition = {
    version: 3,
    source_asset_id: c.source_asset_id,
    start_ms: c.start_ms,
    end_ms: c.end_ms,
    aspect: c.aspect,
    focus: c.focus,
    segments: c.segments,
    ...(c.hook ? { hook: { text: c.hook.text, end_ms: c.hook.end_ms } } : {}),
    ...(c.voiceover ? { voiceover: { voiceover_id: c.voiceover.voiceover_id, start_ms: c.voiceover.start_ms, duck: c.voiceover.duck } } : {}),
  };
  const cap = c.captions;
  return {
    name: r.name,
    composition,
    ...(cap
      ? {
          captions: {
            style: cap.style,
            mode: cap.mode,
            position: cap.position,
            uppercase: cap.uppercase,
            colors: cap.colors,
            speaker_colors: cap.speaker_colors,
          },
        }
      : {}),
  };
}

/** Rendered montages: watch, download, render a failed one again, delete. */
export function MontagesList({ project, canRender }: { project: Project; canRender: boolean }) {
  const qc = useQueryClient();
  const renders = useMontages(project);
  const refresh = () => qc.invalidateQueries({ queryKey: ["project", project.id, "renders"] });
  const remove = useMutation({
    mutationFn: async (r: Render) => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/projects/{projectID}/renders/{renderID}", {
        params: { path: { orgID: project.organisation_id, projectID: project.id, renderID: r.id } },
      });
      if (!response.ok) raise(error, response.status, "Could not delete the montage.");
    },
    onSettled: refresh,
  });
  const retry = useMutation({
    mutationFn: async (r: Render) => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: again(r),
      });
      if (!data) raise(error, response.status, "Could not start the render.");
    },
    onSettled: refresh,
  });

  const list = renders.montages;
  if (renders.isPending) return <div className="h-20 animate-pulse bg-surface-2" />;

  return (
    <div className="space-y-2">
      {renders.isError && <Banner>{renders.error.message}</Banner>}
      {remove.isError && <Banner>{remove.error.message}</Banner>}
      {retry.isError && <Banner>{retry.error.message}</Banner>}
      {renders.isSuccess && list.length === 0 && (
        <p className="text-sm font-medium text-muted">
          {canRender ? "No montages yet. Make one from a plan, or start a new one." : "No montages yet."}
        </p>
      )}
      {list.length > 0 && (
        <ul className="space-y-2">
          {list.map((r) => {
            const c = r.composition;
            const [w, h] = c.aspect.split(":").map(Number);
            const total = c.segments?.reduce((n, s) => n + s.end_ms - s.start_ms, 0) ?? c.end_ms - c.start_ms;
            const parts = [
              c.aspect,
              length(total),
              `${c.segments?.length ?? 1} ${(c.segments?.length ?? 1) === 1 ? "segment" : "segments"}`,
              c.hook && "hook",
              c.voiceover && "voiceover",
              c.captions && "captions",
            ].filter(Boolean);
            return (
              <li key={r.id} className="rounded-xl border border-line bg-surface-2 p-2.5">
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <p className="min-w-0 font-semibold break-words">{r.name}</p>
                  <p className="font-mono text-xs font-bold text-muted">{parts.join(" · ")}</p>
                </div>
                {c.hook && <p className="mt-1 text-sm font-medium break-words">“{c.hook.text}”</p>}
                <FlagList flags={r.flags} />
                {busy(r) && (
                  <p className="mt-1 flex items-center gap-2 text-sm font-semibold" role="status">
                    <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
                    {r.status === "queued" ? "Waiting to render…" : "Rendering…"}
                  </p>
                )}
                {r.status === "failed" && (
                  <div className="mt-1 text-sm font-semibold" role="status">
                    <p className="border border-line bg-white p-2">{r.error ?? "Rendering failed."}</p>
                    {canRender && (
                      <button
                        type="button"
                        disabled={retry.isPending}
                        onClick={() => retry.mutate(r)}
                        className="btn btn-secondary mt-2 min-h-11 px-3 disabled:opacity-60"
                      >
                        Render again
                      </button>
                    )}
                  </div>
                )}
                {r.output && (
                  <div className="mt-2 grid place-items-center bg-stage p-1.5">
                    <video
                      src={r.output.url}
                      controls
                      playsInline
                      preload="metadata"
                      aria-label={`Rendered montage: ${r.name}`}
                      className="max-h-72 max-w-full"
                      style={{ aspectRatio: `${w} / ${h}` }}
                    />
                  </div>
                )}
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  {r.output && (
                    <a href={r.output.url} className="btn btn-primary flex min-h-11 items-center px-3 text-sm">
                      Download MP4
                    </a>
                  )}
                  {r.can_delete && (
                    <button
                      type="button"
                      disabled={remove.isPending && remove.variables?.id === r.id}
                      onClick={() => {
                        if (window.confirm(`Delete the montage “${r.name}”? This can’t be undone.`)) remove.mutate(r);
                      }}
                      className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
                    >
                      Delete
                    </button>
                  )}
                  {r.output && (
                    <span className="text-xs font-semibold text-muted">
                      {r.output.width}×{r.output.height} · {(r.output.size_bytes / 1_000_000).toFixed(1)} MB
                    </span>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {renders.hasNextPage && (
        <button
          type="button"
          onClick={() => renders.fetchNextPage()}
          disabled={renders.isFetchingNextPage}
          className="btn btn-ghost min-h-11 px-3 text-sm disabled:opacity-60"
        >
          {renders.isFetchingNextPage ? "Loading…" : "Show older renders"}
        </button>
      )}
    </div>
  );
}
