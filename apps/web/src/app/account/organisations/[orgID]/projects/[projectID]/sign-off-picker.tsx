"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, useAIModelSettings, type Project, type Voiceover } from "@/lib/queries";
import { stampTenths } from "./clips";
import { stable } from "./media-section";
import { busy, useVoiceover, useVoices } from "./voiceover-picker";

/** Mirrors the render worker (internal/render/montage.go). */
const SILENT_MS = 2500;
const TAIL_MS = 500;
const MIN_SPOKEN_MS = 1500;
export const MAX_SIGN_OFF = 80;
export const DEFAULT_SIGN_OFF = "Like and subscribe for more!";

export type SignOffChoice = { on: boolean; text: string; voiceoverID: string | null };

export const defaultSignOff: SignOffChoice = { on: true, text: DEFAULT_SIGN_OFF, voiceoverID: null };

/** How long the card shows: a fixed time, or while it's said plus a beat. */
export function signOffLengthMS(choice: SignOffChoice, v: Voiceover | undefined) {
  if (!choice.voiceoverID) return SILENT_MS;
  return Math.max((v?.duration_ms ?? 0) + TAIL_MS, MIN_SPOKEN_MS);
}

/**
 * The clip's ending: a card with a call to action ("Like and subscribe")
 * over the last moments, which the narrator can also say.
 */
export function SignOffPicker({
  project,
  canMake,
  value,
  onChange,
  totalMS,
}: {
  project: Project;
  canMake: boolean;
  value: SignOffChoice;
  onChange: (v: SignOffChoice) => void;
  totalMS: number;
}) {
  const id = useId();
  const qc = useQueryClient();
  const voices = useVoices();
  const chosen = useVoiceover(project, value.voiceoverID);
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "voiceover")?.model;
  const offered = (voices.data ?? []).filter((vc) => !model || vc.provider === model.provider);
  // A sign-off is said with energy where the voice model can do it.
  const style = offered[0]?.styles?.includes("excited") ? "excited" : "natural";

  const make = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/voiceovers", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { script: value.text.trim(), style },
      });
      if (!data) raise(error, response.status, "Could not start the spoken sign-off.");
      return data;
    },
    onSuccess: (v) => {
      qc.setQueryData(["project", project.id, "voiceovers", v.id], v);
      qc.invalidateQueries({ queryKey: ["project", project.id, "voiceovers"], exact: true });
      onChange({ ...value, voiceoverID: v.id });
    },
  });

  function speak() {
    if (!value.text.trim()) return;
    const where = model ? model.label : "your organisation’s AI provider";
    if (window.confirm(`The sign-off will be sent to ${where} to be spoken. Continue?`)) make.mutate();
  }

  const v = chosen.data;
  const audio = v?.status === "ready" ? stable(`voiceover:${v.id}`, v.url, v.url_expires_at) : null;
  const length = signOffLengthMS(value, v);

  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-semibold">Sign-off</legend>
      <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
        <input
          type="checkbox"
          checked={value.on}
          onChange={(e) => onChange({ ...value, on: e.target.checked })}
          className="size-5 accent-[var(--ink)]"
        />
        End with a sign-off card
      </label>
      {value.on && (
        <div className="space-y-2 border border-line p-2.5">
          <label htmlFor={`${id}-text`} className="block text-xs font-semibold">
            Card text <span className="text-muted">· {value.text.length}/{MAX_SIGN_OFF}</span>
          </label>
          <input
            id={`${id}-text`}
            value={value.text}
            onChange={(e) => onChange({ ...value, text: e.target.value })}
            maxLength={MAX_SIGN_OFF}
            placeholder={DEFAULT_SIGN_OFF}
            className="block min-h-11 w-full border border-line bg-surface px-3 text-base font-medium"
          />
          <p className="text-xs font-semibold text-muted">
            Shown in the middle for the last {(length / 1000).toFixed(1)} s{totalMS ? ` (from ${stampTenths(Math.max(totalMS - length, 0))})` : ""}.
            Captions stop when it appears.
          </p>
          {make.isError && <Banner>{make.error.message}</Banner>}
          {!value.voiceoverID ? (
            canMake && (
              <button
                type="button"
                onClick={speak}
                disabled={make.isPending || !value.text.trim()}
                className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
              >
                {make.isPending ? "Starting…" : "Have the narrator say it"}
              </button>
            )
          ) : (
            <div className="space-y-2 rounded-xl border border-line bg-surface-2 p-2.5">
              {chosen.isPending && <p className="text-sm font-semibold text-muted">Loading the spoken sign-off…</p>}
              {chosen.isError && <Banner>{chosen.error.message}</Banner>}
              {v && busy(v) && (
                <p className="flex items-center gap-2 text-sm font-semibold" role="status">
                  <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
                  Speaking the sign-off with {v.model_label}…
                </p>
              )}
              {v?.status === "failed" && (
                <p className="border border-line bg-white p-2 text-sm font-semibold" role="status">
                  {v.error ?? "Speaking the sign-off failed."}
                </p>
              )}
              {v?.status === "ready" && (
                <>
                  <p className="text-sm font-medium break-words">
                    <span className="font-semibold">Narrator says ({stampTenths(v.duration_ms ?? 0)}): </span>“{v.script}”
                  </p>
                  {v.script.trim() !== value.text.trim() && (
                    <p className="text-xs font-semibold text-muted">The card text has changed since; say it again to match.</p>
                  )}
                  {audio && <audio src={audio} controls preload="metadata" className="w-full" aria-label="Spoken sign-off" />}
                </>
              )}
              <button type="button" onClick={() => onChange({ ...value, voiceoverID: null })} className="btn btn-ghost min-h-11 px-3 text-sm">
                Card only, no voice
              </button>
            </div>
          )}
        </div>
      )}
    </fieldset>
  );
}

/** Whether the sign-off can be rendered; the message says why not. */
export function signOffProblem(
  choice: SignOffChoice,
  v: Voiceover | undefined,
  totalMS: number,
  voiceoverEndMS: number | null,
): string | null {
  if (!choice.on) return null;
  if (!choice.text.trim()) return "Write the sign-off, or turn it off.";
  if (choice.voiceoverID) {
    if (!v || busy(v)) return "Wait for the spoken sign-off to finish, or use the card only.";
    if (v.status !== "ready" || !v.duration_ms) return "Speaking the sign-off failed. Try again or use the card only.";
  }
  const from = totalMS - signOffLengthMS(choice, v);
  if (from < 0) return "The sign-off is longer than the montage. Add footage.";
  if (voiceoverEndMS !== null && voiceoverEndMS > from)
    return `The voiceover runs into the sign-off, which starts at ${(from / 1000).toFixed(1)} s. Start the voiceover earlier or add footage.`;
  return null;
}
