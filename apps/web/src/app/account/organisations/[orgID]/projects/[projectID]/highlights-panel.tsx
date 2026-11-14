"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, NotFound, raise, useAIModelSettings, type Highlights, type MediaAsset, type Project } from "@/lib/queries";
import { ClipComposer } from "./clips";
import { stamp, useTranscript } from "./transcript-panel";

/** Clip length choices, roughly matching where the clips are headed. */
const lengths = [
  { min: 15, max: 30, label: "15–30s", hint: "Punchy" },
  { min: 30, max: 60, label: "30–60s", hint: "Most short-form" },
  { min: 60, max: 90, label: "60–90s", hint: "Room for a story" },
  { min: 90, max: 180, label: "1½–3 min", hint: "Longer cuts" },
];
const defaultLength = 1;

function seconds(ms: number) {
  const s = Math.round(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
}

/**
 * Candidate clips found in a file's transcript. Editors describe what they
 * want and pick a clip length; everyone with access can review the clips
 * and play each one against the source.
 */
export function HighlightsPanel({
  asset,
  project,
  canEdit,
  canRender,
  url,
  onPlay,
}: {
  asset: MediaAsset;
  project: Project;
  canEdit: boolean;
  /** Whether the viewer may render clips; needs a video and its URL. */
  canRender: boolean;
  url: string | null;
  onPlay: (startMS: number, endMS: number) => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const path = { orgID: project.organisation_id, projectID: project.id, assetID: asset.id };
  const key = ["media", asset.id, "highlights"];
  const transcript = useTranscript(project, asset);
  const hasTranscript = !!transcript.data;
  const transcriptReady = transcript.data?.status === "ready" && transcript.data.segments.length > 0;

  const highlights = useQuery({
    queryKey: key,
    enabled: hasTranscript,
    queryFn: async (): Promise<Highlights | null> => {
      const { data, error, response } = await client().GET(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/highlights",
        { params: { path } },
      );
      if (response.status === 404) return null;
      if (!data) raise(error, response.status, "Could not load highlights.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && n < 2,
    refetchInterval: (q) => (q.state.data && (q.state.data.status === "queued" || q.state.data.status === "running") ? 3000 : false),
  });
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "highlights")?.model;

  const [editing, setEditing] = useState(false);
  const [composing, setComposing] = useState<number | null>(null);
  const renderable = canRender && asset.kind === "video" && !!url;
  const [objective, setObjective] = useState("");
  const [length, setLength] = useState(defaultLength);

  const request = useMutation({
    mutationFn: async () => {
      const { min, max } = lengths[length];
      const { data, error, response } = await client().POST(
        "/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/highlights",
        { params: { path }, body: { objective: objective.trim(), min_seconds: min, max_seconds: max } },
      );
      if (!data) raise(error, response.status, "Could not start finding highlights.");
      return data;
    },
    onSuccess: (h) => {
      qc.setQueryData(key, h);
      setEditing(false);
    },
  });

  if (!hasTranscript || highlights.isPending) return null;
  const h = highlights.data;
  const composingClip = renderable && composing !== null ? h?.clips[composing] : undefined;
  if (h === null && !(canEdit && transcriptReady)) return null;

  const busy = h && (h.status === "queued" || h.status === "running");
  const showForm = canEdit && transcriptReady && !busy && (h === null || h?.status === "failed" || editing);

  function edit() {
    if (h) {
      setObjective(h.objective);
      const i = lengths.findIndex((l) => l.min === h.min_seconds && l.max === h.max_seconds);
      setLength(i >= 0 ? i : defaultLength);
    }
    setEditing(true);
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const where = model ? `${model.provider === "gemini" ? "Google " : ""}${model.label}` : "your organisation’s AI provider";
    if (window.confirm(`To find highlights, the transcript of “${asset.filename}” will be sent to ${where}. Continue?`)) {
      request.mutate();
    }
  }

  return (
    <section aria-labelledby={`${id}-h`} className="mt-3 border-2 border-ink bg-white">
      <div className="flex min-h-11 items-center gap-2 px-3 pt-2">
        <h3 id={`${id}-h`} className="font-extrabold">Highlights</h3>
        {h?.status === "ready" && (
          <span className="text-sm font-semibold text-ink-soft">
            · {h.clips.length} {h.clips.length === 1 ? "clip" : "clips"}
          </span>
        )}
      </div>

      {highlights.isError && <div className="px-3 pb-3"><Banner>{highlights.error.message}</Banner></div>}

      {busy && (
        <p className="flex items-center gap-2 px-3 pb-3 text-sm font-semibold" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-ink" />
          Reading the transcript with {h.model_label}…
        </p>
      )}

      {h?.status === "failed" && (
        <p className="mx-3 mb-3 border-2 border-ink bg-paper-deep p-2 text-sm font-semibold" role="status">
          {h.error ?? "Finding highlights failed."}
        </p>
      )}

      {h?.status === "ready" && !editing && (
        <>
          {composingClip && url ? (
            // The composer replaces the list: it needs the panel's full width.
            <div className="px-3 pb-3">
              <p className="mb-2 text-sm font-semibold text-ink-soft">
                Making a clip from highlight {composing! + 1}: “{composingClip.title}”
              </p>
              <ClipComposer
                asset={asset}
                project={project}
                url={url}
                initial={{ name: composingClip.title, startMS: composingClip.start_ms, endMS: composingClip.end_ms }}
                onPreview={onPlay}
                onClose={() => setComposing(null)}
              />
            </div>
          ) : h.clips.length === 0 ? (
            <p className="px-3 pb-2 text-sm font-semibold text-ink-soft">
              No moments fit this brief. Try a different objective or clip length.
            </p>
          ) : (
            <ol className="max-h-[28rem] space-y-2 overflow-y-auto px-3 pb-2">
              {h.clips.map((c, i) => (
                <li key={`${c.start_ms}-${i}`} className="border-2 border-ink/15 bg-paper p-2.5">
                  <div className="flex items-start gap-2.5">
                    <span
                      aria-label={`Rank ${i + 1}`}
                      className="grid size-7 shrink-0 place-items-center rounded-full border-2 border-ink bg-ink text-xs font-extrabold text-white"
                    >
                      {i + 1}
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="font-extrabold leading-snug break-words">{c.title}</p>
                      <p className="mt-0.5 font-mono text-xs font-bold text-ink-soft">
                        {stamp(c.start_ms)}–{stamp(c.end_ms)} · {seconds(c.end_ms - c.start_ms)}
                      </p>
                      <p className="mt-1 text-sm font-medium break-words">{c.reason}</p>
                      <div className="mt-2 flex flex-wrap gap-2">
                        <button
                          type="button"
                          onClick={() => onPlay(c.start_ms, c.end_ms)}
                          className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold"
                          aria-label={`Play clip ${i + 1}: ${c.title}`}
                        >
                          ▶ Play clip
                        </button>
                        {renderable && (
                          <button
                            type="button"
                            onClick={() => setComposing(i)}
                            className="btn-pop min-h-11 bg-ink px-3 text-sm font-extrabold text-white"
                            aria-label={`Make clip ${i + 1}: ${c.title}`}
                          >
                            Make clip
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                </li>
              ))}
            </ol>
          )}
          <div className={`flex flex-wrap items-center gap-2 border-t-2 border-ink/10 px-3 py-2 ${composingClip ? "hidden" : ""}`}>
            {canEdit && transcriptReady && (
              <button type="button" onClick={edit} className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold">
                Find again
              </button>
            )}
            <span className="min-w-0 text-xs font-semibold break-words text-ink-soft">
              {h.objective ? <>Asked for “{h.objective}” · </> : null}
              {lengths.find((l) => l.min === h.min_seconds && l.max === h.max_seconds)?.label ?? `${h.min_seconds}–${h.max_seconds}s`}
              {" "}· Suggested by {h.model_label}. Check each clip before you use it.
            </span>
          </div>
        </>
      )}

      {showForm && (
        <form onSubmit={submit} className="space-y-3 px-3 pb-3">
          {h === null && (
            <p className="text-sm font-medium text-ink-soft">
              Find moments in the transcript that work as clips on their own. Clips start and end on whole lines of speech.
            </p>
          )}
          {request.isError && <Banner>{request.error.message}</Banner>}
          <div>
            <label htmlFor={`${id}-objective`} className="text-sm font-extrabold">
              What are you looking for? <span className="font-semibold text-ink-soft">(optional)</span>
            </label>
            <textarea
              id={`${id}-objective`}
              value={objective}
              onChange={(e) => setObjective(e.target.value)}
              maxLength={500}
              rows={2}
              placeholder="e.g. The funniest exchange between the hosts, for TikTok"
              className="mt-1 block w-full border-2 border-ink bg-paper px-3 py-2 text-base font-medium"
            />
          </div>
          <fieldset>
            <legend className="text-sm font-extrabold">Clip length</legend>
            <div className="mt-1 grid grid-cols-2 gap-2">
              {lengths.map((l, i) => (
                <label
                  key={l.label}
                  className={`flex min-h-11 min-w-0 cursor-pointer flex-col justify-center border-2 border-ink px-2.5 py-1.5 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                    length === i ? "bg-ink text-white" : "bg-white"
                  }`}
                >
                  <input
                    type="radio"
                    name={`${id}-length`}
                    checked={length === i}
                    onChange={() => setLength(i)}
                    className="sr-only"
                  />
                  <span className="text-sm font-extrabold">{l.label}</span>
                  <span className="text-xs font-semibold text-ink-soft">{l.hint}</span>
                </label>
              ))}
            </div>
          </fieldset>
          <div className="flex flex-wrap gap-2">
            <button
              type="submit"
              disabled={request.isPending}
              className="btn-pop min-h-11 bg-ink px-3 text-sm font-extrabold text-white disabled:opacity-60"
            >
              {request.isPending ? "Starting…" : h?.status === "failed" ? "Try again" : "Find highlights"}
            </button>
            {editing && (
              <button
                type="button"
                onClick={() => setEditing(false)}
                className="btn-pop min-h-11 bg-white px-3 text-sm font-extrabold"
              >
                Cancel
              </button>
            )}
          </div>
        </form>
      )}

      {h && !busy && canEdit && !transcriptReady && (
        <p className="px-3 pb-3 text-xs font-semibold text-ink-soft">Finish the transcript to find highlights again.</p>
      )}
    </section>
  );
}
