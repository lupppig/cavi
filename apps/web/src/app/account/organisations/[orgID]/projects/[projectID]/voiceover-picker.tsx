"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, useAIModelSettings, type Project, type Voice, type Voiceover, type VoiceStyle } from "@/lib/queries";
import { stampTenths } from "./clips";
import { stable } from "./media-section";

export const busy = (v: Voiceover) => v.status === "queued" || v.status === "running";

export function useVoices() {
  return useQuery({
    queryKey: ["voices"],
    queryFn: async (): Promise<Voice[]> => {
      const { data, error, response } = await client().GET("/api/v1/voices");
      if (!data) raise(error, response.status, "Could not load voices.");
      return data.data;
    },
    staleTime: 10 * 60_000,
  });
}

function useVoiceovers(project: Project) {
  return useQuery({
    queryKey: ["project", project.id, "voiceovers"],
    queryFn: async (): Promise<Voiceover[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
      });
      if (!data) raise(error, response.status, "Could not load voiceovers.");
      return data.data;
    },
  });
}

/** One voiceover, polled until it's spoken; ready ones carry a fresh audio URL. */
export function useVoiceover(project: Project, voiceoverID: string | null) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: ["project", project.id, "voiceovers", voiceoverID],
    enabled: !!voiceoverID,
    queryFn: async (): Promise<Voiceover> => {
      const { data, error, response } = await client().GET(
        "/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers/{voiceoverID}",
        { params: { path: { orgID: project.organisation_id, projectID: project.id, voiceoverID: voiceoverID! } } },
      );
      if (!data) raise(error, response.status, "Could not load the voiceover.");
      if (!busy(data)) qc.invalidateQueries({ queryKey: ["project", project.id, "voiceovers"], exact: true });
      return data;
    },
    refetchInterval: (q) => (q.state.data && busy(q.state.data) ? 2500 : false),
  });
}

export type VoiceoverChoice = { id: string | null; start: string; duck: number };

/** The ready voiceover chosen, if any, for the composer to check and send. */
export function useChosenVoiceover(project: Project, id: string | null) {
  return useVoiceover(project, id).data;
}

function excerpt(s: string, n = 60) {
  return s.length > n ? `${s.slice(0, n).trimEnd()}…` : s;
}

/**
 * The montage's optional voiceover: speak a script (people with
 * editing.manage), or pick one already made, then place it and set how
 * much of the clip's own sound stays under it.
 */
export function VoiceoverPicker({
  project,
  canMake,
  initialScript,
  value,
  onChange,
  totalMS,
}: {
  project: Project;
  canMake: boolean;
  initialScript: string;
  value: VoiceoverChoice;
  onChange: (v: VoiceoverChoice) => void;
  totalMS: number;
}) {
  const id = useId();
  const qc = useQueryClient();
  const voices = useVoices();
  const list = useVoiceovers(project);
  const chosen = useVoiceover(project, value.id);
  const [script, setScript] = useState(initialScript);
  const [voice, setVoice] = useState("");
  const [style, setStyle] = useState<VoiceStyle>("natural");
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "voiceover")?.model;
  // Only the voices and styles of the organisation's voiceover model work.
  const offered = (voices.data ?? []).filter((vc) => !model || vc.provider === model.provider);
  const styles: VoiceStyle[] = offered[0]?.styles ?? ["natural"];
  const ready = (list.data ?? []).filter((v) => v.status === "ready");
  // Voices are stored by provider ID; show their names.
  const voiceName = (vid: string) => voices.data?.find((vc) => vc.id === vid)?.label.split(" · ")[0] ?? vid;

  const make = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { script: script.trim(), style, ...(voice ? { voice } : {}) },
      });
      if (!data) raise(error, response.status, "Could not start the voiceover.");
      return data;
    },
    onSuccess: (v) => {
      qc.setQueryData(["project", project.id, "voiceovers", v.id], v);
      qc.invalidateQueries({ queryKey: ["project", project.id, "voiceovers"], exact: true });
      onChange({ ...value, id: v.id });
    },
  });

  function speak() {
    if (!script.trim()) return;
    const where = model ? model.label : "your organisation’s AI provider";
    if (window.confirm(`The script will be sent to ${where} to be spoken. Continue?`)) make.mutate();
  }

  const v = chosen.data;
  const startMS = Math.round(Number(value.start) * 1000);
  const startOK = value.start.trim() !== "" && Number.isFinite(startMS) && startMS >= 0;
  const fits = !!v?.duration_ms && startOK && startMS + v.duration_ms <= totalMS;
  const audio = v?.status === "ready" ? stable(`voiceover:${v.id}`, v.url, v.url_expires_at) : null;

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-semibold">
        Voiceover <span className="font-semibold text-muted">(optional)</span>
      </legend>
      {make.isError && <Banner>{make.error.message}</Banner>}
      {list.isError && <Banner>{list.error.message}</Banner>}

      {canMake && (
        <div className="space-y-2 border border-line p-2.5">
          <label htmlFor={`${id}-script`} className="block text-xs font-semibold">
            Script <span className="text-muted">· {script.length}/1200</span>
          </label>
          <textarea
            id={`${id}-script`}
            value={script}
            onChange={(e) => setScript(e.target.value)}
            maxLength={1200}
            rows={3}
            placeholder="What the narrator says. Keep it shorter than the montage."
            className="block w-full border border-line bg-surface px-3 py-2 text-base font-medium"
          />
          <div className="flex flex-wrap items-end gap-2">
            <label className="min-w-0 flex-1 basis-40 text-xs font-semibold">
              Voice
              <select
                value={voice}
                onChange={(e) => setVoice(e.target.value)}
                disabled={!voices.data}
                className="mt-1 block min-h-11 w-full min-w-0 border border-line bg-white px-2 text-sm font-semibold"
              >
                {voices.isPending && <option value="">Loading voices…</option>}
                {voices.isError && <option value="">Default voice</option>}
                {offered.map((vc, i) => (
                  <option key={vc.id} value={i === 0 ? "" : vc.id}>
                    {vc.provider === "gemini" ? `${vc.id} · ` : ""}{vc.label}
                  </option>
                ))}
              </select>
            </label>
            {styles.length > 1 && (
              <label className="min-w-0 flex-1 basis-40 text-xs font-semibold">
                Delivery
                <select
                  value={style}
                  onChange={(e) => setStyle(e.target.value as VoiceStyle)}
                  className="mt-1 block min-h-11 w-full min-w-0 border border-line bg-white px-2 text-sm font-semibold capitalize"
                >
                  {styles.map((st) => (
                    <option key={st} value={st}>
                      {st}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <button
              type="button"
              onClick={speak}
              disabled={make.isPending || !script.trim() || (!!v && busy(v))}
              className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
            >
              {make.isPending ? "Starting…" : "Make voiceover"}
            </button>
          </div>
        </div>
      )}

      {ready.length > 0 && (
        <label className="block text-xs font-semibold">
          {canMake ? "Or use one already made" : "Use a voiceover made for this project"}
          <select
            value={value.id ?? ""}
            onChange={(e) => onChange({ ...value, id: e.target.value || null })}
            className="mt-1 block min-h-11 w-full min-w-0 border border-line bg-white px-2 text-sm font-semibold"
          >
            <option value="">No voiceover</option>
            {value.id && !ready.some((r) => r.id === value.id) && <option value={value.id}>The one being made</option>}
            {ready.map((r) => (
              <option key={r.id} value={r.id}>
                {stampTenths(r.duration_ms ?? 0)} · {voiceName(r.voice)} · “{excerpt(r.script)}”
              </option>
            ))}
          </select>
        </label>
      )}
      {!canMake && ready.length === 0 && list.isSuccess && (
        <p className="text-sm font-semibold text-muted">No voiceovers have been made for this project. Ask an editor to make one.</p>
      )}

      {value.id && (
        <div className="space-y-2 rounded-xl border border-line bg-surface-2 p-2.5">
          {chosen.isPending && <p className="text-sm font-semibold text-muted">Loading the voiceover…</p>}
          {chosen.isError && <Banner>{chosen.error.message}</Banner>}
          {v && busy(v) && (
            <p className="flex items-center gap-2 text-sm font-semibold" role="status">
              <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
              Speaking the script with {v.model_label}…
            </p>
          )}
          {v?.status === "failed" && (
            <p className="border border-line bg-white p-2 text-sm font-semibold" role="status">
              {v.error ?? "Making the voiceover failed."}
            </p>
          )}
          {v?.status === "ready" && (
            <>
              <p className="text-sm font-medium break-words">
                <span className="font-semibold">
                  {voiceName(v.voice)} · {stampTenths(v.duration_ms ?? 0)} long:{" "}
                </span>
                “{excerpt(v.script, 160)}”
              </p>
              {audio && <audio src={audio} controls preload="metadata" className="w-full" aria-label="Voiceover audio" />}
              <div className="grid gap-2 sm:grid-cols-2">
                <label className="text-xs font-semibold">
                  Starts at (seconds into the montage)
                  <input
                    type="number"
                    min={0}
                    step={0.1}
                    value={value.start}
                    onChange={(e) => onChange({ ...value, start: e.target.value })}
                    className="mt-1 block min-h-11 w-full border border-line bg-white px-2 font-mono text-base"
                  />
                </label>
                <label className="text-xs font-semibold">
                  Original sound under it · {Math.round(value.duck * 100)}%
                  <input
                    type="range"
                    min={0}
                    max={100}
                    value={Math.round(value.duck * 100)}
                    onChange={(e) => onChange({ ...value, duck: Number(e.target.value) / 100 })}
                    className="mt-1 block h-11 w-full accent-[var(--ink)]"
                  />
                </label>
              </div>
              {!startOK ? (
                <p className="text-sm font-semibold text-danger">Enter when the voiceover starts, in seconds.</p>
              ) : (
                !fits && (
                  <p className="text-sm font-semibold text-danger">
                    The voiceover is {((v.duration_ms ?? 0) / 1000).toFixed(1)} s long and starts at {(startMS / 1000).toFixed(1)} s, so it
                    runs past the end of the {(totalMS / 1000).toFixed(1)} s montage. Start it earlier, add footage or shorten the script.
                  </p>
                )
              )}
            </>
          )}
          <button type="button" onClick={() => onChange({ ...value, id: null })} className="btn btn-ghost min-h-11 px-3 text-sm">
            Don’t use a voiceover
          </button>
        </div>
      )}
    </fieldset>
  );
}

/** Whether the chosen voiceover is ready and fits; the message says why not. */
export function voiceoverProblem(v: Voiceover | undefined, choice: VoiceoverChoice, totalMS: number): string | null {
  if (!choice.id) return null;
  if (!v || busy(v)) return "Wait for the voiceover to finish, or don’t use one.";
  if (v.status !== "ready" || !v.duration_ms) return "That voiceover failed. Make it again or don’t use one.";
  const start = Number(choice.start);
  if (choice.start.trim() === "" || !Number.isFinite(start) || start < 0) return "Enter when the voiceover starts, in seconds.";
  if (Math.round(start * 1000) + v.duration_ms > totalMS) return "The voiceover runs past the end of the montage.";
  return null;
}
