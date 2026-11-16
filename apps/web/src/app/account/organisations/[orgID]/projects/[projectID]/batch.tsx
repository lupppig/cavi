"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type CaptionOptions, type Composition, type EditPlan, type Project } from "@/lib/queries";
import { defaultCaptionOptions } from "./caption-options";
import { framingReady } from "./framing";
import { useProjectMedia } from "./media-section";
import { DEFAULT_SIGN_OFF, MAX_SIGN_OFF } from "./sign-off-picker";
import { needsPauses, PrepareForPauses } from "./tighten";

/** How long the hook shows in a batch clip. */
const HOOK_MS = 3000;

/** Batch captions: word by word in capitals, each word popping in. */
const popCaptions: CaptionOptions = { ...defaultCaptionOptions, style: "pop", mode: "words", uppercase: true };

type Outcome = { state: "waiting" | "sending" | "queued" | "failed"; error?: string };

/**
 * Turns many plans into clips at once, with the settings short-form
 * clips do best with: vertical, the camera following people, pop
 * captions, pauses cut, the plan's hook and a sign-off. Each clip can
 * still be opened and changed in the composer afterwards by making the
 * plan again.
 */
export function BatchPanel({ project, plans, onClose }: { project: Project; plans: EditPlan[]; onClose: () => void }) {
  const id = useId();
  const qc = useQueryClient();
  const media = useProjectMedia(project);
  const files = media.data?.pages.flatMap((p) => p.data) ?? [];
  const used = files.filter((f) => plans.some((p) => p.segments.some((s) => s.source_asset_id === f.id)));
  const canFollow = used.length > 0 && used.every(framingReady);
  const unprepared = needsPauses(used);

  const [chosen, setChosen] = useState<boolean[]>(() => plans.map(() => true));
  const [follow, setFollow] = useState(true);
  const [captions, setCaptions] = useState(true);
  const [tighten, setTighten] = useState(true);
  const [signOff, setSignOff] = useState(DEFAULT_SIGN_OFF);
  const [outcomes, setOutcomes] = useState<Outcome[]>(() => plans.map(() => ({ state: "waiting" })));
  const count = chosen.filter(Boolean).length;
  const set = (i: number, o: Outcome) => setOutcomes((os) => os.map((x, j) => (j === i ? o : x)));

  const run = useMutation({
    mutationFn: async () => {
      for (const [i, plan] of plans.entries()) {
        if (!chosen[i]) continue;
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
          ...(signOff.trim() ? { sign_off: { text: signOff.trim() } } : {}),
        };
        const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/renders", {
          params: { path: { orgID: project.organisation_id, projectID: project.id } },
          body: { name: plan.title.slice(0, 120), composition, ...(captions ? { captions: popCaptions } : {}) },
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
  const started = run.isPending || run.isSuccess;
  const queued = outcomes.filter((o) => o.state === "queued").length;

  return (
    <div className="mt-3 space-y-3 rounded-xl border border-line bg-surface-2 p-2.5 sm:p-3">
      <div>
        <h3 className="font-semibold">Make {count === plans.length ? "all" : count} of these as clips</h3>
        <p className="text-xs font-semibold text-muted">
          Vertical 9:16, each opening on its plan’s hook. They render one after another; you can leave this page.
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
          Pop captions (word by word, from the transcripts)
        </label>
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
          <input type="checkbox" checked={tighten} onChange={(e) => setTighten(e.target.checked)} className="size-5 accent-[var(--ink)]" />
          Cut pauses
        </label>
        {tighten && unprepared.length > 0 && <PrepareForPauses project={project} files={unprepared} />}
        <label htmlFor={`${id}-signoff`} className="block pt-1 text-xs font-semibold">
          Sign-off card <span className="text-muted">· empty for none</span>
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
                      {o.state === "sending" ? "Starting…" : o.state === "queued" ? "Rendering — see Montages below" : o.error}
                    </span>
                  )}
                </span>
              </label>
            </li>
          );
        })}
      </ul>
      {run.isError && <Banner>{run.error.message}</Banner>}
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
        <button type="button" onClick={onClose} disabled={run.isPending} className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60">
          {started && !run.isPending ? "Done" : "Cancel"}
        </button>
      </div>
    </div>
  );
}
