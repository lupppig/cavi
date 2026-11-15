"use client";

import { useEffect, useId, useRef, useState } from "react";
import type { BriefRule, Composition, MediaAsset, Project } from "@/lib/queries";
import { BriefCheck, FLAG_WINDOW_MS, flagged, span, useBrief } from "./brief-section";
import { shapes, stampTenths } from "./clips";
import { focusAt, useFramingPath } from "./framing";
import { stable } from "./media-section";

/** The API's montage limits (internal/render/montage.go). */
export const MAX_SEGMENTS = 20;
export const MIN_SEGMENT_MS = 500;
const MAX_TREATMENTS = 20;
const MIN_BLUR_PCT = 2;
/** How far either side of a brief range a suggested blur or mute reaches. */
const SUGGEST_PAD_MS = 1500;

export type MuteDraft = { key: number; start: string; end: string };
/** A blur box being edited; x, y, w and h are percentages of the source frame. */
export type BlurDraft = { key: number; start: string; end: string; x: number; y: number; w: number; h: number };
export type SegmentDraft = {
  key: number;
  assetID: string;
  start: string;
  end: string;
  /** 0 to 1, as the API takes it. */
  focus: number;
  mute: MuteDraft[];
  blur: BlurDraft[];
};

let seq = 0;
export const nextKey = () => ++seq;

/** Reads M:SS, M:SS.t or H:MM:SS(.t) into milliseconds, or null. */
export function parseTenths(s: string): number | null {
  const m = s.trim().match(/^(?:(\d+):)?(\d{1,3}):([0-5]\d)(?:\.(\d{1,3}))?$/);
  if (!m) return null;
  return ((Number(m[1] ?? 0) * 60 + Number(m[2])) * 60 + Number(m[3])) * 1000 + Number((m[4] ?? "").padEnd(3, "0"));
}

/** A processed video that can go in a montage. */
export function montageable(a: MediaAsset) {
  return a.kind === "video" && a.status === "uploaded" && a.processing.status === "ready" && !!a.metadata?.duration_ms;
}

export type CheckedSegment = { startMS: number; endMS: number; problem: string | null };

/** Checks a segment the way the API will, so problems show as you type. */
export function checkSegment(d: SegmentDraft, asset: MediaAsset | undefined): CheckedSegment {
  const startMS = parseTenths(d.start);
  const endMS = parseTenths(d.end);
  const bad = (problem: string): CheckedSegment => ({ startMS: startMS ?? 0, endMS: endMS ?? 0, problem });
  if (!asset) return bad("Choose a processed video for this segment.");
  if (startMS === null || endMS === null) return bad("Write times as minutes:seconds, like 1:30 or 1:30.5.");
  if (endMS - startMS < MIN_SEGMENT_MS) return bad("Segments must be at least half a second long.");
  const duration = asset.metadata?.duration_ms ?? 0;
  if (endMS > duration) return bad(`This video is only ${stampTenths(duration)} long.`);
  const inside = (s: string, e: string) => {
    const a = parseTenths(s);
    const b = parseTenths(e);
    return a !== null && b !== null && b > a && a >= startMS && b <= endMS;
  };
  for (const [i, m] of d.mute.entries()) {
    if (!inside(m.start, m.end)) return bad(`Muted stretch ${i + 1} must start and end inside this segment.`);
  }
  for (const [i, b] of d.blur.entries()) {
    if (!inside(b.start, b.end)) return bad(`Blur box ${i + 1} must start and end inside this segment.`);
    if (b.w < MIN_BLUR_PCT || b.h < MIN_BLUR_PCT) return bad(`Blur box ${i + 1} must be at least 2% wide and tall.`);
    if (b.x < 0 || b.y < 0 || b.x + b.w > 100 || b.y + b.h > 100) return bad(`Blur box ${i + 1} must be inside the frame.`);
  }
  return { startMS, endMS, problem: null };
}

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi);
const pct = (v: number) => Math.round(v * 10) / 10;

/**
 * Where a brief rule applies, kept inside the segment: a single moment
 * spans the whole flag window (what it names usually stays on screen
 * around it); a range is padded a little.
 */
function around(r: BriefRule, startMS: number, endMS: number) {
  const pad = r.start_ms === r.end_ms ? FLAG_WINDOW_MS : SUGGEST_PAD_MS;
  const from = clamp(r.start_ms - pad, startMS, endMS - MIN_SEGMENT_MS);
  const to = clamp(r.end_ms + pad, from + MIN_SEGMENT_MS, endMS);
  return { start: stampTenths(from), end: stampTenths(to) };
}

/** Whether an entry already covers the rule's moment. */
function covers(entries: { start: string; end: string }[], r: BriefRule) {
  return entries.some((e) => {
    const a = parseTenths(e.start);
    const b = parseTenths(e.end);
    return a !== null && b !== null && a <= r.start_ms && b >= r.end_ms;
  });
}

const timeInput = "mt-1 block min-h-11 w-full border border-line bg-white px-2 font-mono text-base";

/** One stretch of a montage: which file, where, framing, and what to mute or blur. */
export function SegmentEditor({
  project,
  index,
  count,
  draft,
  files,
  aspect,
  auto,
  offsetMS,
  onChange,
  onMove,
  onRemove,
}: {
  project: Project;
  index: number;
  count: number;
  draft: SegmentDraft;
  files: MediaAsset[];
  aspect: Composition["aspect"];
  /** The montage follows people (auto framing). */
  auto: boolean;
  /** Where this segment starts in the montage. */
  offsetMS: number;
  onChange: (patch: Partial<SegmentDraft>) => void;
  onMove: (by: -1 | 1) => void;
  onRemove: () => void;
}) {
  const id = useId();
  const asset = files.find((f) => f.id === draft.assetID);
  const checked = checkSegment(draft, asset);
  const timesOK = !!asset && parseTenths(draft.start) !== null && parseTenths(draft.end) !== null && checked.endMS > checked.startMS;
  const [active, setActive] = useState<number | null>(null);
  const activeBlur = draft.blur.find((b) => b.key === active);
  const brief = useBrief(project);
  const n = index + 1;

  const suggestions =
    asset && timesOK && brief.data?.status === "ready"
      ? brief.data.rules.filter(
          (r) =>
            r.media_asset_id === asset.id &&
            (r.kind === "blur" || r.kind === "mute") &&
            flagged(r, checked.startMS, checked.endMS) &&
            !covers(r.kind === "blur" ? draft.blur : draft.mute, r),
        )
      : [];

  const setMute = (key: number, patch: Partial<MuteDraft>) =>
    onChange({ mute: draft.mute.map((m) => (m.key === key ? { ...m, ...patch } : m)) });
  const setBlur = (key: number, patch: Partial<BlurDraft>) =>
    onChange({ blur: draft.blur.map((b) => (b.key === key ? { ...b, ...patch } : b)) });

  function addBlur(at?: { start: string; end: string }) {
    const b: BlurDraft = { key: nextKey(), ...(at ?? { start: draft.start, end: draft.end }), x: 40, y: 40, w: 20, h: 20 };
    onChange({ blur: [...draft.blur, b] });
    setActive(b.key);
  }
  function addMute(at?: { start: string; end: string }) {
    onChange({ mute: [...draft.mute, { key: nextKey(), ...(at ?? { start: draft.start, end: draft.end }) }] });
  }

  // The preview shows the active blur box's first frame, or the segment's.
  const seekMS = (activeBlur && parseTenths(activeBlur.start)) ?? (timesOK ? checked.startMS : 0);

  return (
    <li className="rounded-xl border border-line bg-surface-2 p-2.5">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <h4 id={`${id}-h`} className="font-semibold">Segment {n}</h4>
        {!checked.problem && (
          <span className="font-mono text-xs font-bold text-muted">
            plays {stampTenths(offsetMS)}–{stampTenths(offsetMS + checked.endMS - checked.startMS)}
          </span>
        )}
        <div className="ml-auto flex gap-1">
          <button
            type="button"
            onClick={() => onMove(-1)}
            disabled={index === 0}
            aria-label={`Move segment ${n} earlier`}
            className="btn btn-ghost grid min-h-11 min-w-11 place-items-center text-sm disabled:opacity-40"
          >
            ↑
          </button>
          <button
            type="button"
            onClick={() => onMove(1)}
            disabled={index === count - 1}
            aria-label={`Move segment ${n} later`}
            className="btn btn-ghost grid min-h-11 min-w-11 place-items-center text-sm disabled:opacity-40"
          >
            ↓
          </button>
          <button
            type="button"
            onClick={onRemove}
            disabled={count === 1}
            aria-label={`Remove segment ${n}`}
            className="btn btn-ghost min-h-11 px-3 text-sm disabled:opacity-40"
          >
            Remove
          </button>
        </div>
      </div>

      <div className="mt-2 grid gap-3 @[40rem]:grid-cols-[minmax(0,1fr)_minmax(0,18rem)]">
        <div className="min-w-0 space-y-2">
          <label className="block text-xs font-semibold">
            Video
            <select
              value={asset ? draft.assetID : ""}
              onChange={(e) => onChange({ assetID: e.target.value })}
              className="mt-1 block min-h-11 w-full min-w-0 border border-line bg-white px-2 text-sm font-semibold"
            >
              {!asset && <option value="">{draft.assetID ? "That file isn’t a usable video — choose one" : "Choose a video…"}</option>}
              {files.map((f) => (
                <option key={f.id} value={f.id}>
                  {f.filename} ({stampTenths(f.metadata?.duration_ms ?? 0)})
                </option>
              ))}
            </select>
          </label>
          <div className="grid grid-cols-2 gap-2">
            <label className="text-xs font-semibold">
              From
              <input
                value={draft.start}
                onChange={(e) => onChange({ start: e.target.value })}
                inputMode="decimal"
                placeholder="1:30.0"
                aria-label={`Segment ${n} start`}
                className={timeInput}
              />
            </label>
            <label className="text-xs font-semibold">
              To
              <input
                value={draft.end}
                onChange={(e) => onChange({ end: e.target.value })}
                inputMode="decimal"
                placeholder="1:52.0"
                aria-label={`Segment ${n} end`}
                className={timeInput}
              />
            </label>
          </div>
          {checked.problem ? (
            <p className="text-sm font-semibold text-danger">{checked.problem}</p>
          ) : (
            <p className="text-xs font-semibold text-muted">{stampTenths(checked.endMS - checked.startMS)} long · times are in the source video</p>
          )}
          {asset && timesOK && <BriefCheck project={project} asset={asset} startMS={checked.startMS} endMS={checked.endMS} />}
          {suggestions.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {suggestions.map((r, i) =>
                r.kind === "blur" ? (
                  <button
                    key={i}
                    type="button"
                    onClick={() => addBlur(around(r, checked.startMS, checked.endMS))}
                    className="btn btn-secondary min-h-11 px-3 text-sm"
                  >
                    Add blur at {span(r)}
                  </button>
                ) : (
                  <button
                    key={i}
                    type="button"
                    onClick={() => addMute(around(r, checked.startMS, checked.endMS))}
                    className="btn btn-secondary min-h-11 px-3 text-sm"
                  >
                    Mute {span(r)}
                  </button>
                ),
              )}
            </div>
          )}

          <fieldset>
            <legend className="text-xs font-semibold uppercase text-muted">Muted audio</legend>
            {draft.mute.length > 0 && (
              <ul className="mt-1 space-y-1.5">
                {draft.mute.map((m, j) => (
                  <li key={m.key} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-end gap-2">
                    <label className="text-xs font-semibold">
                      From
                      <input value={m.start} onChange={(e) => setMute(m.key, { start: e.target.value })} inputMode="decimal" aria-label={`Segment ${n} muted stretch ${j + 1} start`} className={timeInput} />
                    </label>
                    <label className="text-xs font-semibold">
                      To
                      <input value={m.end} onChange={(e) => setMute(m.key, { end: e.target.value })} inputMode="decimal" aria-label={`Segment ${n} muted stretch ${j + 1} end`} className={timeInput} />
                    </label>
                    <button
                      type="button"
                      onClick={() => onChange({ mute: draft.mute.filter((x) => x.key !== m.key) })}
                      aria-label={`Remove muted stretch ${j + 1}`}
                      className="btn btn-ghost min-h-11 px-3 text-sm"
                    >
                      Remove
                    </button>
                  </li>
                ))}
              </ul>
            )}
            <button
              type="button"
              onClick={() => addMute()}
              disabled={draft.mute.length >= MAX_TREATMENTS}
              className="btn btn-ghost mt-1 min-h-11 px-3 text-sm disabled:opacity-40"
            >
              + Mute a stretch
            </button>
          </fieldset>

          <fieldset>
            <legend className="text-xs font-semibold uppercase text-muted">Blurred areas</legend>
            {draft.blur.length > 0 && (
              <ul className="mt-1 space-y-2">
                {draft.blur.map((b, j) => (
                  <li
                    key={b.key}
                    className={`space-y-1.5 border p-2 ${active === b.key ? "border-accent bg-white" : "border-line bg-white"}`}
                  >
                    <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-2">
                      <label className="text-xs font-semibold">
                        From
                        <input value={b.start} onFocus={() => setActive(b.key)} onChange={(e) => setBlur(b.key, { start: e.target.value })} inputMode="decimal" aria-label={`Segment ${n} blur box ${j + 1} start`} className={timeInput} />
                      </label>
                      <label className="text-xs font-semibold">
                        To
                        <input value={b.end} onFocus={() => setActive(b.key)} onChange={(e) => setBlur(b.key, { end: e.target.value })} inputMode="decimal" aria-label={`Segment ${n} blur box ${j + 1} end`} className={timeInput} />
                      </label>
                    </div>
                    <div className="grid grid-cols-4 gap-1.5">
                      {(["x", "y", "w", "h"] as const).map((k) => (
                        <label key={k} className="min-w-0 text-xs font-semibold">
                          {{ x: "Left %", y: "Top %", w: "Width %", h: "Height %" }[k]}
                          <input
                            type="number"
                            min={k === "w" || k === "h" ? MIN_BLUR_PCT : 0}
                            max={100}
                            step={1}
                            value={b[k]}
                            onFocus={() => setActive(b.key)}
                            onChange={(e) => setBlur(b.key, { [k]: Number(e.target.value) })}
                            aria-label={`Segment ${n} blur box ${j + 1} ${{ x: "left", y: "top", w: "width", h: "height" }[k]} percent`}
                            className="mt-1 block min-h-11 w-full min-w-0 border border-line bg-white px-1.5 font-mono text-base"
                          />
                        </label>
                      ))}
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {active !== b.key && (
                        <button type="button" onClick={() => setActive(b.key)} className="btn btn-ghost min-h-11 px-3 text-sm">
                          Show on frame
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() => {
                          onChange({ blur: draft.blur.filter((x) => x.key !== b.key) });
                          if (active === b.key) setActive(null);
                        }}
                        aria-label={`Remove blur box ${j + 1}`}
                        className="btn btn-ghost min-h-11 px-3 text-sm"
                      >
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
            <button
              type="button"
              onClick={() => addBlur()}
              disabled={draft.blur.length >= MAX_TREATMENTS}
              className="btn btn-ghost mt-1 min-h-11 px-3 text-sm disabled:opacity-40"
            >
              + Blur an area
            </button>
          </fieldset>
        </div>

        {asset && (
          <SegmentFrame
            project={project}
            asset={asset}
            aspect={aspect}
            focus={draft.focus}
            auto={auto && timesOK}
            startMS={checked.startMS}
            endMS={checked.endMS}
            seekMS={seekMS}
            blur={draft.blur}
            active={active}
            onFocus={(focus) => onChange({ focus })}
            onPlace={(x, y) => {
              if (!activeBlur) return;
              setBlur(activeBlur.key, {
                x: pct(clamp(x - activeBlur.w / 2, 0, 100 - activeBlur.w)),
                y: pct(clamp(y - activeBlur.h / 2, 0, 100 - activeBlur.h)),
              });
            }}
          />
        )}
      </div>
    </li>
  );
}

/**
 * The whole source frame: what the chosen shape keeps (outlined, the rest
 * dimmed, using the renderer's framing rule) and each blur box, blurred
 * live where it sits. Tapping the frame moves the selected box there.
 */
function SegmentFrame({
  project,
  asset,
  aspect,
  focus,
  auto,
  startMS,
  endMS,
  seekMS,
  blur,
  active,
  onFocus,
  onPlace,
}: {
  project: Project;
  asset: MediaAsset;
  aspect: Composition["aspect"];
  focus: number;
  /** Auto framing: the outline follows the camera path at seekMS. */
  auto: boolean;
  startMS: number;
  endMS: number;
  seekMS: number;
  blur: BlurDraft[];
  active: number | null;
  onFocus: (focus: number) => void;
  onPlace: (xPct: number, yPct: number) => void;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const url = stable(asset.id, asset.url, asset.url_expires_at);
  const srcW = asset.metadata?.width ?? 16;
  const srcH = asset.metadata?.height ?? 9;
  const shape = shapes.find((s) => s.aspect === aspect)!;
  const horizontal = srcW * shape.h > srcH * shape.w; // cropped left/right
  const crops = srcW * shape.h !== srcH * shape.w;
  const keepW = horizontal ? (srcH * shape.w) / shape.h / srcW : 1;
  const keepH = horizontal ? 1 : (srcW * shape.h) / shape.w / srcH;
  const path = useFramingPath(project, asset, aspect, startMS, endMS, focus, auto && crops);
  const at = auto && path.data ? focusAt(path.data, seekMS - startMS) : focus;
  const keep = { left: (1 - keepW) * at, top: (1 - keepH) * at, width: keepW, height: keepH };

  useEffect(() => {
    const el = video.current;
    if (!el) return;
    const seek = () => {
      el.currentTime = seekMS / 1000;
    };
    if (el.readyState >= 1) seek();
    else el.addEventListener("loadedmetadata", seek, { once: true });
    return () => el.removeEventListener("loadedmetadata", seek);
  }, [seekMS]);

  const hasActive = blur.some((b) => b.key === active);

  return (
    <div className="min-w-0">
      <div className="grid place-items-center bg-stage p-1.5">
        <div
          className={`relative overflow-hidden ${hasActive ? "cursor-crosshair" : ""}`}
          style={{ width: `min(100%, calc(14rem * ${srcW} / ${srcH}))`, aspectRatio: `${srcW} / ${srcH}` }}
          onClick={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            onPlace(((e.clientX - r.left) / r.width) * 100, ((e.clientY - r.top) / r.height) * 100);
          }}
        >
          {url ? (
            <video
              ref={video}
              src={url}
              muted
              playsInline
              preload="metadata"
              aria-label={`${asset.filename} at ${stampTenths(seekMS)}`}
              className="pointer-events-none block h-full w-full bg-stage object-fill"
            />
          ) : (
            <span className="grid h-full place-items-center text-xs font-semibold text-surface">No preview</span>
          )}
          {crops && (
            <>
              {/* Dim what the shape cuts off, either side of what it keeps. */}
              <span
                aria-hidden="true"
                className="pointer-events-none absolute top-0 left-0 bg-stage/60"
                style={horizontal ? { width: `${keep.left * 100}%`, height: "100%" } : { width: "100%", height: `${keep.top * 100}%` }}
              />
              <span
                aria-hidden="true"
                className="pointer-events-none absolute right-0 bottom-0 bg-stage/60"
                style={
                  horizontal
                    ? { width: `${(1 - keep.left - keep.width) * 100}%`, height: "100%" }
                    : { width: "100%", height: `${(1 - keep.top - keep.height) * 100}%` }
                }
              />
              <span
                aria-hidden="true"
                className="pointer-events-none absolute border-2 border-surface"
                style={{ left: `${keep.left * 100}%`, top: `${keep.top * 100}%`, width: `${keep.width * 100}%`, height: `${keep.height * 100}%` }}
              />
            </>
          )}
          {blur.map((b, j) => (
            <span
              key={b.key}
              aria-hidden="true"
              className={`pointer-events-none absolute backdrop-blur-md ${b.key === active ? "border-2 border-accent" : "border border-dashed border-surface"}`}
              style={{ left: `${b.x}%`, top: `${b.y}%`, width: `${b.w}%`, height: `${b.h}%` }}
            >
              <span className="absolute top-0 left-0 bg-stage px-1 font-mono text-[10px] font-bold text-surface">{j + 1}</span>
            </span>
          ))}
        </div>
      </div>
      <p className="mt-1 text-xs font-semibold text-muted">
        {hasActive
          ? "Tap the frame to move the selected blur box there, then fine-tune it with the numbers."
          : crops
            ? auto
              ? `The outline is what ${aspect} keeps at ${stampTenths(seekMS)}; it moves with the people in shot.`
              : `The outline is what ${aspect} keeps.`
            : `The whole frame fits ${aspect}.`}
      </p>
      {crops && (
        <label className="mt-1 block">
          <span className="text-xs font-semibold text-muted">
            {auto ? "Where the frame sits when nobody’s in shot" : "Framing"} ({horizontal ? "left to right" : "top to bottom"})
          </span>
          <input
            type="range"
            min={0}
            max={100}
            value={Math.round(focus * 100)}
            onChange={(e) => onFocus(Number(e.target.value) / 100)}
            className="mt-1 block h-11 w-full accent-[var(--ink)]"
          />
        </label>
      )}
    </div>
  );
}
