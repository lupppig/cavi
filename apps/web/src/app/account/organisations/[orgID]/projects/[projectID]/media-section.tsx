"use client";

import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useId, useRef, useState, type DragEvent } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type MediaAsset, type Project } from "@/lib/queries";
import { acceptedFiles, formatBytes, maxUploadBytes, sendFile, UploadAborted } from "@/lib/upload";
import { ClipsPanel } from "./clips";
import { framingBusy } from "./framing";
import { HighlightsPanel } from "./highlights-panel";
import { ImportLinkForm, importing, shortLink } from "./import-link";
import { TranscriptPanel } from "./transcript-panel";

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

/**
 * The studio: the project's media library beside the selected file, with
 * its transcript, highlights and clips.
 */
export function Studio({
  project,
  canUpload,
  canTranscribe,
  canRender,
}: {
  project: Project;
  canUpload: boolean;
  canTranscribe: boolean;
  canRender: boolean;
}) {
  const id = useId();
  const qc = useQueryClient();
  const listKey = ["project", project.id, "media"];
  const orgID = project.organisation_id;
  const [jobs, setJobs] = useState<Job[]>([]);
  const [linking, setLinking] = useState(false);
  const router = useRouter();

  const media = useProjectMedia(project);

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
  const search = useSearchParams();
  const chosen = search.get("media");
  // Wide screens open the first file; phones show the library until one is picked.
  const selected = items.find((a) => a.id === chosen) ?? (chosen ? undefined : items[0]);
  const hrefFor = (assetID: string | null) => {
    const q = new URLSearchParams(search.toString());
    if (assetID) q.set("media", assetID);
    else q.delete("media");
    q.delete("tab");
    const str = q.toString();
    return str ? `?${str}` : "?";
  };

  return (
    <div className="lg:grid lg:grid-cols-[19rem_minmax(0,1fr)] lg:items-start lg:gap-6">
      <section
        aria-labelledby={`${id}-h`}
        className={`card lg:sticky lg:top-6 ${chosen ? "hidden lg:block" : ""}`}
        onDragOver={(e) => {
          if (canUpload) e.preventDefault();
        }}
        onDrop={(e) => {
          if (!canUpload) return;
          e.preventDefault();
          Array.from(e.dataTransfer.files).forEach(upload);
        }}
      >
        <div className="flex min-h-14 items-center gap-2 border-b border-line px-3">
          <h2 id={`${id}-h`} className="mr-auto text-xl">
            Media{items.length > 0 && <span className="text-muted"> · {items.length}{media.hasNextPage ? "+" : ""}</span>}
          </h2>
          {canUpload && (
            <button
              type="button"
              onClick={() => setLinking((l) => !l)}
              aria-expanded={linking}
              className="btn btn-secondary min-h-11 px-3 text-sm"
            >
              {linking ? "Close" : "+ Link"}
            </button>
          )}
          {canUpload && <UploadButton onFiles={(files) => files.forEach(upload)} />}
        </div>
        {canUpload && linking && (
          <ImportLinkForm
            project={project}
            onDone={(a) => {
              setLinking(false);
              router.push(hrefFor(a.id), { scroll: false });
            }}
          />
        )}

        {visibleJobs.length > 0 && (
          <ul className="space-y-2 border-b border-line p-2" aria-label="Uploads">
            {visibleJobs.map((j) => (
              <UploadRow key={j.key} job={j} onCancel={() => j.controller.abort()} />
            ))}
            {!busy && (
              <li>
                <button type="button" onClick={() => setJobs([])} className="min-h-11 px-1 text-sm font-semibold underline decoration-2 underline-offset-4">
                  Clear finished
                </button>
              </li>
            )}
          </ul>
        )}

        {media.isPending && <div className="m-3 h-40 animate-pulse bg-surface-2" />}
        {media.isError && (
          <div className="space-y-3 p-3">
            <Banner>{media.error.message}</Banner>
            <button type="button" onClick={() => media.refetch()} className="btn btn-secondary min-h-11 px-4">
              Try again
            </button>
          </div>
        )}
        {media.isSuccess && items.length === 0 &&
          (canUpload ? (
            <DropZone onFiles={(files) => files.forEach(upload)} />
          ) : (
            <p className="p-4 font-semibold text-muted">No media in this project yet.</p>
          ))}
        {items.length > 0 && (
          <ul aria-label="Files" className="max-h-[70vh] overflow-y-auto">
            {items.map((a) => (
              <li key={a.id}>
                <MediaRow
                  asset={a}
                  href={hrefFor(a.id)}
                  selected={a.id === chosen ? "always" : !chosen && a.id === selected?.id ? "wide" : "no"}
                />
              </li>
            ))}
          </ul>
        )}
        {media.hasNextPage && (
          <button
            type="button"
            onClick={() => media.fetchNextPage()}
            disabled={media.isFetchingNextPage}
            className="min-h-11 w-full border-t border-line px-3 text-sm font-semibold disabled:opacity-60"
          >
            {media.isFetchingNextPage ? "Loading…" : "Show more"}
          </button>
        )}
        {canUpload && items.length > 0 && (
          <p className="border-t border-line px-3 py-2 text-xs font-semibold text-muted">Drop files here to upload. Up to 5 GB each.</p>
        )}
      </section>

      <div className={`${chosen ? "" : "hidden lg:block"} min-w-0`}>
        {chosen && (
          <Link href={hrefFor(null)} scroll={false} className="mb-3 inline-flex min-h-11 items-center font-semibold underline decoration-2 underline-offset-4 lg:hidden">
            ← All media
          </Link>
        )}
        {selected ? (
          <MediaViewer
            key={selected.id}
            asset={selected}
            project={project}
            canUpload={canUpload}
            canTranscribe={canTranscribe}
            canRender={canRender}
          />
        ) : (
          media.isSuccess && (
            <div className="grid min-h-64 place-items-center border border-dashed border-line-strong p-8 text-center">
              <p className="max-w-sm font-semibold text-muted">
                {chosen ? "That file isn’t in this project any more." : canUpload ? "Upload footage, audio or stills to start." : "Nothing to show yet."}
              </p>
            </div>
          )
        )}
      </div>
    </div>
  );
}

function UploadButton({ onFiles }: { onFiles: (files: File[]) => void }) {
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  return (
    <label htmlFor={`${id}-file`} className="btn btn-primary flex min-h-11 cursor-pointer items-center px-3 text-sm focus-within:ring-4 focus-within:ring-accent/30">
      + Upload
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
  );
}

/** One file in the library: thumbnail, name, length and where it's at. */
// "wide": the file shown by default on wide screens (phones show the
// library alone, so nothing is selected there).
function MediaRow({ asset, href, selected }: { asset: MediaAsset; href: string; selected: "always" | "wide" | "no" }) {
  const thumb = stable(`${asset.id}:thumb`, asset.thumbnail_url, asset.url_expires_at);
  const status = asset.processing.status;
  const state =
    asset.status === "importing"
      ? asset.import_error
        ? "Import failed"
        : "Importing…"
      : status === "pending" || status === "processing"
        ? "Processing…"
        : status === "failed"
          ? "Couldn’t process"
          : null;
  const length = asset.metadata?.duration_ms ? formatDuration(asset.metadata.duration_ms) : null;
  return (
    <Link
      href={href}
      scroll={false}
      aria-current={selected === "always" ? "true" : undefined}
      className={`flex min-h-16 items-center gap-3 border-b border-line px-3 py-2 transition-colors ${
        selected === "always" ? "bg-accent-soft text-accent-text" : selected === "wide" ? "hover:bg-surface-2 lg:bg-accent-soft lg:text-accent-text" : "hover:bg-surface-2"
      }`}
    >
      <span className="grid h-10 w-16 shrink-0 place-items-center overflow-hidden border border-current bg-accent-soft text-accent-text">
        {thumb ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={thumb} alt="" className="h-full w-full object-cover" />
        ) : (
          <span className="font-mono text-[11px] font-bold uppercase">{asset.kind ?? "file"}</span>
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-semibold">{asset.filename}</span>
        <span className="block truncate font-mono text-xs font-semibold uppercase opacity-80">
          {[asset.kind, length, state].filter(Boolean).join(" · ")}
        </span>
      </span>
    </Link>
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
      className={`m-3 flex flex-col items-center gap-3 border border-dashed border-line-strong p-6 text-center transition-colors ${over ? "bg-surface-2" : "bg-surface"}`}
    >
      <p className="text-lg font-semibold">Drop footage, audio or stills here</p>
      <p className="text-sm font-semibold text-muted">Video, audio, PNG, JPEG or WebP · up to 5 GB each</p>
      <label htmlFor={`${id}-file`} className="btn btn-primary flex min-h-12 cursor-pointer items-center px-6 focus-within:ring-4 focus-within:ring-accent/30">
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
    <li className={`border border-line p-3 ${job.state === "failed" ? "bg-surface-2" : "bg-surface"}`}>
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <p className="truncate font-semibold">{job.name}</p>
          <p className="text-sm font-semibold text-muted" role={job.state === "failed" ? "alert" : undefined}>
            {formatBytes(job.size)} · {label}
          </p>
        </div>
        {job.state === "sending" && (
          <button type="button" onClick={onCancel} className="btn btn-secondary min-h-11 px-3 text-sm">
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
          className="mt-2 h-2 overflow-hidden rounded-full bg-surface-2"
        >
          <div className="h-full rounded-full bg-accent transition-[width]" style={{ width: `${pct}%` }} />
        </div>
      )}
    </li>
  );
}

// Signed URLs change on every fetch. Keep using the first one until it's
// close to expiring so refetches don't restart a playing video.
const urlCache = new Map<string, { url: string; expires: number }>();
export function stable(cacheKey: string, url: string | null, expiresAt: string | null) {
  if (!url) return null;
  const cached = urlCache.get(cacheKey);
  if (cached && cached.expires - Date.now() > 5 * 60_000) return cached.url;
  urlCache.set(cacheKey, { url, expires: expiresAt ? Date.parse(expiresAt) : Date.now() });
  return url;
}

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

type ViewerTab = "transcript" | "highlights" | "clips";

/** The selected file: a large player, its details and actions, and tabs for what you can do with it. */
function MediaViewer({
  asset,
  project,
  canUpload,
  canTranscribe,
  canRender,
}: {
  asset: MediaAsset;
  project: Project;
  canUpload: boolean;
  canTranscribe: boolean;
  canRender: boolean;
}) {
  const qc = useQueryClient();
  const player = useRef<HTMLMediaElement>(null);
  // Where to pause when playing a clip; null plays on.
  const stopAt = useRef<number | null>(null);
  const play = (startMS: number, endMS: number | null) => {
    const el = player.current;
    if (!el) return;
    stopAt.current = endMS === null ? null : endMS / 1000;
    el.currentTime = startMS / 1000;
    void el.play().catch(() => {});
    el.scrollIntoView({ block: "nearest", behavior: "smooth" });
  };
  const seek = (ms: number) => play(ms, null);
  const playClip = (startMS: number, endMS: number) => play(startMS, endMS);
  const onTimeUpdate = (e: React.SyntheticEvent<HTMLMediaElement>) => {
    const el = e.currentTarget;
    if (stopAt.current !== null && el.currentTime >= stopAt.current) {
      stopAt.current = null;
      el.pause();
    }
  };
  // Seeking past the clip's end yourself ends clip playback.
  const onSeeking = (e: React.SyntheticEvent<HTMLMediaElement>) => {
    if (stopAt.current !== null && e.currentTarget.currentTime > stopAt.current) stopAt.current = null;
  };
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
  const incoming = asset.status === "importing";
  const busy = !incoming && (status === "pending" || status === "processing");
  const details = describe(asset);
  const search = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const spoken = status === "ready" && !!asset.metadata?.audio_codec && (asset.kind === "video" || asset.kind === "audio");
  const clippable = status === "ready" && asset.kind === "video" && !!url;
  const tabs: { tab: ViewerTab; label: string }[] = [
    ...(spoken ? [{ tab: "transcript" as const, label: "Transcript" }, { tab: "highlights" as const, label: "Highlights" }] : []),
    ...(clippable ? [{ tab: "clips" as const, label: "Clips" }] : []),
  ];
  const tab = tabs.find((t) => t.tab === search.get("tab"))?.tab ?? tabs[0]?.tab;
  const setTab = (t: ViewerTab) => {
    const q = new URLSearchParams(search.toString());
    q.set("tab", t);
    router.replace(`${pathname}?${q}`, { scroll: false });
  };

  return (
    <article className="card">
      <header className="flex flex-col gap-3 border-b border-line p-3 sm:flex-row sm:items-start sm:p-4">
        <div className="min-w-0 flex-1">
          <h2 className="text-xl break-all normal-case [font-stretch:100%] sm:text-2xl">{asset.filename}</h2>
          <p className="mt-1 flex flex-wrap items-center gap-x-2 font-mono text-xs font-semibold uppercase text-muted">
            {[asset.kind, ...details, incoming ? null : formatBytes(asset.size_bytes), asset.uploaded_by?.name].filter(Boolean).join(" · ")}
          </p>
        </div>
        <div className="flex shrink-0 gap-2">
          {url && (
            <a href={url} className="btn btn-secondary flex min-h-11 items-center px-3 text-sm">
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
              className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
            >
              {remove.isPending ? "Deleting…" : "Delete"}
            </button>
          )}
        </div>
      </header>
      <div className="relative grid min-h-48 place-items-center bg-stage">
        {url && asset.kind === "video" && (
          <video
            ref={player as React.RefObject<HTMLVideoElement>}
            src={url}
            poster={thumb ?? undefined}
            controls
            playsInline
            preload="metadata"
            onTimeUpdate={onTimeUpdate}
            onSeeking={onSeeking}
            className="max-h-[60vh] w-full"
          />
        )}
        {url && asset.kind === "image" && (
          // The preview is the processed thumbnail when there is one; the
          // original can be large. Download always gets the original.
          // eslint-disable-next-line @next/next/no-img-element
          <img src={thumb ?? url} alt={asset.filename} loading="lazy" className="max-h-[60vh] w-full object-contain" />
        )}
        {url && asset.kind === "audio" && (
          <div className="flex w-full flex-col items-center gap-4 p-6 sm:p-10">
            <span aria-hidden="true" className="flex h-10 items-center gap-1">
              {[0.4, 0.9, 0.6, 1, 0.5, 0.8, 0.35].map((h, i) => (
                <span key={i} className="block w-1.5 bg-surface" style={{ height: `${h * 100}%` }} />
              ))}
            </span>
            <audio
              ref={player as React.RefObject<HTMLAudioElement>}
              src={url}
              controls
              preload="metadata"
              onTimeUpdate={onTimeUpdate}
              onSeeking={onSeeking}
              className="w-full"
            />
          </div>
        )}
        {busy && (
          <span className="pointer-events-none absolute top-2 left-2 flex items-center gap-1.5 border border-line bg-white px-2.5 py-0.5 text-xs font-semibold">
            <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
            Processing…
          </span>
        )}
      </div>
      {incoming && (
        <div className="m-3 space-y-1 border border-line bg-surface-2 p-3 text-sm font-semibold sm:m-4" role="status">
          {asset.import_error ? (
            <>
              <p>{asset.import_error}</p>
              <p className="text-xs text-muted">Delete this file, then paste the link again to retry.</p>
            </>
          ) : (
            <p className="flex items-center gap-2">
              <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
              Importing… Long videos take a few minutes. You can leave this page.
            </p>
          )}
          {asset.source_url && (
            <p className="truncate text-xs text-muted">
              From{" "}
              <a href={asset.source_url} target="_blank" rel="noreferrer" className="underline decoration-2 underline-offset-2">
                {shortLink(asset.source_url)}
              </a>
            </p>
          )}
        </div>
      )}
      {!incoming && status === "failed" && (
        <div className="m-3 border border-line bg-surface-2 p-3 text-sm font-semibold sm:m-4" role="status">
          <p>{asset.processing.error ?? "Processing failed."}</p>
          {canUpload && (
            <button
              type="button"
              disabled={retry.isPending}
              onClick={() => retry.mutate()}
              className="btn btn-secondary mt-2 min-h-11 px-3 disabled:opacity-60"
            >
              {retry.isPending ? "Retrying…" : "Try again"}
            </button>
          )}
        </div>
      )}
      {remove.isError && <div className="m-3"><Banner>{remove.error.message}</Banner></div>}
      {retry.isError && <div className="m-3"><Banner>{retry.error.message}</Banner></div>}

      {tabs.length > 0 && (
        <>
          <div role="tablist" aria-label="Work with this file" className="flex border-b border-line px-1 sm:px-2">
            {tabs.map((t) => (
              <button
                key={t.tab}
                type="button"
                role="tab"
                id={`${asset.id}-${t.tab}-tab`}
                aria-selected={tab === t.tab}
                aria-controls={`${asset.id}-${t.tab}-panel`}
                onClick={() => setTab(t.tab)}
                className={`min-h-12 min-w-0 flex-1 px-3 text-sm font-semibold transition-colors sm:flex-none sm:px-5 ${
                  tab === t.tab ? "text-ink shadow-[inset_0_-2px_0_var(--accent)]" : "text-muted hover:text-ink"
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>
          <div role="tabpanel" id={`${asset.id}-${tab}-panel`} aria-labelledby={`${asset.id}-${tab}-tab`} className="p-3 sm:p-4 [&>*:first-child]:mt-0">
            {tab === "transcript" && <TranscriptPanel asset={asset} project={project} canTranscribe={canTranscribe} onSeek={seek} />}
            {tab === "highlights" && (
              <HighlightsPanel asset={asset} project={project} canEdit={canTranscribe} canRender={canRender} url={url} onPlay={playClip} />
            )}
            {tab === "clips" && url && (
              <ClipsPanel
                asset={asset}
                project={project}
                url={url}
                canRender={canRender}
                canTranscribe={canTranscribe}
                playheadMS={() => (player.current?.currentTime ?? 0) * 1000}
                onPreview={playClip}
              />
            )}
          </div>
        </>
      )}
      {status === "ready" && tabs.length === 0 && asset.kind !== "image" && (
        <p className="p-4 text-sm font-semibold text-muted">This file has no sound, so there’s nothing to transcribe.</p>
      )}
    </article>
  );
}

/** A project's media, newest first, in pages. Polls while anything is still processing. */
export function useProjectMedia(project: Project) {
  const orgID = project.organisation_id;
  return useInfiniteQuery({
    queryKey: ["project", project.id, "media"],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/media", {
        params: { path: { orgID, projectID: project.id }, query: { cursor: pageParam } },
      });
      if (!data) raise(error, response.status, "Could not load media.");
      return data;
    },
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    // Poll while anything is still processing or being analysed. (Live updates over a
    // WebSocket can replace this later.)
    refetchInterval: (q) => {
      const assets = q.state.data?.pages.flatMap((p) => p.data) ?? [];
      if (assets.some((a) => a.status === "uploaded" && (a.processing.status === "pending" || a.processing.status === "processing"))) return 2000;
      if (assets.some(importing)) return 3000;
      // Framing analysis takes minutes: check less often.
      return assets.some(framingBusy) ? 5000 : false;
    },
  });
}
