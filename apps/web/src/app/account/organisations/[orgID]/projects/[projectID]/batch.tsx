"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type CaptionOptions, type Composition, type EditPlan, type Project } from "@/lib/queries";
import { defaultCaptionOptions } from "./caption-options";
import { useLook, useSaveLook, type SavedLook } from "./look";
import { framingReady } from "./framing";
import { useProjectMedia } from "./media-section";
import { MAX_SIGN_OFF } from "./sign-off-picker";
import { COMMENTARY_DUCK, speak, waitFor } from "./commentary";
import { needsPauses, PrepareForPauses } from "./tighten";

/** How long the hook shows in a batch clip. */
const HOOK_MS = 3000;

/** Captions when the look has none: word by word in capitals, popping in. */
const popCaptions: CaptionOptions = { ...defaultCaptionOptions, style: "pop", mode: "words", uppercase: true };

type Outcome = { state: "waiting" | "voicing" | "sending" | "queued" | "failed"; error?: string };

/**
 * Turns many plans into clips at once, with the settings short-form
 * clips do best with: vertical, the camera following people, pop
 * captions, pauses cut, the plan's hook and a sign-off. Each clip can
 * still be opened and changed in the composer afterwards by making the
 * plan again.
 */
export function BatchPanel(props: { project: Project; plans: EditPlan[]; onClose: () => void }) {
  const look = useLook(props.project.organisation_id);
  if (look.isPending) return <div className="mt-3 h-40 animate-pulse bg-surface-2" />;
  // Without the look (it failed to load), start from the defaults.
  return <Batch {...props} look={look.data} />;
}

function Batch({
  project,
  plans,
  look,
  onClose,
}: {
  project: Project;
  plans: EditPlan[];
  look?: SavedLook;
  onClose: () => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const save = useSaveLook(project.organisation_id);
  const series = plans.length > 1 && plans.every((p) => !!p.part);
  const media = useProjectMedia(project);
  const files = media.data?.pages.flatMap((p) => p.data) ?? [];
  const used = files.filter((f) => plans.some((p) => p.segments.some((s) => s.source_asset_id === f.id)));
  const canFollow = used.length > 0 && used.every(framingReady);
  const unprepared = needsPauses(used);

  const [chosen, setChosen] = useState<boolean[]>(() => plans.map(() => true));
  const [follow, setFollow] = useState(look?.follow_people ?? true);
  const [captions, setCaptions] = useState(look ? !!look.captions : true);
  const captionStyle = look?.captions ?? popCaptions;
  const [tighten, setTighten] = useState(look?.cut_pauses ?? true);
  const [signOff, setSignOff] = useState(look?.sign_off ?? "Follow for more!");
  const [badge, setBadge] = useState(look?.series_badge ?? true);
  const lines = plans.reduce((n, p, i) => n + (chosen[i] ? p.commentary.length : 0), 0);
  const chars = plans.reduce((n, p, i) => n + (chosen[i] ? p.commentary.reduce((m, c) => m + c.text.length, 0) : 0), 0);
  const [narrate, setNarrate] = useState(true);
  const [outcomes, setOutcomes] = useState<Outcome[]>(() => plans.map(() => ({ state: "waiting" })));
  const count = chosen.filter(Boolean).length;
  const set = (i: number, o: Outcome) => setOutcomes((os) => os.map((x, j) => (j === i ? o : x)));

  const run = useMutation({
    mutationFn: async () => {
      for (const [i, plan] of plans.entries()) {
        if (!chosen[i]) continue;
        // Voice the plan's commentary first: every line, then wait for them.
        let commentary: { voiceover_id: string; segment: number; start_ms: number; duck: number }[] = [];
        if (narrate && plan.commentary.length > 0) {
          set(i, { state: "voicing" });
          try {
            const started = [];
            for (const c of plan.commentary) started.push({ segment: c.segment, v: await speak(project, c.text, "", "natural") });
            const done = await Promise.all(started.map(async (s) => ({ segment: s.segment, v: await waitFor(project, s.v.id) })));
            const failed = done.find((d) => d.v.status !== "ready");
            if (failed) throw new Error(failed.v.error ?? "A commentary line couldn’t be spoken.");
            commentary = done.map((d) => ({ voiceover_id: d.v.id, segment: d.segment, start_ms: 0, duck: COMMENTARY_DUCK }));
          } catch (e) {
            set(i, { state: "failed", error: e instanceof Error ? e.message : "The commentary couldn’t be spoken." });
            continue;
          }
        }
        set(i, { state: "sending" });
        const segs = plan.segments.map((s) => ({ source_asset_id: s.source_asset_id, start_ms: s.start_ms, end_ms: s.end_ms, focus: 0.5 }));
        const composition: Composition = {
          version: 3,
          source_asset_id: segs[0].source_asset_id,
          start_ms: segs[0].start_ms,
          end_ms: segs[0].end_ms,
          aspect: "9:16",
          focus: 0.5,
          segments: segs,
          ...(follow && canFollow ? { framing: "auto" as const } : {}),
          ...(tighten && unprepared.length === 0 ? { tighten: true } : {}),
          ...(plan.hook ? { hook: { text: plan.hook, end_ms: Math.min(HOOK_MS, plan.duration_ms) } } : {}),
          ...(commentary.length ? { commentary } : {}),
          ...(series && badge && plan.part ? { badge: `PART ${plan.part}/${plans.length}` } : {}),
          ...(signOffFor(plan) ? { sign_off: { text: signOffFor(plan) } } : {}),
        };
        const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
          params: { path: { orgID: project.organisation_id, projectID: project.id } },
          body: { name: plan.title.slice(0, 120), composition, ...(captions ? { captions: captionStyle } : {}) },
        });
        if (data) set(i, { state: "queued" });
        else {
          try {
            raise(error, response.status, "Could not start this clip.");
          } catch (e) {
            set(i, { state: "failed", error: e instanceof Error ? e.message : "Could not start this clip." });
          }
        }
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["project", project.id, "renders"] }),
  });
  // A series' parts point to the next one; the last signs off as usual.
  function signOffFor(plan: EditPlan) {
    if (series && plan.part && plan.part < plans.length) return `Follow for Part ${plan.part + 1}`;
    return signOff.trim();
  }
  const started = run.isPending || run.isSuccess;
  const queued = outcomes.filter((o) => o.state === "queued").length;

  return (
    <div className="mt-3 space-y-3 rounded-xl border border-line bg-surface-2 p-2.5 sm:p-3">
      <div>
        <h3 className="font-semibold">
          Make {count === plans.length ? "all" : count} of these as {series ? "parts" : "clips"}
        </h3>
        <p className="text-xs font-semibold text-muted">
          Vertical 9:16, each opening on its plan’s hook, in your look{look?.custom ? "" : " (the default until you save one)"}. They render
          one after another; you can leave this page.
        </p>
      </div>
      <fieldset className="space-y-1" disabled={started}>
        <legend className="sr-only">Settings</legend>
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60">
          <input type="checkbox" checked={follow && canFollow} disabled={!canFollow} onChange={(e) => setFollow(e.target.checked)} className="size-5 accent-[var(--ink)]" />
          <span>
            Camera follows people
            {!canFollow && <span className="block text-xs text-muted">Not every video has been analysed for this yet.</span>}
          </span>
        </label>
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
          <input type="checkbox" checked={captions} onChange={(e) => setCaptions(e.target.checked)} className="size-5 accent-[var(--ink)]" />
          Captions ({captionStyle.style === "pop" ? "pop, word by word" : captionStyle.style}, from the transcripts)
        </label>
        {lines > 0 && (
          <label className="flex min-h-11 cursor-pointer items-start gap-2 text-sm font-semibold">
            <input type="checkbox" checked={narrate} onChange={(e) => setNarrate(e.target.checked)} className="mt-0.5 size-5 shrink-0 accent-[var(--ink)]" />
            <span>
              Narrate the commentary ({lines} lines, {chars} characters of your voice model)
              <span className="block text-xs text-muted">
                Commentary through each clip is what makes it your own work for YouTube’s reused-content review. Keep this page open while
                the lines are voiced.
              </span>
            </span>
          </label>
        )}
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
          <input type="checkbox" checked={tighten} onChange={(e) => setTighten(e.target.checked)} className="size-5 accent-[var(--ink)]" />
          Cut pauses
        </label>
        {tighten && unprepared.length > 0 && <PrepareForPauses project={project} files={unprepared} />}
        {series && (
          <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
            <input type="checkbox" checked={badge} onChange={(e) => setBadge(e.target.checked)} className="size-5 accent-[var(--ink)]" />
            PART 1/{plans.length} badge on each part
          </label>
        )}
        <label htmlFor={`${id}-signoff`} className="block pt-1 text-xs font-semibold">
          Sign-off card <span className="text-muted">· empty for none{series ? "; earlier parts say “Follow for Part 2” and so on" : ""}</span>
        </label>
        <input
          id={`${id}-signoff`}
          value={signOff}
          onChange={(e) => setSignOff(e.target.value)}
          maxLength={MAX_SIGN_OFF}
          className="block min-h-11 w-full border border-line bg-white px-3 text-base font-medium"
        />
      </fieldset>
      <ul className="space-y-1" aria-label="Plans to make">
        {plans.map((p, i) => {
          const o = outcomes[i];
          return (
            <li key={i}>
              <label className="flex min-h-11 cursor-pointer items-start gap-2 py-1 text-sm">
                <input
                  type="checkbox"
                  checked={chosen[i]}
                  disabled={started}
                  onChange={(e) => setChosen((c) => c.map((x, j) => (j === i ? e.target.checked : x)))}
                  className="mt-0.5 size-5 shrink-0 accent-[var(--ink)]"
                />
                <span className="min-w-0">
                  <span className="font-semibold break-words">{p.title}</span>
                  {o.state !== "waiting" && (
                    <span className={`block text-xs font-semibold ${o.state === "failed" ? "text-ink" : "text-muted"}`} role="status">
                      {o.state === "voicing" ? "Voicing the commentary…" : o.state === "sending" ? "Starting…" : o.state === "queued" ? "Rendering — see Montages below" : o.error}
                    </span>
                  )}
                </span>
              </label>
            </li>
          );
        })}
      </ul>
      {run.isError && <Banner>{run.error.message}</Banner>}
      {save.isError && <Banner>{save.error.message}</Banner>}
      <div className="flex flex-wrap items-center gap-2">
        {!started ? (
          <button
            type="button"
            onClick={() => run.mutate()}
            disabled={count === 0}
            className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60"
          >
            Render {count} {count === 1 ? "clip" : "clips"}
          </button>
        ) : (
          <span className="text-sm font-semibold" role="status">
            {run.isPending ? "Starting clips…" : `${queued} of ${count} started.`}
          </span>
        )}
        {!started && (
          <button
            type="button"
            onClick={() =>
              save.mutate({
                captions: captions ? captionStyle : null,
                sign_off: signOff.trim(),
                follow_people: follow,
                cut_pauses: tighten,
                series_badge: badge,
              })
            }
            disabled={save.isPending}
            className="btn btn-ghost min-h-11 px-3 text-sm disabled:opacity-60"
          >
            {save.isPending ? "Saving…" : save.isSuccess ? "Saved as your look" : "Save as our look"}
          </button>
        )}
        <button type="button" onClick={onClose} disabled={run.isPending} className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60">
          {started && !run.isPending ? "Done" : "Cancel"}
        </button>
      </div>
    </div>
  );
}
