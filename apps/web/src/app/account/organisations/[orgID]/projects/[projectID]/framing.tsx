"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type Composition, type MediaAsset, type Project } from "@/lib/queries";

export type FramingKey = { at_ms: number; focus: number };

/** Where a camera path puts the crop at ms (the server's framing.FocusAt). */
export function focusAt(keys: FramingKey[], ms: number): number {
  if (keys.length === 0) return 0.5;
  if (ms <= keys[0].at_ms) return keys[0].focus;
  for (let i = 1; i < keys.length; i++) {
    const a = keys[i - 1];
    const b = keys[i];
    if (ms < b.at_ms) return a.focus + ((b.focus - a.focus) * (ms - a.at_ms)) / (b.at_ms - a.at_ms);
  }
  return keys[keys.length - 1].focus;
}

/** Whether a video's people have been found, so auto framing can follow them. */
export const framingReady = (a: MediaAsset | undefined) => a?.framing?.status === "ready";
export const framingBusy = (a: MediaAsset | undefined) => a?.framing?.status === "pending" || a?.framing?.status === "processing";

/**
 * The camera path auto framing follows for a stretch of a video: the same
 * one the render uses, computed by the API.
 */
export function useFramingPath(
  project: Project,
  asset: MediaAsset | undefined,
  aspect: Composition["aspect"],
  startMS: number,
  endMS: number,
  focus: number,
  enabled: boolean,
) {
  const start = Math.round(startMS);
  const end = Math.round(endMS);
  const f = Math.round(focus * 100) / 100;
  return useQuery({
    queryKey: ["project", project.id, "media", asset?.id, "framing", aspect, start, end, f],
    enabled: enabled && !!asset && framingReady(asset) && end > start,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error, response } = await client().GET(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/framing/path",
        {
          params: {
            path: { orgID: project.organisation_id, projectID: project.id, assetID: asset!.id },
            query: { aspect, start_ms: start, end_ms: end, focus: f },
          },
        },
      );
      if (!data) raise(error, response.status, "Could not load the auto framing.");
      return data.keys;
    },
  });
}

function useRequestFraming(project: Project, asset: MediaAsset) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/framing", {
        params: { path: { orgID: project.organisation_id, projectID: project.id, assetID: asset.id } },
      });
      if (!data) raise(error, response.status, "Could not start finding the people in this video.");
      return data;
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project", project.id, "media"] }),
  });
}

/** One video's framing analysis: its state, and a way to start or retry it. */
export function FramingStatus({ project, asset, canAnalyse }: { project: Project; asset: MediaAsset; canAnalyse: boolean }) {
  const request = useRequestFraming(project, asset);
  const f = asset.framing;
  if (f?.status === "ready") return null;
  return (
    <div className="space-y-1.5">
      {request.isError && <Banner>{request.error.message}</Banner>}
      {framingBusy(asset) ? (
        <p className="flex items-center gap-2 text-xs font-semibold" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
          Finding the people in {asset.filename}… (a few minutes for a long video)
        </p>
      ) : (
        <>
          <p className="text-xs font-semibold text-muted">
            {f?.status === "failed" ? (f.error ?? "Finding the people in this video failed.") : `${asset.filename} hasn’t been analysed for auto framing.`}
          </p>
          {canAnalyse && (
            <button
              type="button"
              onClick={() => request.mutate()}
              disabled={request.isPending}
              className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
            >
              {request.isPending ? "Starting…" : f?.status === "failed" ? "Try again" : "Find the people"}
            </button>
          )}
        </>
      )}
    </div>
  );
}

/**
 * Fixed framing (a slider) or auto framing, which follows the people in
 * shot. Auto needs every video analysed; until then it says why.
 */
export function FramingChoice({
  project,
  assets,
  auto,
  onAuto,
  canAnalyse,
}: {
  project: Project;
  assets: MediaAsset[];
  auto: boolean;
  onAuto: (auto: boolean) => void;
  canAnalyse: boolean;
}) {
  const id = useId();
  const ready = assets.length > 0 && assets.every(framingReady);
  // A choice that can't be rendered any more (a video was swapped in) falls back.
  useEffect(() => {
    if (auto && !ready) onAuto(false);
  }, [auto, ready, onAuto]);
  const options = [
    { value: false, label: "Fixed", hint: "Stays where you put it" },
    { value: true, label: "Follow people", hint: "Moves with whoever’s in shot" },
  ];
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-xs font-semibold text-muted">Camera</legend>
      <div className="grid grid-cols-2 gap-1.5">
        {options.map((o) => (
          <label
            key={o.label}
            className={`flex min-h-11 min-w-0 cursor-pointer flex-col justify-center border border-line px-2 py-1.5 has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
              auto === o.value ? "bg-accent-soft text-accent-text" : "bg-white"
            }`}
          >
            <input
              type="radio"
              name={`${id}-framing`}
              checked={auto === o.value}
              disabled={o.value && !ready}
              onChange={() => onAuto(o.value)}
              className="sr-only"
            />
            <span className="text-sm font-semibold">{o.label}</span>
            <span className="text-xs font-medium">{o.hint}</span>
          </label>
        ))}
      </div>
      {!ready && assets.map((a) => <FramingStatus key={a.id} project={project} asset={a} canAnalyse={canAnalyse} />)}
      {auto && (
        <p className="text-xs font-semibold text-muted">
          Follows the most prominent face, moving smoothly and cutting with the shot. Where nobody’s in shot, the slider sets the frame.
        </p>
      )}
    </fieldset>
  );
}

/**
 * Plays startMS to endMS of a video cropped to its shape, moving the crop
 * along keys as the render will. Muted; the main player has the sound.
 */
export function FramingPreview({
  url,
  startMS,
  endMS,
  keys,
  horizontal,
  label,
}: {
  url: string;
  startMS: number;
  endMS: number;
  keys: FramingKey[];
  horizontal: boolean;
  label: string;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const [playing, setPlaying] = useState(false);
  const [at, setAt] = useState(0);
  const position = (ms: number) => {
    const f = focusAt(keys, ms) * 100;
    return horizontal ? `${f}% 50%` : `50% ${f}%`;
  };

  // Show the first frame until played.
  useEffect(() => {
    const el = video.current;
    if (!el) return;
    const seek = () => {
      el.currentTime = startMS / 1000;
      setAt(0);
    };
    if (el.readyState >= 1) seek();
    else el.addEventListener("loadedmetadata", seek, { once: true });
    return () => el.removeEventListener("loadedmetadata", seek);
  }, [startMS]);

  // While playing, follow the path every frame and stop at the end.
  useEffect(() => {
    const el = video.current;
    if (!el || !playing) return;
    let raf = 0;
    const tick = () => {
      const ms = el.currentTime * 1000 - startMS;
      if (el.currentTime * 1000 >= endMS || el.ended) {
        el.pause();
        setPlaying(false);
        return;
      }
      setAt(ms);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [playing, startMS, endMS]);

  function toggle() {
    const el = video.current;
    if (!el) return;
    if (playing) {
      el.pause();
      setPlaying(false);
      return;
    }
    if (el.currentTime * 1000 < startMS || el.currentTime * 1000 >= endMS - 100) el.currentTime = startMS / 1000;
    void el.play().then(
      () => setPlaying(true),
      () => setPlaying(false),
    );
  }

  return (
    <>
      <video
        ref={video}
        src={url}
        muted
        playsInline
        preload="metadata"
        aria-label={label}
        className="block h-full w-full bg-stage"
        style={{ objectFit: "cover", objectPosition: position(at) }}
      />
      <button
        type="button"
        onClick={toggle}
        aria-label={playing ? "Pause the framing preview" : "Play the clip with auto framing"}
        className="absolute bottom-1.5 left-1.5 grid min-h-11 min-w-11 place-items-center rounded-full bg-stage/80 px-3 text-sm font-semibold text-surface"
      >
        {playing ? "❚❚" : "▶"}
      </button>
    </>
  );
}
