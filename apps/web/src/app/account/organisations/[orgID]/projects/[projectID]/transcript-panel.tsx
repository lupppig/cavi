"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, NotFound, raise, useAIModelSettings, type MediaAsset, type Project, type Transcript } from "@/lib/queries";

function stamp(ms: number) {
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

export function TranscriptPanel({
  asset,
  project,
  canTranscribe,
  onSeek,
}: {
  asset: MediaAsset;
  project: Project;
  canTranscribe: boolean;
  onSeek: (ms: number) => void;
}) {
  const qc = useQueryClient();
  const [copied, setCopied] = useState(false);
  const path = { orgID: project.organisation_id, projectID: project.id, assetID: asset.id };
  const key = ["media", asset.id, "transcript"];

  const transcript = useQuery({
    queryKey: key,
    queryFn: async (): Promise<Transcript | null> => {
      const { data, error, response } = await client().GET(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/transcript",
        { params: { path } },
      );
      if (response.status === 404) return null;
      if (!data) raise(error, response.status, "Could not load the transcript.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && n < 2,
    refetchInterval: (q) => (q.state.data && (q.state.data.status === "queued" || q.state.data.status === "running") ? 3000 : false),
  });
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "transcription")?.model;

  const request = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/transcript",
        { params: { path } },
      );
      if (!data) raise(error, response.status, "Could not start transcription.");
      return data;
    },
    onSuccess: (t) => qc.setQueryData(key, t),
  });

  function start() {
    const where = model ? `${model.provider === "gemini" ? "Google " : ""}${model.label}` : "your organisation’s AI provider";
    if (window.confirm(`To transcribe “${asset.filename}”, its audio will be sent to ${where}. Continue?`)) request.mutate();
  }

  const t = transcript.data;
  const busy = t && (t.status === "queued" || t.status === "running");
  const startButton = (label: string) =>
    canTranscribe && (
      <button
        type="button"
        onClick={start}
        disabled={request.isPending}
        className="btn-pop min-h-11 rounded-xl bg-cobalt px-3 text-sm font-extrabold text-white disabled:opacity-60"
      >
        {request.isPending ? "Starting…" : label}
      </button>
    );

  if (transcript.isPending) return null;
  return (
    <div className="mt-3">
      {transcript.isError && <Banner>{transcript.error.message}</Banner>}
      {request.isError && <div className="mb-2"><Banner>{request.error.message}</Banner></div>}
      {t === null && startButton("Transcribe")}
      {busy && (
        <p className="flex items-center gap-2 text-sm font-semibold" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-cobalt" />
          Transcribing with {t.model_label}…
        </p>
      )}
      {t?.status === "failed" && (
        <div className="rounded-xl border-2 border-ink bg-tomato/15 p-3 text-sm font-semibold" role="status">
          <p>{t.error ?? "Transcription failed."}</p>
          <div className="mt-2">{startButton("Try again")}</div>
        </div>
      )}
      {t?.status === "ready" && (
        <details className="group rounded-xl border-2 border-ink bg-white">
          <summary className="flex min-h-11 cursor-pointer list-none items-center gap-2 px-3 font-extrabold">
            <span aria-hidden="true" className="transition-transform group-open:rotate-90">▸</span>
            Transcript
            <span className="text-sm font-semibold text-ink-soft">
              · {t.segments.length} {t.segments.length === 1 ? "line" : "lines"}
              {t.language ? ` · ${t.language}` : ""}
            </span>
          </summary>
          {t.segments.length === 0 ? (
            <p className="px-3 pb-3 text-sm font-semibold text-ink-soft">No speech was found.</p>
          ) : (
            <>
              <ol className="max-h-80 space-y-2 overflow-y-auto px-3 pb-3">
                {t.segments.map((s, i) => (
                  <li key={i} className="text-sm">
                    <button
                      type="button"
                      onClick={() => onSeek(s.start_ms)}
                      className="mr-2 inline-flex min-h-8 items-center rounded-md bg-paper px-1.5 font-mono text-xs font-bold underline-offset-2 hover:underline"
                      aria-label={`Play from ${stamp(s.start_ms)}`}
                    >
                      {stamp(s.start_ms)}
                    </button>
                    <span className="font-extrabold">{s.speaker}:</span> <span className="font-medium">{s.text}</span>
                  </li>
                ))}
              </ol>
              <div className="flex flex-wrap items-center gap-2 border-t-2 border-ink/10 px-3 py-2">
                <button
                  type="button"
                  onClick={async () => {
                    const text = t.segments.map((s) => `[${stamp(s.start_ms)}] ${s.speaker}: ${s.text}`).join("\n");
                    try {
                      await navigator.clipboard.writeText(text);
                      setCopied(true);
                    } catch {
                      setCopied(false);
                    }
                  }}
                  className="btn-pop min-h-11 rounded-xl bg-white px-3 text-sm font-extrabold"
                >
                  {copied ? "Copied!" : "Copy text"}
                </button>
                {startButton("Transcribe again")}
                <span className="text-xs font-semibold text-ink-soft">Made with {t.model_label}. AI transcripts can contain mistakes.</span>
              </div>
            </>
          )}
        </details>
      )}
    </div>
  );
}
