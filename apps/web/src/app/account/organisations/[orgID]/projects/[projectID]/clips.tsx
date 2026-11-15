"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { BriefCheck, FlagList } from "./brief-section";
import { client, raise, type CaptionOptions, type Composition, type MediaAsset, type Project, type Render } from "@/lib/queries";
import { CaptionOptionsPicker, CaptionPreview, captionLayout, defaultCaptionOptions, firstCaption } from "./caption-options";
import { FramingChoice, FramingPreview, useFramingPath } from "./framing";
import { useRequestTranscript, useTranscript } from "./transcript-panel";

/** Output shapes, matching the API's composition aspects and render sizes. */
export const shapes: { aspect: Composition["aspect"]; w: number; h: number; outW: number; outH: number; hint: string }[] = [
  { aspect: "9:16", w: 9, h: 16, outW: 1080, outH: 1920, hint: "Reels, TikTok, Shorts" },
  { aspect: "1:1", w: 1, h: 1, outW: 1080, outH: 1080, hint: "Square feed" },
  { aspect: "4:5", w: 4, h: 5, outW: 1080, outH: 1350, hint: "Portrait feed" },
  { aspect: "16:9", w: 16, h: 9, outW: 1920, outH: 1080, hint: "YouTube, web" },
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

export function length(ms: number) {
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
    mutationFn: async ({
      name,
      composition,
      captions,
    }: {
      name: string;
      composition: Composition;
      captions: CaptionOptions | null;
    }) => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { name, composition, ...(captions ? { captions } : {}) },
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
  canTranscribe,
  onPreview,
  onClose,
}: {
  asset: MediaAsset;
  project: Project;
  url: string;
  initial: { name: string; startMS: number; endMS: number };
  canTranscribe: boolean;
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
  const [auto, setAuto] = useState(false);
  const [captions, setCaptions] = useState<CaptionOptions | null>(null);
  const create = useCreateRender(project, asset);
  const transcript = useTranscript(project, asset);
  const transcribe = useRequestTranscript(project, asset);
  const t = transcript.data;
  const ready = t?.status === "ready";
  const inClip = ready ? t.segments.filter((sg) => sg.end_ms > start && sg.start_ms < end) : [];
  const spoken = inClip[0];
  const speakersInClip = ready ? t.speakers.filter((sp) => inClip.some((sg) => sg.speaker === sp.label)) : [];
  const frame = useRef<HTMLVideoElement>(null);

  const shape = shapes.find((s) => s.aspect === aspect)!;
  const crops = srcW * shape.h !== srcH * shape.w;
  const horizontal = srcW * shape.h > srcH * shape.w; // the source is cropped left/right
  const len = end - start;
  const path = useFramingPath(project, asset, aspect, start, end, focus / 100, auto && crops);
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
          version: captions || (auto && crops) ? 2 : 1,
          source_asset_id: asset.id,
          start_ms: Math.round(start),
          end_ms: Math.round(end),
          aspect,
          focus: crops ? focus / 100 : 0.5,
          ...(auto && crops ? { framing: "auto" as const } : {}),
        },
        captions: captions && {
          ...captions,
          // Only speakers heard in this clip.
          speaker_colors: captions.speaker_colors
            ? Object.fromEntries(Object.entries(captions.speaker_colors).filter(([label]) => speakersInClip.some((sp) => sp.label === label)))
            : undefined,
        },
      },
      { onSuccess: onClose },
    );
  }

  const edge = (which: "start" | "end", label: string, value: number) => (
    <div className="flex flex-wrap items-center gap-1.5">
      {/* On narrow composers the label sits above its buttons. */}
      <span className="w-full shrink-0 text-xs font-semibold uppercase text-muted @[13rem]:w-8">{label}</span>
      <button
        type="button"
        onClick={() => nudge(which, -NUDGE_MS)}
        aria-label={`Move ${label.toLowerCase()} half a second earlier`}
        className="btn btn-secondary grid min-h-11 min-w-11 place-items-center text-sm"
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
        className="btn btn-secondary grid min-h-11 min-w-11 place-items-center text-sm"
      >
        +½s
      </button>
    </div>
  );

  return (
    <form onSubmit={submit} aria-label="Make a clip" className="@container space-y-3 card p-2.5 sm:p-3">
      {create.isError && <Banner>{create.error.message}</Banner>}
      <BriefCheck project={project} asset={asset} startMS={start} endMS={end} />
      {/* Two columns when there's room: what you're cutting on the left, how it looks on the right. */}
      <div className="grid gap-3 @[44rem]:grid-cols-2 @[44rem]:gap-6">
        <div className="min-w-0 space-y-3">
          <div>
            <label htmlFor={`${id}-name`} className="text-sm font-semibold">Clip name</label>
            <input
              id={`${id}-name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={120}
              className="mt-1 block min-h-11 w-full border border-line bg-surface px-3 text-base font-semibold"
            />
          </div>

          <fieldset className="space-y-1.5">
            <legend className="text-sm font-semibold">
              Trim <span className="font-semibold text-muted">· {length(len)}</span>
            </legend>
            {edge("start", "In", start)}
            {edge("end", "Out", end)}
            {problem && <p className="text-sm font-semibold text-ink">{problem}</p>}
            <button
              type="button"
              onClick={() => onPreview(start, end)}
              aria-label="Preview the trimmed clip in the player"
              className="btn btn-secondary min-h-11 px-3 text-sm"
            >
              ▶ Preview
            </button>
          </fieldset>

          <div>
            <p className="text-sm font-semibold">Framing</p>
            <div className="mt-1 grid place-items-center bg-stage p-2">
              <div
                className="relative [container-type:inline-size]"
                style={{
                  // Width-led sizing keeps the exact shape at any width (at most
                  // 16rem tall), so the preview crops exactly as the render will.
                  width: `min(100%, calc(16rem * ${shape.w} / ${shape.h}))`,
                  aspectRatio: `${shape.w} / ${shape.h}`,
                }}
              >
                {auto && crops && path.data ? (
                  <FramingPreview
                    url={url}
                    startMS={start}
                    endMS={end}
                    keys={path.data}
                    horizontal={horizontal}
                    label="How the clip will be framed, following the people in shot"
                  />
                ) : (
                  <video
                    ref={frame}
                    src={url}
                    muted
                    playsInline
                    preload="metadata"
                    aria-label="How the clip will be framed"
                    className="block h-full w-full bg-stage"
                    style={{ objectFit: "cover", objectPosition: `${focus}% ${focus}%` }}
                  />
                )}
                {captions && spoken && (
                  <CaptionPreview
                    options={captions}
                    speaker={spoken.speaker}
                    lines={firstCaption(
                      spoken,
                      start,
                      captionLayout(shape.outW, shape.outH, captions.mode, captions.position).maxChars,
                      captions.mode,
                    )}
                    outW={shape.outW}
                    outH={shape.outH}
                  />
                )}
              </div>
            </div>
            {auto && crops && path.isError && <Banner>{path.error.message}</Banner>}
            {crops && (
              <div className="mt-2">
                <FramingChoice project={project} assets={[asset]} auto={auto} onAuto={setAuto} canAnalyse={canTranscribe} />
              </div>
            )}
            {crops ? (
              <label className="mt-2 block">
                <span className="text-xs font-semibold text-muted">
                  {auto
                    ? `Where the frame sits when nobody’s in shot (${horizontal ? "left to right" : "top to bottom"})`
                    : `Slide to choose what stays in frame (${horizontal ? "left to right" : "top to bottom"})`}
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
              <p className="mt-1 text-xs font-semibold text-muted">The whole frame fits this shape.</p>
            )}
          </div>
        </div>
        <div className="min-w-0 space-y-3">
          <fieldset>
            <legend className="text-sm font-semibold">
              Shape <span className="font-semibold text-muted">· {shape.hint}</span>
            </legend>
            <div className="mt-1 grid grid-cols-2 gap-1.5 @[13rem]:grid-cols-4">
              {shapes.map((s) => (
                <label
                  key={s.aspect}
                  title={s.hint}
                  className={`flex min-h-14 min-w-0 cursor-pointer flex-col items-center justify-center gap-1 border border-line px-1 py-1.5 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                    aspect === s.aspect ? "bg-accent-soft text-accent-text" : "bg-white"
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
                    className="shrink-0 card"
                    style={{ width: s.w >= s.h ? 22 : (22 * s.w) / s.h, height: s.h >= s.w ? 22 : (22 * s.h) / s.w }}
                  />
                  <span className="text-xs font-semibold">{s.aspect}</span>
                </label>
              ))}
            </div>
          </fieldset>

          <fieldset>
            <legend className="text-sm font-semibold">Captions</legend>
            {ready && t.segments.length > 0 ? (
              <>
                <label className="mt-1 flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
                  <input
                    type="checkbox"
                    checked={!!captions}
                    onChange={(e) => setCaptions(e.target.checked ? defaultCaptionOptions : null)}
                    className="size-5 accent-[var(--ink)]"
                  />
                  Burn in captions
                </label>
                {captions && (
                  <>
                    <CaptionOptionsPicker value={captions} onChange={setCaptions} speakers={speakersInClip} />
                    <p className="mt-1 text-xs font-semibold text-muted">
                      {spoken
                        ? "Captions come from the transcript, timed word by word. Check it for mistakes first."
                        : "There’s no speech in this part of the clip, so there’s nothing to caption."}
                    </p>
                  </>
                )}
              </>
            ) : (
              <div className="mt-1 border border-dashed border-line-strong p-2.5 text-sm font-semibold" role="status">
                {transcript.isPending ? (
                  <p>Checking for a transcript…</p>
                ) : t && (t.status === "queued" || t.status === "running") ? (
                  <p>Transcribing… Captions will be available when the transcript is ready.</p>
                ) : ready ? (
                  <p>No speech was found in this file, so there’s nothing to caption.</p>
                ) : (
                  <>
                    <p>
                      {t?.status === "failed"
                        ? `Captions need a transcript, and the last one failed: ${t.error ?? "transcription failed"}.`
                        : "Captions need a transcript of this file."}
                    </p>
                    {canTranscribe ? (
                      <button
                        type="button"
                        onClick={transcribe.start}
                        disabled={transcribe.request.isPending}
                        className="btn btn-primary mt-2 min-h-11 px-3 text-sm disabled:opacity-60"
                      >
                        {transcribe.request.isPending ? "Starting…" : t?.status === "failed" ? "Transcribe again" : "Transcribe now"}
                      </button>
                    ) : (
                      <p className="mt-1 text-muted">Ask an editor to transcribe it.</p>
                    )}
                    {transcribe.request.isError && <p className="mt-1">{transcribe.request.error.message}</p>}
                  </>
                )}
              </div>
            )}
          </fieldset>

          <div className="flex flex-wrap gap-2">
            <button
              type="submit"
              disabled={create.isPending || !!problem || (!!captions && !spoken)}
              className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60"
            >
              {create.isPending ? "Starting…" : "Render clip"}
            </button>
            <button type="button" onClick={onClose} className="btn btn-secondary min-h-11 px-3 text-sm">
              Cancel
            </button>
          </div>
        </div>
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
  canTranscribe,
  playheadMS,
  onPreview,
}: {
  asset: MediaAsset;
  project: Project;
  url: string;
  canRender: boolean;
  canTranscribe: boolean;
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
    <section aria-labelledby={`${id}-h`} className="mt-3 card">
      <div className="flex min-h-11 flex-wrap items-center gap-2 px-3 pt-2">
        <h3 id={`${id}-h`} className="font-semibold">Clips</h3>
        {list.length > 0 && <span className="text-sm font-semibold text-muted">· {list.length}</span>}
        {canRender && !composing && (
          <button type="button" onClick={startHere} className="btn btn-secondary ml-auto min-h-11 px-3 text-sm">
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
            canTranscribe={canTranscribe}
            onPreview={onPreview}
            onClose={() => setComposing(null)}
          />
        </div>
      )}
      {list.length === 0 && !composing && (
        <p className="px-3 pb-3 text-sm font-medium text-muted">
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
              <li key={r.id} className="rounded-xl border border-line bg-surface-2 p-2.5">
                <div className="flex flex-wrap items-baseline gap-x-2">
                  <p className="min-w-0 font-semibold break-words">{r.name}</p>
                  <p className="font-mono text-xs font-bold text-muted">
                    {c.aspect} · {length(c.end_ms - c.start_ms)} · from {stampTenths(c.start_ms)}
                    {c.captions ? ` · ${c.captions.mode === "words" ? "word-by-word" : "line"} captions` : ""}
                  </p>
                </div>
                <FlagList flags={r.flags} />
                {busy && (
                  <p className="mt-1 flex items-center gap-2 text-sm font-semibold" role="status">
                    <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
                    {r.status === "queued" ? "Waiting to render…" : "Rendering…"}
                  </p>
                )}
                {r.status === "failed" && (
                  <div className="mt-1 text-sm font-semibold" role="status">
                    <p className="border border-line bg-surface-2 p-2">{r.error ?? "Rendering failed."}</p>
                    {canRender && r.source_asset_id && (
                      <button
                        type="button"
                        disabled={retry.isPending}
                        onClick={() => {
                          const { captions: cap, ...composition } = r.composition;
                          const options: CaptionOptions | null = cap
                            ? {
                                style: cap.style,
                                mode: cap.mode,
                                position: cap.position,
                                uppercase: cap.uppercase,
                                colors: cap.colors,
                                speaker_colors: cap.speaker_colors,
                              }
                            : null;
                          retry.mutate({ name: r.name, composition, captions: options });
                        }}
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
                      aria-label={`Rendered clip: ${r.name}`}
                      className="max-h-72 max-w-full"
                      style={{ aspectRatio: `${w} / ${h}` }}
                    />
                  </div>
                )}
                <div className="mt-2 flex flex-wrap items-center gap-2">
                  {r.output && (
                    <a href={r.output.url} className="btn btn-secondary flex min-h-11 items-center px-3 text-sm">
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
    </section>
  );
}
