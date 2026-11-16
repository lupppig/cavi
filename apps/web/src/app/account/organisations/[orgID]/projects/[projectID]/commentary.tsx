"use client";

import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, useAIModelSettings, type Project, type Voiceover, type VoiceStyle } from "@/lib/queries";
import { sendFile } from "@/lib/upload";
import { stampTenths } from "./clips";
import { busy, useVoices } from "./voiceover-picker";

/** Lines a clip can carry; the API allows 8, plans suggest up to 6. */
export const MAX_LINES = 8;
const MAX_LINE = 200;
/** The clip's own sound under commentary. */
export const COMMENTARY_DUCK = 0.2;

export type CommentaryLine = { segment: number; text: string };

/** A line being edited: its text, where it plays, and its voiceover once spoken. */
export type LineDraft = { key: number; segment: number; text: string; voiceoverID: string | null; spokenText: string | null };

let seq = 0;
export const draftLines = (lines: CommentaryLine[]): LineDraft[] =>
  lines.map((l) => ({ key: ++seq, segment: l.segment, text: l.text, voiceoverID: null, spokenText: null }));

/** Each line's voiceover, polled until spoken. */
export function useLineVoiceovers(project: Project, lines: LineDraft[]) {
  const qc = useQueryClient();
  return useQueries({
    queries: lines.map((l) => ({
      queryKey: ["project", project.id, "voiceovers", l.voiceoverID],
      enabled: !!l.voiceoverID,
      queryFn: async (): Promise<Voiceover> => {
        const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/{voiceoverID}", {
          params: { path: { orgID: project.organisation_id, projectID: project.id, voiceoverID: l.voiceoverID! } },
        });
        if (!data) raise(error, response.status, "Could not load a commentary line.");
        if (!busy(data)) qc.invalidateQueries({ queryKey: ["project", project.id, "voiceovers"], exact: true });
        return data;
      },
      refetchInterval: (q: { state: { data?: Voiceover } }) => (q.state.data && busy(q.state.data) ? 2500 : false),
    })),
  });
}

/** The voice and style the organisation's voiceover model offers for narration. */
export function useNarrator(orgID: string) {
  const voices = useVoices();
  const settings = useAIModelSettings(orgID);
  const model = settings.data?.find((s) => s.task === "voiceover")?.model;
  const offered = (voices.data ?? []).filter((v) => !model || v.provider === model.provider);
  const styles: VoiceStyle[] = offered[0]?.styles ?? ["natural"];
  return { model, offered, styles };
}

/** Starts speaking text as a voiceover. */
export async function speak(project: Project, text: string, voice: string, style: VoiceStyle): Promise<Voiceover> {
  const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers", {
    params: { path: { orgID: project.organisation_id, projectID: project.id } },
    body: { script: text, style, ...(voice ? { voice } : {}) },
  });
  if (!data) raise(error, response.status, "Could not start speaking a commentary line.");
  return data;
}

/** Waits for a voiceover to be spoken (or fail), up to a few minutes. */
export async function waitFor(project: Project, id: string, signal?: AbortSignal): Promise<Voiceover> {
  const until = Date.now() + 4 * 60_000;
  for (;;) {
    const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/{voiceoverID}", {
      params: { path: { orgID: project.organisation_id, projectID: project.id, voiceoverID: id } },
    });
    if (!data) raise(error, response.status, "Could not check a commentary line.");
    if (!busy(data)) return data;
    if (Date.now() > until || signal?.aborted) throw new Error("A commentary line took too long to speak. Try again.");
    await new Promise((r) => setTimeout(r, 2000));
  }
}

/** Why the commentary can't be rendered yet; null when it can. */
export function commentaryProblem(lines: LineDraft[], voiced: (Voiceover | undefined)[]): string | null {
  for (const [i, l] of lines.entries()) {
    const v = voiced[i];
    if (!l.text.trim()) return `Commentary line ${i + 1} is empty. Write it or remove it.`;
    if (!l.voiceoverID || l.spokenText !== l.text.trim()) return "Voice the commentary (or remove lines) before rendering.";
    if (!v || busy(v)) return "Wait for the commentary to finish speaking.";
    if (v.status !== "ready") return `Commentary line ${i + 1} couldn’t be spoken. Voice it again.`;
  }
  return null;
}

/**
 * Narration through the clip, in the channel's own words: each line plays
 * at the start of a segment (or at the end), with the clip's sound lowered.
 */
export function CommentaryEditor({
  project,
  segments,
  value,
  onChange,
  canMake,
}: {
  project: Project;
  segments: number;
  value: LineDraft[];
  onChange: (v: LineDraft[]) => void;
  canMake: boolean;
}) {
  const id = useId();
  const voiced = useLineVoiceovers(project, value).map((q) => q.data);
  const { model, offered, styles } = useNarrator(project.organisation_id);
  const [voice, setVoice] = useState("");
  const [style, setStyle] = useState<VoiceStyle>("natural");
  const set = (key: number, patch: Partial<LineDraft>) => onChange(value.map((l) => (l.key === key ? { ...l, ...patch } : l)));
  const pending = value.filter((l) => l.text.trim() && (!l.voiceoverID || l.spokenText !== l.text.trim()));

  const voiceAll = useMutation({
    mutationFn: async () => {
      let next = value;
      for (const l of pending) {
        const v = await speak(project, l.text.trim(), voice, style);
        next = next.map((x) => (x.key === l.key ? { ...x, voiceoverID: v.id, spokenText: l.text.trim() } : x));
        onChange(next);
      }
    },
  });

  function start() {
    const where = model ? model.label : "your organisation’s voice model";
    const chars = pending.reduce((n, l) => n + l.text.trim().length, 0);
    if (window.confirm(`${pending.length} line${pending.length === 1 ? "" : "s"} (${chars} characters) will be sent to ${where} to be spoken. Continue?`)) {
      voiceAll.mutate();
    }
  }

  const anchors = Array.from({ length: segments + 1 }, (_, i) => i + 1);
  return (
    <fieldset className="min-w-0 space-y-2">
      <legend className="text-sm font-semibold">
        Commentary <span className="font-semibold text-muted">· your narration through the clip</span>
      </legend>
      <p className="text-xs font-semibold text-muted">
        Short lines in your own words: set up the moment, explain, give your take, end with a takeaway. The clip’s sound dips while each
        plays. Commentary is what makes a clip of someone else’s video your own work, and recording it in your own voice is the strongest
        sign of that.
      </p>
      {voiceAll.isError && <Banner>{voiceAll.error.message}</Banner>}
      <ol className="space-y-2">
        {value.map((l, i) => {
          const v = voiced[i];
          const stale = !!l.voiceoverID && l.spokenText !== l.text.trim();
          return (
            <li key={l.key} className="space-y-1.5 rounded-xl border border-line bg-surface-2 p-2.5">
              <div className="flex flex-wrap items-center gap-2">
                <label className="text-xs font-semibold" htmlFor={`${id}-a${l.key}`}>
                  Line {i + 1} plays at
                </label>
                <select
                  id={`${id}-a${l.key}`}
                  value={l.segment}
                  onChange={(e) => set(l.key, { segment: Number(e.target.value) })}
                  className="min-h-11 max-w-full min-w-0 border border-line bg-white px-2 text-sm font-semibold"
                >
                  {anchors.map((a) => (
                    <option key={a} value={a}>
                      {a <= segments ? `the start of segment ${a}` : "the end"}
                    </option>
                  ))}
                </select>
                <button type="button" onClick={() => onChange(value.filter((x) => x.key !== l.key))} className="btn btn-ghost ml-auto min-h-11 px-3 text-sm">
                  Remove
                </button>
              </div>
              <textarea
                value={l.text}
                onChange={(e) => set(l.key, { text: e.target.value })}
                maxLength={MAX_LINE}
                rows={2}
                aria-label={`Commentary line ${i + 1}`}
                className="block w-full border border-line bg-white px-3 py-2 text-base font-medium"
              />
              {canMake && (
                <RecordButton
                  project={project}
                  text={l.text}
                  busy={voiceAll.isPending}
                  onRecorded={(rv) => set(l.key, { voiceoverID: rv.id, spokenText: l.text.trim() })}
                />
              )}
              <p className="text-xs font-semibold text-muted" role="status">
                {!l.voiceoverID
                  ? "Not voiced yet"
                  : stale
                    ? "Changed since it was voiced: voice it again"
                    : !v || busy(v)
                      ? "Speaking…"
                      : v.status === "ready"
                        ? `${v.provider === "recording" ? "Your voice" : "Voiced"} · ${stampTenths(v.duration_ms ?? 0)}`
                        : (v.error ?? "Couldn’t be spoken")}
              </p>
            </li>
          );
        })}
      </ol>
      <div className="flex flex-wrap items-end gap-2">
        {value.length < MAX_LINES && (
          <button
            type="button"
            onClick={() => onChange([...value, { key: ++seq, segment: Math.min(segments + 1, (value.at(-1)?.segment ?? 0) + 1), text: "", voiceoverID: null, spokenText: null }])}
            className="btn btn-secondary min-h-11 px-3 text-sm"
          >
            + Add a line
          </button>
        )}
        {canMake && pending.length > 0 && (
          <>
            {offered.length > 1 && (
              <label className="text-xs font-semibold">
                Voice
                <select value={voice} onChange={(e) => setVoice(e.target.value)} className="mt-1 block min-h-11 border border-line bg-white px-2 text-sm">
                  <option value="">Default</option>
                  {offered.map((v) => (
                    <option key={v.id} value={v.id}>
                      {v.label}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {styles.length > 1 && (
              <label className="text-xs font-semibold">
                Delivery
                <select value={style} onChange={(e) => setStyle(e.target.value as VoiceStyle)} className="mt-1 block min-h-11 border border-line bg-white px-2 text-sm">
                  {styles.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <button type="button" onClick={start} disabled={voiceAll.isPending} className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60">
              {voiceAll.isPending ? "Starting…" : `Voice ${pending.length === value.length ? "all lines" : `${pending.length} line${pending.length === 1 ? "" : "s"}`}`}
            </button>
          </>
        )}
      </div>
    </fieldset>
  );
}

/**
 * A guide to how clearly a clip is the channel's own work, from what
 * YouTube's reused-content review looks for. It can't promise approval:
 * YouTube decides, looking at the whole channel.
 */
export function OriginalityCheck({
  totalMS,
  narratedMS,
  lines,
  captions,
  hook,
  tightened,
  ownVoice = 0,
}: {
  totalMS: number;
  narratedMS: number;
  lines: number;
  captions: boolean;
  hook: boolean;
  tightened: boolean;
  /** Lines recorded in the creator's own voice. */
  ownVoice?: number;
}) {
  const share = totalMS > 0 ? Math.min(1, narratedMS / totalMS) : 0;
  const checks = [
    { ok: lines >= 3, label: lines >= 3 ? `${lines} commentary lines` : "At least 3 commentary lines through the clip" },
    { ok: share >= 0.2, label: `Your narration covers ${Math.round(share * 100)}% of the clip${share >= 0.2 ? "" : " (aim for 20% or more)"}` },
    { ok: captions, label: captions ? "Captions" : "Add captions" },
    { ok: hook, label: hook ? "An opening hook" : "Add a hook" },
    { ok: tightened, label: tightened ? "Pauses cut" : "Cut the pauses" },
    {
      ok: lines > 0 && ownVoice >= lines,
      label:
        lines > 0 && ownVoice >= lines
          ? "In your own voice"
          : "Record the lines in your own voice (stronger than an AI voice)",
    },
  ];
  const strong = checks[0].ok && checks[1].ok;
  const passed = checks.filter((c) => c.ok).length;
  return (
    <section aria-label="Original content check" className={`rounded-xl border p-2.5 ${strong ? "border-line bg-surface-2" : "border-accent-line bg-accent-soft"}`}>
      <p className="text-sm font-semibold">
        {strong ? "Clearly your own work" : passed >= 3 ? "Thin: mostly the original footage" : "At risk: little of this is your own"}
      </p>
      <ul className="mt-1 space-y-0.5 text-xs font-semibold">
        {checks.map((c) => (
          <li key={c.label}>
            <span aria-hidden="true">{c.ok ? "✓" : "○"}</span> {c.label}
          </li>
        ))}
      </ul>
      <p className="mt-1 text-xs font-medium text-muted">
        A guide to YouTube’s reused-content rules, not a promise: YouTube decides, and looks at your whole channel.
      </p>
    </section>
  );
}

/** The longest take: the API refuses recordings over a minute. */
const MAX_TAKE_MS = 60_000;

/** The best format this browser records audio in. */
function recordingType() {
  for (const t of ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/ogg;codecs=opus"]) {
    if (typeof MediaRecorder !== "undefined" && MediaRecorder.isTypeSupported(t)) return t;
  }
  return "";
}

/** Uploads a take as a voiceover of text in the creator's own voice. */
async function uploadTake(project: Project, text: string, take: Blob): Promise<Voiceover> {
  const path = { orgID: project.organisation_id, projectID: project.id };
  const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/recordings", {
    params: { path },
    body: { script: text, content_type: take.type || "audio/webm", size_bytes: take.size },
  });
  if (!data) raise(error, response.status, "Could not upload the recording.");
  await sendFile(data.upload, take, () => {}, new AbortController().signal);
  const done = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/{voiceoverID}/complete", {
    params: { path: { ...path, voiceoverID: data.voiceover.id } },
  });
  if (!done.data) raise(done.error, done.response.status, "Could not finish uploading the recording.");
  return done.data;
}

/**
 * Records one commentary line with the microphone, in the creator's own
 * voice: start, read the line, stop. The take is uploaded and becomes the
 * line's voiceover once processed (silence trimmed, words timed).
 */
export function RecordButton({
  project,
  text,
  busy: disabled,
  onRecorded,
}: {
  project: Project;
  text: string;
  busy?: boolean;
  onRecorded: (v: Voiceover) => void;
}) {
  const [state, setState] = useState<"idle" | "recording" | "uploading">("idle");
  const [elapsed, setElapsed] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const rec = useRef<MediaRecorder | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearInterval(timer.current);
      rec.current?.stream.getTracks().forEach((t) => t.stop());
    },
    [],
  );

  async function start() {
    setError(null);
    if (typeof MediaRecorder === "undefined" || !navigator.mediaDevices?.getUserMedia) {
      setError("This browser can’t record audio. Try Chrome, Edge, Firefox or Safari.");
      return;
    }
    let stream: MediaStream;
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true } });
    } catch {
      setError("Allow the microphone to record. Check your browser’s site permissions.");
      return;
    }
    const type = recordingType();
    const r = new MediaRecorder(stream, type ? { mimeType: type } : undefined);
    const chunks: Blob[] = [];
    r.ondataavailable = (e) => e.data.size > 0 && chunks.push(e.data);
    r.onstop = async () => {
      stream.getTracks().forEach((t) => t.stop());
      if (timer.current) clearInterval(timer.current);
      setState("uploading");
      try {
        const v = await uploadTake(project, text.trim(), new Blob(chunks, { type: r.mimeType || type || "audio/webm" }));
        onRecorded(v);
        setState("idle");
      } catch (e) {
        setError(e instanceof Error ? e.message : "Could not upload the recording.");
        setState("idle");
      }
    };
    rec.current = r;
    r.start();
    const began = Date.now();
    setElapsed(0);
    setState("recording");
    timer.current = setInterval(() => {
      const ms = Date.now() - began;
      setElapsed(ms);
      if (ms >= MAX_TAKE_MS && r.state === "recording") r.stop();
    }, 200);
  }

  return (
    <span className="inline-flex flex-wrap items-center gap-2">
      {state === "recording" ? (
        <button type="button" onClick={() => rec.current?.stop()} className="btn btn-primary min-h-11 px-3 text-sm" aria-live="polite">
          <span aria-hidden="true" className="mr-1.5 inline-block size-2 animate-pulse rounded-full bg-surface" />
          Stop · {stampTenths(elapsed)}
        </button>
      ) : (
        <button
          type="button"
          onClick={start}
          disabled={disabled || state === "uploading" || !text.trim()}
          className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
        >
          {state === "uploading" ? "Uploading…" : "● Record in your voice"}
        </button>
      )}
      {error && <span className="text-xs font-semibold text-ink">{error}</span>}
    </span>
  );
}
