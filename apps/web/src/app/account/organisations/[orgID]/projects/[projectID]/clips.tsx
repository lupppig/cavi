"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type Composition, type MediaAsset, type Project, type Render } from "@/lib/queries";

/** Output shapes, matching the API's composition aspects. */
const shapes: { aspect: Composition["aspect"]; w: number; h: number; hint: string }[] = [
  { aspect: "9:16", w: 9, h: 16, hint: "Reels, TikTok, Shorts" },
  { aspect: "1:1", w: 1, h: 1, hint: "Square feed" },
  { aspect: "4:5", w: 4, h: 5, hint: "Portrait feed" },
  { aspect: "16:9", w: 16, h: 9, hint: "YouTube, web" },
];

const MIN_MS = 1000;
const MAX_MS = 10 * 60_000;
const NUDGE_MS = 500;

/** m:ss.t — tenths matter when trimming. */
export function stampTenths(ms: number) {
  const tenths = Math.floor(ms / 100);
  const s = Math.floor(tenths / 10);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const ss = String(s % 60).padStart(2, "0");
  return `${h > 0 ? `${h}:${String(m).padStart(2, "0")}` : m}:${ss}.${tenths % 10}`;
}

function length(ms: number) {
  const s = Math.round(ms / 100) / 10;
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${String(Math.round(s % 60)).padStart(2, "0")}s`;
}

function rendersKey(project: Project, asset: MediaAsset) {
  return ["project", project.id, "renders", asset.id];
}

/** A video file's rendered clips, newest first. Polls while any are in progress. */
export function useRenders(project: Project, asset: MediaAsset) {
  return useQuery({
    queryKey: rendersKey(project, asset),
    queryFn: async (): Promise<Render[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: {
          path: { orgID: project.organisation_id, projectID: project.id },
          query: { source_asset_id: asset.id, limit: 100 },
        },
      });
      if (!data) raise(error, response.status, "Could not load clips.");
      return data.data;
    },
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "queued" || r.status === "running") ? 3000 : false),
  });
}

function useCreateRender(project: Project, asset: MediaAsset) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, composition }: { name: string; composition: Composition }) => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { name, composition },
      });
      if (!data) raise(error, response.status, "Could not start the render.");
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: rendersKey(project, asset) }),
  });
}

/**
 * Sets up one clip from a video: name, in and out points, shape and
 * framing. The framing preview is the source video positioned exactly as
 * the renderer crops it (CSS object-position uses the same formula).
 */
export function ClipComposer({
  asset,
  project,
  url,
  initial,
  onPreview,
  onClose,
}: {
  asset: MediaAsset;
  project: Project;
  url: string;
  initial: { name: string; startMS: number; endMS: number };
  onPreview: (startMS: number, endMS: number) => void;
  onClose: () => void;
}) {
  const id = useId();
  const duration = asset.metadata?.duration_ms ?? 0;
  const srcW = asset.metadata?.width ?? 16;
  const srcH = asset.metadata?.height ?? 9;
  const [name, setName] = useState(initial.name);
  const [start, setStart] = useState(initial.startMS);
  const [end, setEnd] = useState(initial.endMS);
  const [aspect, setAspect] = useState<Composition["aspect"]>("9:16");
  const [focus, setFocus] = useState(50);
  const create = useCreateRender(project, asset);
  const frame = useRef<HTMLVideoElement>(null);

  const shape = shapes.find((s) => s.aspect === aspect)!;
  const crops = srcW * shape.h !== srcH * shape.w;
  const horizontal = srcW * shape.h > srcH * shape.w; // the source is cropped left/right
  const len = end - start;
  const problem =
    len < MIN_MS ? "Clips must be at least a second long." : len > MAX_MS ? "Clips can be at most 10 minutes long." : null;

  // Show the first frame of the clip in the framing preview.
  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    const seek = () => {
      el.currentTime = start / 1000;
    };
    if (el.readyState >= 1) seek();
    else el.addEventListener("loadedmetadata", seek, { once: true });
    return () => el.removeEventListener("loadedmetadata", seek);
  }, [start]);

  const nudge = (which: "start" | "end", delta: number) => {
    if (which === "start") setStart((s) => Math.min(Math.max(0, s + delta), end - MIN_MS));
    else setEnd((e) => Math.max(Math.min(duration, e + delta), start + MIN_MS));
  };

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (problem) return;
    create.mutate(
      {
        name: name.trim() || "Clip",
        composition: {
          version: 1,
          source_asset_id: asset.id,
          start_ms: Math.round(start),
          end_ms: Math.round(end),
          aspect,
          focus: crops ? focus / 100 : 0.5,
        },
      },
      { onSuccess: onClose },
    );
  }

  const edge = (which: "start" | "end", label: string, value: number) => (
    <div className="flex flex-wrap items-center gap-1.5">
      {/* On narrow composers the label sits above its buttons. */}
      <span className="w-full shrink-0 text-xs font-extrabold uppercase text-ink-soft @[13rem]:w-8">{label}</span>
      <button
        type="button"
        onClick={() => nudge(which, -NUDGE_MS)}
        aria-label={`Move ${label.toLowerCase()} half a second earlier`}
        className="btn-pop grid min-h-11 min-w-11 place-items-center bg-white text-sm font-extrabold"
      >
        −½s
      </button>
      <output className="min-w-16 text-center font-mono text-sm font-bold" aria-label={`${label} time`}>
        {stampTenths(value)}
      </output>
      <button
        type="button"
        onClick={() => nudge(which, NUDGE_MS)}
        aria-label={`Move ${label.toLowerCase()} half a second later`}
        className="btn-pop grid min-h-11 min-w-11 place-items-center bg-white text-sm font-extrabold"
      >
        +½s
      </button>
    </div>
  );

  return (
    <form onSubmit={submit} aria-label="Make a clip" className="@container space-y-3 border-2 border-ink bg-white p-2.5 sm:p-3">
      {create.isError && <Banner>{create.error.message}</Banner>}
      <div>
        <label htmlFor={`${id}-name`} className="text-sm font-extrabold">Clip name</label>
        <input
          id={`${id}-name`}
          value={name}
          onChange={(e) => setName(e.target.value)}
          maxLength={120}
          className="mt-1 block min-h-11 w-full border-2 border-ink bg-paper px-3 text-base font-semibold"
        />
      </div>

      <fieldset className="space-y-1.5">
        <legend className="text-sm font-extrabold">
          Trim <span className="font-semibold text-ink-soft">· {length(len)}</span>
        </legend>
        {edge("start", "In", start)}
        {edge("end", "Out", end)}
        {problem && <p className="text-sm font-semibold text-ink">{problem}</p>}
        <button
          type="button"
          onClick={() => onPreview(start, end)}
          aria-label="Preview the trimmed clip in the player"
          className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold"
        >
          ▶ Preview
        </button>
      </fieldset>

      <fieldset>
        <legend className="text-sm font-extrabold">
          Shape <span className="font-semibold text-ink-soft">· {shape.hint}</span>
        </legend>
        <div className="mt-1 grid grid-cols-2 gap-1.5 @[13rem]:grid-cols-4">
          {shapes.map((s) => (
            <label
              key={s.aspect}
              title={s.hint}
              className={`flex min-h-14 min-w-0 cursor-pointer flex-col items-center justify-center gap-1 border-2 border-ink px-1 py-1.5 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                aspect === s.aspect ? "bg-ink text-white" : "bg-white"
              }`}
            >
              <input
                type="radio"
                name={`${id}-shape`}
                checked={aspect === s.aspect}
                onChange={() => setAspect(s.aspect)}
                aria-label={`${s.aspect}, ${s.hint}`}
                className="sr-only"
              />
              <span
                aria-hidden="true"
                className="shrink-0 border-2 border-ink bg-white"
                style={{ width: s.w >= s.h ? 22 : (22 * s.w) / s.h, height: s.h >= s.w ? 22 : (22 * s.h) / s.w }}
              />
              <span className="text-xs font-extrabold">{s.aspect}</span>
            </label>
          ))}
        </div>
      </fieldset>

      <div>
        <p className="text-sm font-extrabold">Framing</p>
        <div className="mt-1 grid place-items-center bg-ink p-2">
          <video
            ref={frame}
            src={url}
            muted
            playsInline
            preload="metadata"
            aria-label="How the clip will be framed"
            className="block bg-ink"
            style={{
              // Width-led sizing keeps the exact shape at any width (at most
              // 16rem tall), so the preview crops exactly as the render will.
              width: `min(100%, calc(16rem * ${shape.w} / ${shape.h}))`,
              aspectRatio: `${shape.w} / ${shape.h}`,
              objectFit: "cover",
              objectPosition: `${focus}% ${focus}%`,
            }}
          />
        </div>
        {crops ? (
          <label className="mt-2 block">
            <span className="text-xs font-semibold text-ink-soft">
              Slide to choose what stays in frame ({horizontal ? "left to right" : "top to bottom"})
            </span>
            <input
              type="range"
              min={0}
              max={100}
              value={focus}
              onChange={(e) => setFocus(Number(e.target.value))}
              className="mt-1 block h-11 w-full accent-[var(--ink)]"
            />
          </label>
        ) : (
          <p className="mt-1 text-xs font-semibold text-ink-soft">The whole frame fits this shape.</p>
        )}
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={create.isPending || !!problem}
          className="btn-pop min-h-11 bg-ink px-3 text-sm font-extrabold text-white disabled:opacity-60"
        >
          {create.isPending ? "Starting…" : "Render clip"}
        </button>
        <button type="button" onClick={onClose} className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold">
          Cancel
        </button>
      </div>
    </form>
  );
}

/** The rendered clips made from one video, with a way to make one from any point. */
export function ClipsPanel({
  asset,
  project,
  url,
  canRender,
  playheadMS,
  onPreview,
}: {
  asset: MediaAsset;
  project: Project;
  url: string;
  canRender: boolean;
  playheadMS: () => number;
  onPreview: (startMS: number, endMS: number) => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const renders = useRenders(project, asset);
  const retry = useCreateRender(project, asset);
  const [composing, setComposing] = useState<null | { startMS: number; endMS: number }>(null);
  const remove = useMutation({
    mutationFn: async (r: Render) => {
      const { error, response } = await client().DELETE(
        "/api/v1/organisations/{orgID}/projects/{projectID}/renders/{renderID}",
        { params: { path: { orgID: project.organisation_id, projectID: project.id, renderID: r.id } } },
      );
      if (!response.ok) raise(error, response.status, "Could not delete the clip.");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: rendersKey(project, asset) }),
  });

  const list = renders.data ?? [];
  if (renders.isPending || (list.length === 0 && !canRender && !renders.isError)) return null;
  const duration = asset.metadata?.duration_ms ?? 0;

  function startHere() {
    const from = Math.min(Math.max(0, Math.round(playheadMS())), Math.max(0, duration - MIN_MS));
    setComposing({ startMS: from, endMS: Math.min(duration, from + 30_000) });
  }

  return (
    <section aria-labelledby={`${id}-h`} className="mt-3 border-2 border-ink bg-white">
      <div className="flex min-h-11 flex-wrap items-center gap-2 px-3 pt-2">
        <h3 id={`${id}-h`} className="font-extrabold">Clips</h3>
        {list.length > 0 && <span className="text-sm font-semibold text-ink-soft">· {list.length}</span>}
        {canRender && !composing && (
          <button type="button" onClick={startHere} className="btn-pop ml-auto min-h-11 bg-white px-3 text-sm font-extrabold">
            + Clip from playhead
          </button>
        )}
      </div>
      {renders.isError && <div className="px-3 pb-3"><Banner>{renders.error.message}</Banner></div>}
      {remove.isError && <div className="px-3 pb-3"><Banner>{remove.error.message}</Banner></div>}
      {retry.isError && <div className="px-3 pb-3"><Banner>{retry.error.message}</Banner></div>}
      {composing && (
        <div className="px-3 pb-3">
          <ClipComposer
            asset={asset}
            project={project}
            url={url}
            initial={{ name: `${asset.filename.replace(/\.[^.]+$/, "")} clip`, ...composing }}
            onPreview={onPreview}
            onClose={() => setComposing(null)}
          />
        </div>
      )}
      {list.length === 0 && !composing && (
        <p className="px-3 pb-3 text-sm font-medium text-ink-soft">
          Render a highlight, or pause where you want a clip to start and use “Clip from playhead”.
        </p>
      )}
      {list.length > 0 && (
        <ul className="space-y-2 px-3 pb-3">
          {list.map((r) => {
            const c = r.composition;
            const busy = r.status === "queued" || r.status === "running";
            const [w, h] = c.aspect.split(":").map(Number);
            return (
              <li key={r.id} className="border-2 border-ink/15 bg-paper p-2.5">
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <p className="min-w-0 font-extrabold break-words">{r.name}</p>
                  <p className="font-mono text-xs font-bold text-ink-soft">
                    {c.aspect} · {length(c.end_ms - c.start_ms)} · from {stampTenths(c.start_ms)}
                  </p>
                </div>
                {busy && (
                  <p className="mt-1 flex items-center gap-2 text-sm font-semibold" role="status">
                    <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-ink" />
                    {r.status === "queued" ? "Waiting to render…" : "Rendering…"}
                  </p>
                )}
                {r.status === "failed" && (
                  <div className="mt-1 text-sm font-semibold" role="status">
                    <p className="border-2 border-ink bg-paper-deep p-2">{r.error ?? "Rendering failed."}</p>
                    {canRender && r.source_asset_id && (
                      <button
                        type="button"
                        disabled={retry.isPending}
                        onClick={() => retry.mutate({ name: r.name, composition: r.composition })}
                        className="btn-pop mt-2 min-h-11 bg-white px-3 font-extrabold disabled:opacity-60"
                      >
                        Render again
                      </button>
                    )}
                  </div>
                )}
                {r.output && (
                  <div className="mt-2 grid place-items-center bg-ink p-1.5">
                    <video
                      src={r.output.url}
                      controls
                      playsInline
                      preload="metadata"
                      aria-label={`Rendered clip: ${r.name}`}
                      className="max-h-72 max-w-full"
                      style={{ aspectRatio: `${w} / ${h}` }}
                    />
                  </div>
                )}
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  {r.output && (
                    <a href={r.output.url} className="btn-pop flex min-h-11 items-center bg-white px-3 text-sm font-extrabold">
                      Download
                    </a>
                  )}
                  {r.can_delete && (
                    <button
                      type="button"
                      disabled={remove.isPending && remove.variables?.id === r.id}
                      onClick={() => {
                        if (window.confirm(`Delete the clip “${r.name}”? This can’t be undone.`)) remove.mutate(r);
                      }}
                      className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold disabled:opacity-60"
                    >
                      Delete
                    </button>
                  )}
                  {r.output && (
                    <span className="text-xs font-semibold text-ink-soft">
                      {r.output.width}×{r.output.height} · {(r.output.size_bytes / 1_000_000).toFixed(1)} MB
                    </span>
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
