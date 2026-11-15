"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type CaptionOptions, type Composition, type MontageSegment, type Project } from "@/lib/queries";
import { CaptionOptionsPicker, defaultCaptionOptions } from "./caption-options";
import { shapes, stampTenths } from "./clips";
import { FramingChoice } from "./framing";
import { useProjectMedia } from "./media-section";
import {
  checkSegment,
  MAX_SEGMENTS,
  montageable,
  nextKey,
  parseTenths,
  SegmentEditor,
  type SegmentDraft,
} from "./montage-segment";
import { defaultSignOff, SignOffPicker, signOffProblem, type SignOffChoice } from "./sign-off-picker";
import { useChosenVoiceover, VoiceoverPicker, voiceoverProblem, type VoiceoverChoice } from "./voiceover-picker";

const MIN_TOTAL_MS = 1000;
const MAX_TOTAL_MS = 10 * 60_000;
const DEFAULT_SEGMENT_MS = 5000;

/** What a montage starts from: a plan, or nothing. */
export type MontageInitial = {
  name: string;
  hook: string;
  script: string;
  segments: { assetID: string; startMS: number; endMS: number }[];
};

const draftOf = (s: { assetID: string; startMS: number; endMS: number }): SegmentDraft => ({
  key: nextKey(),
  assetID: s.assetID,
  start: stampTenths(s.startMS),
  end: stampTenths(s.endMS),
  focus: 0.5,
  mute: [],
  blur: [],
});

/** Sends a montage to the renderer. */
function toSegment(d: SegmentDraft): MontageSegment {
  const ms = (s: string) => parseTenths(s)!;
  return {
    source_asset_id: d.assetID,
    start_ms: ms(d.start),
    end_ms: ms(d.end),
    focus: d.focus,
    ...(d.mute.length ? { mute: d.mute.map((m) => ({ start_ms: ms(m.start), end_ms: ms(m.end) })) } : {}),
    ...(d.blur.length
      ? {
          blur: d.blur.map((b) => ({
            start_ms: ms(b.start),
            end_ms: ms(b.end),
            x: b.x / 100,
            y: b.y / 100,
            w: b.w / 100,
            h: b.h / 100,
          })),
        }
      : {}),
  };
}

/**
 * Builds a montage: segments from one or more videos played in order, each
 * with its own framing, muted stretches and blurred areas, plus an opening
 * hook, captions and an optional voiceover. Renders with composition v3.
 */
export function MontageComposer({
  project,
  initial,
  canMakeVoiceover,
  onClose,
}: {
  project: Project;
  initial?: MontageInitial;
  /** editing.manage: may have a script spoken. */
  canMakeVoiceover: boolean;
  onClose: () => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const media = useProjectMedia(project);
  const files = (media.data?.pages.flatMap((p) => p.data) ?? []).filter(montageable);
  const [name, setName] = useState(initial?.name.slice(0, 120) ?? "");
  const [aspect, setAspect] = useState<Composition["aspect"]>("9:16");
  const [hook, setHook] = useState(initial?.hook.slice(0, 120) ?? "");
  const [hookSeconds, setHookSeconds] = useState("3");
  const [segments, setSegments] = useState<SegmentDraft[]>(() =>
    initial?.segments.length ? initial.segments.map(draftOf) : [draftOf({ assetID: "", startMS: 0, endMS: DEFAULT_SEGMENT_MS })],
  );
  const [captions, setCaptions] = useState<CaptionOptions | null>(null);
  const [auto, setAuto] = useState(false);
  const [voiceover, setVoiceover] = useState<VoiceoverChoice>({ id: null, start: "0.5", duck: 0.25 });
  const chosenVoiceover = useChosenVoiceover(project, voiceover.id);
  const [signOff, setSignOff] = useState<SignOffChoice>(defaultSignOff);
  const spokenSignOff = useChosenVoiceover(project, signOff.voiceoverID);
  const [problem, setProblem] = useState<string | null>(null);

  // A new montage starts on the first video once the files load.
  const withDefault = (d: SegmentDraft) => (d.assetID || !files[0] ? d : { ...d, assetID: files[0].id });
  const drafts = segments.map(withDefault);
  const checked = drafts.map((d) => checkSegment(d, files.find((f) => f.id === d.assetID)));
  // Where each segment starts in the montage, and the montage's length.
  const offsets: number[] = [];
  let total = 0;
  for (const c of checked) {
    offsets.push(total);
    if (!c.problem) total += c.endMS - c.startMS;
  }
  const shape = shapes.find((s) => s.aspect === aspect)!;
  // The videos the segments use, once each, for auto framing.
  const used = files.filter((f) => drafts.some((d) => d.assetID === f.id));

  const create = useMutation({
    mutationFn: async (body: { name: string; composition: Composition; captions?: CaptionOptions }) => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body,
      });
      if (!data) raise(error, response.status, "Could not start the render.");
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["project", project.id, "renders"] });
      onClose();
    },
  });

  const set = (i: number, patch: Partial<SegmentDraft>) =>
    setSegments((ss) => ss.map((s, j) => (j === i ? { ...withDefault(s), ...patch } : s)));
  const move = (i: number, by: -1 | 1) =>
    setSegments((ss) => {
      const out = [...ss];
      [out[i], out[i + by]] = [out[i + by], out[i]];
      return out;
    });

  function addSegment() {
    const last = drafts[drafts.length - 1];
    const lastEnd = last ? parseTenths(last.end) : null;
    const asset = files.find((f) => f.id === last?.assetID) ?? files[0];
    const duration = asset?.metadata?.duration_ms ?? 0;
    const from = lastEnd !== null && lastEnd < duration - 500 ? lastEnd : 0;
    setSegments((ss) => [...ss, draftOf({ assetID: asset?.id ?? "", startMS: from, endMS: Math.min(duration || DEFAULT_SEGMENT_MS, from + DEFAULT_SEGMENT_MS) })]);
  }

  function check(): string | null {
    if (!name.trim()) return "Give the montage a name.";
    const bad = checked.findIndex((c) => c.problem);
    if (bad >= 0) return `Segment ${bad + 1}: ${checked[bad].problem}`;
    if (total < MIN_TOTAL_MS) return "Montages must be at least a second long.";
    if (total > MAX_TOTAL_MS) return "Montages can be at most 10 minutes long.";
    if (hook.trim()) {
      const ms = Math.round(Number(hookSeconds) * 1000);
      if (!Number.isFinite(ms) || ms < 500 || ms > total) return `Show the hook for between 0.5 and ${(total / 1000).toFixed(1)} seconds.`;
    }
    const voProblem = voiceoverProblem(chosenVoiceover, voiceover, total);
    if (voProblem) return voProblem;
    const voEnd = voiceover.id && chosenVoiceover?.duration_ms ? Math.round(Number(voiceover.start) * 1000) + chosenVoiceover.duration_ms : null;
    return signOffProblem(signOff, spokenSignOff, total, voEnd);
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const p = check();
    setProblem(p);
    if (p) return;
    const segs = drafts.map(toSegment);
    const first = segs[0];
    create.mutate({
      name: name.trim(),
      composition: {
        version: 3,
        source_asset_id: first.source_asset_id,
        start_ms: first.start_ms,
        end_ms: first.end_ms,
        aspect,
        focus: first.focus,
        ...(auto ? { framing: "auto" as const } : {}),
        segments: segs,
        ...(hook.trim() ? { hook: { text: hook.trim(), end_ms: Math.round(Number(hookSeconds) * 1000) } } : {}),
        ...(voiceover.id
          ? { voiceover: { voiceover_id: voiceover.id, start_ms: Math.round(Number(voiceover.start) * 1000), duck: voiceover.duck } }
          : {}),
        ...(signOff.on
          ? { sign_off: { text: signOff.text.trim(), ...(signOff.voiceoverID ? { voiceover_id: signOff.voiceoverID } : {}) } }
          : {}),
      },
      ...(captions ? { captions } : {}),
    });
  }

  if (media.isPending) return <div className="h-40 animate-pulse bg-surface-2" />;
  if (media.isError) return <Banner>{media.error.message}</Banner>;

  return (
    <form onSubmit={submit} aria-label="Make a montage" className="@container space-y-4 card p-2.5 sm:p-3">
      {files.length === 0 && (
        <p className="border border-dashed border-line-strong p-2.5 text-sm font-semibold">
          Upload a video and wait for it to finish processing to make a montage.
        </p>
      )}

      <div className="grid gap-3 @[44rem]:grid-cols-2 @[44rem]:gap-6">
        <div className="min-w-0 space-y-3">
          <div>
            <label htmlFor={`${id}-name`} className="text-sm font-semibold">Name</label>
            <input
              id={`${id}-name`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={120}
              required
              placeholder="Also the download’s file name"
              className="mt-1 block min-h-11 w-full border border-line bg-surface px-3 text-base font-semibold"
            />
          </div>
          <fieldset>
            <legend className="text-sm font-semibold">
              Hook <span className="font-semibold text-muted">· on screen at the top from the first frame (optional)</span>
            </legend>
            <div className="mt-1 grid grid-cols-[minmax(0,1fr)_6.5rem] gap-2">
              <label className="min-w-0 text-xs font-semibold">
                Text
                <input
                  value={hook}
                  onChange={(e) => setHook(e.target.value)}
                  maxLength={120}
                  placeholder="e.g. How diamonds built a nation"
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 text-base font-medium"
                />
              </label>
              <label className="text-xs font-semibold">
                Seconds
                <input
                  type="number"
                  min={0.5}
                  step={0.5}
                  value={hookSeconds}
                  onChange={(e) => setHookSeconds(e.target.value)}
                  disabled={!hook.trim()}
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 font-mono text-base disabled:opacity-60"
                />
              </label>
            </div>
          </fieldset>
        </div>
        <fieldset className="min-w-0">
          <legend className="text-sm font-semibold">
            Shape <span className="font-semibold text-muted">· {shape.hint}</span>
          </legend>
          <div className="mt-1 grid grid-cols-2 gap-1.5 @[20rem]:grid-cols-4">
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
          {used.length > 0 && (
            <div className="mt-2">
              <FramingChoice project={project} assets={used} auto={auto} onAuto={setAuto} canAnalyse={canMakeVoiceover} />
            </div>
          )}
        </fieldset>
      </div>

      <fieldset>
        <legend className="text-sm font-semibold">
          Segments{" "}
          <span className="font-semibold text-muted">
            · {segments.length} of {MAX_SEGMENTS} · {stampTenths(total)} in total
          </span>
        </legend>
        <p className="text-xs font-semibold text-muted">Played in order. Times are minutes:seconds in each source video, like 1:30.5.</p>
        <ol className="mt-2 space-y-2">
          {drafts.map((d, i) => (
            <SegmentEditor
              key={d.key}
              project={project}
              index={i}
              count={drafts.length}
              draft={d}
              files={files}
              aspect={aspect}
              auto={auto}
              offsetMS={offsets[i]}
              onChange={(patch) => set(i, patch)}
              onMove={(by) => move(i, by)}
              onRemove={() => setSegments((ss) => ss.filter((_, j) => j !== i))}
            />
          ))}
        </ol>
        <button
          type="button"
          onClick={addSegment}
          disabled={segments.length >= MAX_SEGMENTS || files.length === 0}
          className="btn btn-secondary mt-2 min-h-11 px-3 text-sm disabled:opacity-60"
        >
          + Add segment
        </button>
      </fieldset>

      <fieldset>
        <legend className="text-sm font-semibold">Captions</legend>
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
            <CaptionOptionsPicker value={captions} onChange={setCaptions} speakers={[]} />
            <p className="mt-1 text-xs font-semibold text-muted">
              Captions come from each file’s transcript, so every video in the montage must be transcribed. Check the transcripts for
              mistakes first.
            </p>
          </>
        )}
      </fieldset>

      <VoiceoverPicker
        project={project}
        canMake={canMakeVoiceover}
        initialScript={initial?.script.slice(0, 1200) ?? ""}
        value={voiceover}
        onChange={setVoiceover}
        totalMS={total}
      />

      <SignOffPicker project={project} canMake={canMakeVoiceover} value={signOff} onChange={setSignOff} totalMS={total} />

      {(problem || create.isError) && <Banner>{problem ?? create.error?.message}</Banner>}
      <div className="flex flex-wrap items-center gap-2 border-t border-line pt-3">
        <button type="submit" disabled={create.isPending || files.length === 0} className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60">
          {create.isPending ? "Starting…" : "Render montage"}
        </button>
        <button type="button" onClick={onClose} className="btn btn-secondary min-h-11 px-3 text-sm">
          Cancel
        </button>
        <span className="text-xs font-semibold text-muted">
          {aspect} · {stampTenths(total)}
        </span>
      </div>
    </form>
  );
}
