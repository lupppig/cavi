"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, NotFound, raise, useAIModelSettings, type MediaAsset, type Project, type Transcript } from "@/lib/queries";

export function stamp(ms: number) {
  const total = Math.floor(ms / 1000);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = String(total % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

/** A file's transcript, or null if it has never been transcribed. Polls while it's in progress. */
export function useTranscript(project: Project, asset: MediaAsset) {
  const path = { orgID: project.organisation_id, projectID: project.id, assetID: asset.id };
  return useQuery({
    queryKey: ["media", asset.id, "transcript"],
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
}

/**
 * Starts transcription after asking for consent: the confirmation names
 * the provider and model the audio goes to.
 */
export function useRequestTranscript(project: Project, asset: MediaAsset) {
  const qc = useQueryClient();
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "transcription")?.model;
  const request = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/transcript",
        { params: { path: { orgID: project.organisation_id, projectID: project.id, assetID: asset.id } } },
      );
      if (!data) raise(error, response.status, "Could not start transcription.");
      return data;
    },
    onSuccess: (t) => qc.setQueryData(["media", asset.id, "transcript"], t),
  });
  const start = () => {
    const where = model ? `${model.provider === "gemini" ? "Google " : ""}${model.label}` : "your organisation’s AI provider";
    if (window.confirm(`To transcribe “${asset.filename}”, its audio will be sent to ${where}. Continue?`)) request.mutate();
  };
  return { request, start };
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
  const [copied, setCopied] = useState(false);

  const transcript = useTranscript(project, asset);
  const { request, start } = useRequestTranscript(project, asset);

  const t = transcript.data;
  const busy = t && (t.status === "queued" || t.status === "running");
  const startButton = (label: string) =>
    canTranscribe && (
      <button
        type="button"
        onClick={start}
        disabled={request.isPending}
        className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60"
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
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
          Transcribing with {t.model_label}…
        </p>
      )}
      {t?.status === "failed" && (
        <div className="border border-line bg-surface-2 p-3 text-sm font-semibold" role="status">
          <p>{t.error ?? "Transcription failed."}</p>
          <div className="mt-2">{startButton("Try again")}</div>
        </div>
      )}
      {t?.status === "ready" && (
        <div className="card">
          <div className="flex min-h-11 items-center gap-2 px-3 font-semibold">
            Transcript
            <span className="text-sm font-semibold text-muted">
              · {t.segments.length} {t.segments.length === 1 ? "line" : "lines"}
              {t.language ? ` · ${t.language}` : ""}
            </span>
          </div>
          {t.segments.length === 0 ? (
            <p className="px-3 pb-3 text-sm font-semibold text-muted">No speech was found.</p>
          ) : (
            <>
              {t.speakers.length > 0 && <Speakers transcript={t} project={project} asset={asset} canEdit={canTranscribe} />}
              <ol className="max-h-[32rem] space-y-1 overflow-y-auto px-3 pb-3">
                {t.segments.map((s, i) => (
                  <li key={i} className="text-sm">
                    <button
                      type="button"
                      onClick={() => onSeek(s.start_ms)}
                      className="mr-2 inline-flex min-h-11 min-w-11 items-center justify-center bg-surface px-1.5 font-mono text-xs font-bold underline-offset-2 hover:underline"
                      aria-label={`Play from ${stamp(s.start_ms)}`}
                    >
                      {stamp(s.start_ms)}
                    </button>
                    <span className="font-semibold">{s.speaker}:</span> <span className="font-medium">{s.text}</span>
                  </li>
                ))}
              </ol>
              <div className="flex flex-wrap items-center gap-2 border-t border-line px-3 py-2">
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
                  className="btn btn-secondary min-h-11 px-3 text-sm"
                >
                  {copied ? "Copied!" : "Copy text"}
                </button>
                {startButton("Transcribe again")}
                <span className="text-xs font-semibold text-muted">Made with {t.model_label}. AI transcripts can contain mistakes.</span>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

const voiceOptions = [
  { value: "woman", label: "Woman’s voice" },
  { value: "man", label: "Man’s voice" },
  { value: "unclear", label: "Unclear" },
] as const;

/**
 * Who speaks, and how each voice sounds. CAVI suggests this from the audio
 * alone; editors can correct it. It describes a voice, not who someone is.
 */
function Speakers({ transcript, project, asset, canEdit }: { transcript: Transcript; project: Project; asset: MediaAsset; canEdit: boolean }) {
  const qc = useQueryClient();
  const set = useMutation({
    mutationFn: async ({ label, voice }: { label: string; voice: "woman" | "man" | "unclear" }) => {
      const { data, error, response } = await client().PATCH(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/transcript/speakers/{label}",
        { params: { path: { orgID: project.organisation_id, projectID: project.id, assetID: asset.id, label } }, body: { voice } },
      );
      if (!data) raise(error, response.status, "Could not change the voice.");
      return data;
    },
    onSuccess: (t) => qc.setQueryData(["media", asset.id, "transcript"], t),
  });
  return (
    <div className="border-b border-line px-3 pb-3">
      <p className="text-xs font-semibold uppercase text-muted">Speakers</p>
      <ul className="mt-1 space-y-1">
        {transcript.speakers.map((sp) => {
          const label = voiceOptions.find((v) => v.value === sp.voice)?.label ?? sp.voice;
          return (
            <li key={sp.label} className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-semibold">{sp.label}</span>
              {canEdit ? (
                <select
                  value={sp.voice}
                  disabled={set.isPending}
                  onChange={(e) => set.mutate({ label: sp.label, voice: e.target.value as "woman" | "man" | "unclear" })}
                  aria-label={`How ${sp.label}’s voice sounds`}
                  className="min-h-11 border border-line bg-white px-2 text-sm font-bold disabled:opacity-60"
                >
                  {voiceOptions.map((v) => (
                    <option key={v.value} value={v.value}>
                      {v.label}
                    </option>
                  ))}
                </select>
              ) : (
                <span className="font-semibold">{label}</span>
              )}
              <span className="text-xs font-semibold text-muted">{sp.voice_set_by === "ai" ? "suggested by AI" : "set by a person"}</span>
            </li>
          );
        })}
      </ul>
      {set.isError && <p className="mt-1 text-sm font-bold">{set.error.message}</p>}
    </div>
  );
}
