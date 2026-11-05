"use client";

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type DragEvent } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type MediaAsset, type Project } from "@/lib/queries";
import { acceptedFiles, formatBytes, maxUploadBytes, sendFile, UploadAborted } from "@/lib/upload";

type Job = {
  key: string;
  name: string;
  size: number;
  progress: number;
  state: "sending" | "checking" | "done" | "failed" | "cancelled";
  error?: string;
  controller: AbortController;
};

let jobSeq = 0;

export function MediaSection({ project, canUpload }: { project: Project; canUpload: boolean }) {
  const id = useId();
  const qc = useQueryClient();
  const listKey = ["project", project.id, "media"];
  const orgID = project.organisation_id;
  const [jobs, setJobs] = useState<Job[]>([]);

  const media = useInfiniteQuery({
    queryKey: listKey,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/media", {
        params: { path: { orgID, projectID: project.id }, query: { cursor: pageParam } },
      });
      if (!data) raise(error, response.status, "Could not load media.");
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    // Poll while anything is still processing. (Live updates over a
    // WebSocket can replace this later.)
    refetchInterval: (q) =>
      q.state.data?.pages.some((p) => p.data.some((a) => a.processing.status === "pending" || a.processing.status === "processing"))
        ? 2000
        : false,
  });

  const busy = jobs.some((j) => j.state === "sending" || j.state === "checking");
  // Leaving mid-upload loses the upload; ask first.
  useEffect(() => {
    if (!busy) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [busy]);

  const update = (key: string, patch: Partial<Job>) =>
    setJobs((js) => js.map((j) => (j.key === key ? { ...j, ...patch } : j)));

  async function upload(file: File) {
    const job: Job = {
      key: `job-${++jobSeq}`,
      name: file.name,
      size: file.size,
      progress: 0,
      state: "sending",
      controller: new AbortController(),
    };
    setJobs((js) => [job, ...js]);
    if (file.size === 0 || file.size > maxUploadBytes) {
      update(job.key, { state: "failed", error: "Files must be between 1 byte and 5 GB." });
      return;
    }
    try {
      const { data, error, response } = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/uploads",
        {
          params: { path: { orgID, projectID: project.id } },
          body: { filename: file.name, content_type: file.type, size_bytes: file.size },
        },
      );
      if (!data) raise(error, response.status, "Could not start the upload.");
      await sendFile(data.upload, file, (p) => update(job.key, { progress: p }), job.controller.signal);
      update(job.key, { state: "checking", progress: 1 });
      const done = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/complete",
        { params: { path: { orgID, projectID: project.id, assetID: data.asset.id } } },
      );
      if (!done.data) raise(done.error, done.response.status, "Could not finish the upload.");
      update(job.key, { state: "done" });
      await qc.invalidateQueries({ queryKey: listKey });
    } catch (e) {
      if (e instanceof UploadAborted) update(job.key, { state: "cancelled" });
      else update(job.key, { state: "failed", error: e instanceof Error ? e.message : "The upload failed." });
    }
  }

  const items = media.data?.pages.flatMap((p) => p.data) ?? [];
  const visibleJobs = jobs.filter((j) => j.state !== "done");

  return (
    <section aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-white p-5 sm:p-7">
      <div className="flex flex-wrap items-baseline gap-2">
        <h2 id={`${id}-h`} className="mr-auto text-2xl font-extrabold tracking-tight">
          Media {items.length > 0 && <span className="text-ink-soft">· {items.length}{media.hasNextPage ? "+" : ""}</span>}
        </h2>
        {jobs.some((j) => j.state !== "sending" && j.state !== "checking") && !busy && (
          <button type="button" onClick={() => setJobs([])} className="min-h-11 px-2 text-sm font-extrabold underline decoration-2 underline-offset-4">
            Clear finished
          </button>
        )}
      </div>

      {canUpload && <DropZone onFiles={(files) => files.forEach(upload)} />}

      {visibleJobs.length > 0 && (
        <ul className="mt-4 space-y-2" aria-label="Uploads">
          {visibleJobs.map((j) => (
            <UploadRow key={j.key} job={j} onCancel={() => j.controller.abort()} />
          ))}
        </ul>
      )}

      {media.isPending && <div className="mt-5 h-40 animate-pulse rounded-2xl bg-paper-deep" />}
      {media.isError && (
        <div className="mt-5 space-y-3">
          <Banner>{media.error.message}</Banner>
          <button type="button" onClick={() => media.refetch()} className="btn-pop min-h-11 rounded-xl bg-white px-4 font-extrabold">
            Try again
          </button>
        </div>
      )}
      {media.isSuccess && items.length === 0 && !canUpload && (
        <p className="mt-4 font-semibold text-ink-soft">No media in this project yet.</p>
      )}
      {items.length > 0 && (
        <ul className="mt-5 grid grid-cols-1 gap-5 md:grid-cols-2">
          {items.map((a) => (
            <li key={a.id}>
              <MediaCard asset={a} project={project} canUpload={canUpload} />
            </li>
          ))}
        </ul>
      )}
      {media.hasNextPage && (
        <button
          type="button"
          onClick={() => media.fetchNextPage()}
          disabled={media.isFetchingNextPage}
          className="btn-pop mt-5 min-h-12 w-full rounded-2xl bg-white px-6 font-extrabold disabled:opacity-60 sm:w-auto"
        >
          {media.isFetchingNextPage ? "Loading…" : "Show more"}
        </button>
      )}
    </section>
  );
}

function DropZone({ onFiles }: { onFiles: (files: File[]) => void }) {
  const id = useId();
  const [over, setOver] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setOver(false);
    const files = Array.from(e.dataTransfer.files);
    if (files.length) onFiles(files);
  }

  return (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={onDrop}
      className={`mt-4 flex flex-col items-center gap-3 rounded-2xl border-[3px] border-dashed border-ink p-6 text-center transition-colors ${over ? "bg-sun" : "bg-paper"}`}
    >
      <p className="text-lg font-extrabold">Drop footage, audio or stills here</p>
      <p className="text-sm font-semibold text-ink-soft">Video, audio, PNG, JPEG or WebP · up to 5 GB each</p>
      <label htmlFor={`${id}-file`} className="btn-pop flex min-h-12 cursor-pointer items-center rounded-2xl bg-tomato px-6 font-extrabold text-white focus-within:ring-4 focus-within:ring-cobalt/40">
        Choose files
        <input
          ref={input}
          id={`${id}-file`}
          type="file"
          multiple
          accept={acceptedFiles}
          className="sr-only"
          onChange={(e) => {
            const files = Array.from(e.target.files ?? []);
            if (files.length) onFiles(files);
            if (input.current) input.current.value = "";
          }}
        />
      </label>
    </div>
  );
}

function UploadRow({ job, onCancel }: { job: Job; onCancel: () => void }) {
  const pct = Math.round(job.progress * 100);
  const label =
    job.state === "sending" ? `Uploading · ${pct}%` :
    job.state === "checking" ? "Checking the file…" :
    job.state === "cancelled" ? "Cancelled" :
    job.error ?? "Failed";
  return (
    <li className={`rounded-2xl border-2 border-ink p-3 ${job.state === "failed" ? "bg-tomato/15" : "bg-paper"}`}>
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <p className="truncate font-extrabold">{job.name}</p>
          <p className="text-sm font-semibold text-ink-soft" role={job.state === "failed" ? "alert" : undefined}>
            {formatBytes(job.size)} · {label}
          </p>
        </div>
        {job.state === "sending" && (
          <button type="button" onClick={onCancel} className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold">
            Cancel
          </button>
        )}
      </div>
      {(job.state === "sending" || job.state === "checking") && (
        <div
          role="progressbar"
          aria-label={`Uploading ${job.name}`}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={pct}
          className="mt-2 h-3 overflow-hidden rounded-full border-2 border-ink bg-white"
        >
          <div className="h-full bg-cobalt transition-[width]" style={{ width: `${pct}%` }} />
        </div>
      )}
    </li>
  );
}

// Signed URLs change on every fetch. Keep using the first one until it's
// close to expiring so refetches don't restart a playing video.
const urlCache = new Map<string, { url: string; expires: number }>();
function stable(cacheKey: string, url: string | null, expiresAt: string | null) {
  if (!url) return null;
  const cached = urlCache.get(cacheKey);
  if (cached && cached.expires - Date.now() > 5 * 60_000) return cached.url;
  urlCache.set(cacheKey, { url, expires: expiresAt ? Date.parse(expiresAt) : Date.now() });
  return url;
}

const kindColour: Record<string, string> = { video: "bg-sky", audio: "bg-lime", image: "bg-pink" };

function formatDuration(ms: number) {
  const total = Math.round(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

/** A short, human description of what processing found. */
function describe(a: MediaAsset): string[] {
  const m = a.metadata;
  if (!m) return [];
  const out: string[] = [];
  if (m.duration_ms) out.push(formatDuration(m.duration_ms));
  if (m.width && m.height) out.push(`${m.width}×${m.height}`);
  if (m.frame_rate && a.kind === "video") out.push(`${Number(m.frame_rate.toFixed(2))} fps`);
  if (m.video_codec && a.kind === "video") out.push(m.video_codec.toUpperCase());
  if (a.kind === "audio") {
    if (m.audio_channels) out.push(m.audio_channels === 1 ? "Mono" : m.audio_channels === 2 ? "Stereo" : `${m.audio_channels} channels`);
    if (m.sample_rate) out.push(`${Number((m.sample_rate / 1000).toFixed(1))} kHz`);
  }
  if (a.kind === "video" && m.video_codec && !m.audio_codec) out.push("No audio");
  return out;
}

function MediaCard({ asset, project, canUpload }: { asset: MediaAsset; project: Project; canUpload: boolean }) {
  const qc = useQueryClient();
  const url = stable(asset.id, asset.url, asset.url_expires_at);
  const thumb = stable(`${asset.id}:thumb`, asset.thumbnail_url, asset.url_expires_at);
  const path = { orgID: project.organisation_id, projectID: project.id, assetID: asset.id };
  const refresh = () => qc.invalidateQueries({ queryKey: ["project", project.id, "media"] });
  const remove = useMutation({
    mutationFn: async () => {
      const { error, response } = await client().DELETE(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}",
        { params: { path } },
      );
      if (!response.ok) raise(error, response.status, "Could not delete the file.");
    },
    onSuccess: refresh,
  });
  const retry = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/process",
        { params: { path } },
      );
      if (!data) raise(error, response.status, "Could not retry processing.");
    },
    onSettled: refresh,
  });
  const status = asset.processing.status;
  const busy = status === "pending" || status === "processing";
  const details = describe(asset);

  return (
    <article className="flex h-full flex-col overflow-hidden rounded-2xl border-[3px] border-ink bg-paper">
      <div className="relative grid aspect-video place-items-center border-b-[3px] border-ink bg-ink">
        {url && asset.kind === "video" && (
          <video src={url} poster={thumb ?? undefined} controls playsInline preload="metadata" className="h-full w-full" />
        )}
        {url && asset.kind === "image" && (
          // The preview is the processed thumbnail when there is one; the
          // original can be large. Download always gets the original.
          // eslint-disable-next-line @next/next/no-img-element
          <img src={thumb ?? url} alt={asset.filename} loading="lazy" className="h-full w-full object-contain" />
        )}
        {url && asset.kind === "audio" && (
          <div className="flex w-full flex-col items-center gap-4 p-4">
            <span aria-hidden="true" className="text-4xl">🎧</span>
            <audio src={url} controls preload="metadata" className="w-full" />
          </div>
        )}
        {busy && (
          <span className="pointer-events-none absolute top-2 left-2 flex items-center gap-1.5 rounded-full border-2 border-ink bg-sun px-2.5 py-0.5 text-xs font-extrabold">
            <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-ink" />
            Processing…
          </span>
        )}
      </div>
      <div className="flex flex-1 flex-col p-4">
        <p className="font-extrabold break-all">{asset.filename}</p>
        <p className="mt-1 flex flex-wrap items-center gap-2 text-sm font-semibold text-ink-soft">
          {asset.kind && (
            <span className={`rounded-full border-2 border-ink px-2 py-0.5 text-xs font-extrabold text-ink capitalize ${kindColour[asset.kind] ?? "bg-white"}`}>
              {asset.kind}
            </span>
          )}
          <span>{formatBytes(asset.size_bytes)}</span>
          {asset.uploaded_by && <span>· {asset.uploaded_by.name}</span>}
        </p>
        {details.length > 0 && (
          <p className="mt-1 font-mono text-xs font-semibold text-ink-soft" aria-label="File details">
            {details.join(" · ")}
          </p>
        )}
        {status === "failed" && (
          <div className="mt-3 rounded-xl border-2 border-ink bg-tomato/15 p-3 text-sm font-semibold" role="status">
            <p>{asset.processing.error ?? "Processing failed."}</p>
            {canUpload && (
              <button
                type="button"
                disabled={retry.isPending}
                onClick={() => retry.mutate()}
                className="btn-pop mt-2 min-h-11 rounded-xl bg-white px-3 font-extrabold disabled:opacity-60"
              >
                {retry.isPending ? "Retrying…" : "Try again"}
              </button>
            )}
          </div>
        )}
        {remove.isError && <div className="mt-3"><Banner>{remove.error.message}</Banner></div>}
        {retry.isError && <div className="mt-3"><Banner>{retry.error.message}</Banner></div>}
        <div className="mt-auto flex gap-2 pt-4">
          {url && (
            <a href={url} className="btn-pop flex min-h-11 items-center rounded-xl bg-white px-3 text-sm font-extrabold">
              Download
            </a>
          )}
          {asset.can_delete && (
            <button
              type="button"
              disabled={remove.isPending}
              onClick={() => {
                if (window.confirm(`Delete “${asset.filename}”? This can’t be undone.`)) remove.mutate();
              }}
              className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold disabled:opacity-60"
            >
              {remove.isPending ? "Deleting…" : "Delete"}
            </button>
          )}
        </div>
      </div>
    </article>
  );
}
