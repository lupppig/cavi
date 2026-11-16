"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, NotFound, raise, useAIModelSettings, type EditPlan, type PlanRun, type Project } from "@/lib/queries";
import { BatchPanel } from "./batch";
import { FlagList, useBrief } from "./brief-section";
import { stampTenths } from "./clips";
import { useProjectMedia } from "./media-section";
import { MontageComposer, type MontageInitial } from "./montage-composer";
import { MontagesList, useMontages } from "./montages-list";
import { stamp } from "./transcript-panel";

/** Montage length choices; the API takes 10–180 s with at least 5 s between. */
const lengths = [
  { min: 20, max: 40, label: "20–40s", hint: "Best for Shorts" },
  { min: 15, max: 30, label: "15–30s", hint: "Punchy" },
  { min: 30, max: 60, label: "30–60s", hint: "A fuller moment" },
  { min: 60, max: 90, label: "60–90s", hint: "Room for a story" },
];
const defaultLength = 0;

const busy = (p: PlanRun) => p.status === "queued" || p.status === "running";

function usePlans(project: Project) {
  return useQuery({
    queryKey: ["project", project.id, "plans"],
    queryFn: async (): Promise<PlanRun | null> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/plans", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
      });
      if (response.status === 404) return null;
      if (!data) raise(error, response.status, "Could not load plans.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && n < 2,
    refetchInterval: (q) => (q.state.data && busy(q.state.data) ? 3000 : false),
  });
}

/**
 * Edit plans and montages for the whole project. Editors have AI propose
 * montages across the transcribed files; people who can render turn a
 * plan (or nothing) into a montage and download the result.
 */
export function PlansSection({ project, canEdit, canRender }: { project: Project; canEdit: boolean; canRender: boolean }) {
  const id = useId();
  const plans = usePlans(project);
  const montages = useMontages(project);
  const [requesting, setRequesting] = useState(false);
  const [composing, setComposing] = useState<null | { key: number; title: string; initial?: MontageInitial }>(null);

  if (plans.isPending || montages.isPending) return null;
  const run = plans.data ?? null;
  if (!run && montages.montages.length === 0 && !canEdit && !canRender && !plans.isError) return null;
  const showForm = canEdit && !composing && (!run || run.status === "failed" || (requesting && !busy(run)));

  function make(plan: EditPlan, i: number) {
    setComposing({
      key: Date.now(),
      title: `Making plan ${i + 1}: “${plan.title}”`,
      initial: {
        name: plan.title,
        hook: plan.hook,
        script: plan.voiceover_script,
        commentary: plan.commentary,
        segments: plan.segments.map((s) => ({ assetID: s.source_asset_id, startMS: s.start_ms, endMS: s.end_ms })),
      },
    });
  }

  return (
    <section aria-labelledby={`${id}-h`} className="card p-4 sm:p-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h2 id={`${id}-h`} className="text-lg font-semibold">Plans and montages</h2>
        {canRender && !composing && (
          <button
            type="button"
            onClick={() => setComposing({ key: Date.now(), title: "New montage" })}
            className="btn btn-secondary ml-auto min-h-11 px-3 text-sm"
          >
            + New montage
          </button>
        )}
      </div>
      {!run && canEdit && !composing && (
        <p className="mt-1 text-sm font-medium text-muted">
          Have CAVI propose up to ten clips across your transcribed videos, each opening on its strongest line with a hook on screen.
          Check the plans, then make one, or all of them at once.
        </p>
      )}
      {plans.isError && <div className="mt-3"><Banner>{plans.error.message}</Banner></div>}

      {composing && (
        <div className="mt-3">
          <p className="mb-2 text-sm font-semibold text-muted">{composing.title}</p>
          <MontageComposer
            key={composing.key}
            project={project}
            initial={composing.initial}
            canMakeVoiceover={canEdit}
            onClose={() => setComposing(null)}
          />
        </div>
      )}

      {!composing && run && busy(run) && (
        <p className="mt-3 flex items-center gap-2 text-sm font-semibold" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
          Planning montages with {run.model_label}…
        </p>
      )}

      {!composing && run?.status === "failed" && (
        <p className="mt-3 border border-line bg-surface-2 p-2 text-sm font-semibold" role="status">
          {run.error ?? "Planning failed."}
        </p>
      )}

      {!composing && run?.status === "ready" && !requesting && (
        <PlanList project={project} run={run} canEdit={canEdit} canRender={canRender} onMake={make} onAgain={() => setRequesting(true)} />
      )}

      {showForm && <RequestForm project={project} run={run} onDone={() => setRequesting(false)} onCancel={run?.status === "ready" ? () => setRequesting(false) : undefined} />}

      <h3 className="mt-5 border-t border-line pt-4 font-semibold">
        Montages{montages.montages.length > 0 && <span className="text-muted"> · {montages.montages.length}</span>}
      </h3>
      <div className="mt-2">
        <MontagesList project={project} canRender={canRender} />
      </div>
    </section>
  );
}

function PlanList({
  project,
  run,
  canEdit,
  canRender,
  onMake,
  onAgain,
}: {
  project: Project;
  run: PlanRun;
  canEdit: boolean;
  canRender: boolean;
  onMake: (plan: EditPlan, i: number) => void;
  onAgain: () => void;
}) {
  const media = useProjectMedia(project);
  const files = media.data?.pages.flatMap((p) => p.data) ?? [];
  const fileName = (assetID: string) => files.find((f) => f.id === assetID)?.filename ?? "A file no longer in this project";
  const [batch, setBatch] = useState(false);

  return (
    <>
      {run.series && run.plans[0]?.series_title && (
        <p className="mt-3 text-sm font-semibold">
          Series: “{run.plans[0].series_title}” · {run.plans.length} parts
        </p>
      )}
      {canRender && run.plans.length > 1 && !batch && (
        <button type="button" onClick={() => setBatch(true)} className="btn btn-primary mt-3 min-h-11 px-3 text-sm">
          {run.series ? `Make all ${run.plans.length} parts` : `Make all ${run.plans.length} as clips`}
        </button>
      )}
      {batch && <BatchPanel project={project} plans={run.plans} onClose={() => setBatch(false)} />}
      {run.plans.length === 0 ? (
        <p className="mt-3 text-sm font-semibold text-muted">No montages fit. Try a different objective or length.</p>
      ) : (
        <ol className="mt-3 space-y-2 sm:max-h-[36rem] sm:overflow-y-auto">
          {run.plans.map((p, i) => (
            <li key={i} className="rounded-xl border border-line bg-surface-2 p-2.5">
              <div className="flex items-start gap-2.5">
                <span
                  aria-label={p.part ? `Part ${p.part}` : `Plan ${i + 1}`}
                  className="grid size-7 shrink-0 place-items-center rounded-full bg-accent-soft text-xs font-semibold text-accent-text"
                >
                  {p.part ? `P${p.part}` : i + 1}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="font-semibold leading-snug break-words">{p.title}</p>
                  <p className="mt-0.5 font-mono text-xs font-bold text-muted">
                    {stamp(p.duration_ms)} · {p.segments.length} {p.segments.length === 1 ? "segment" : "segments"}
                  </p>
                  {p.hook && (
                    <p className="mt-1 text-sm font-medium break-words">
                      <span className="font-semibold">On screen: </span>“{p.hook}”
                    </p>
                  )}
                  {p.commentary.length > 0 && (
                    <ol className="mt-1 space-y-0.5 text-sm font-medium break-words">
                      {p.commentary.map((c) => (
                        <li key={c.segment}>
                          <span className="font-semibold">{c.segment > p.segments.length ? "At the end: " : `Segment ${c.segment}: `}</span>“{c.text}”
                        </li>
                      ))}
                    </ol>
                  )}
                  {p.voiceover_script && (
                    <p className="mt-1 text-sm font-medium break-words">
                      <span className="font-semibold">Voiceover: </span>“{p.voiceover_script}”
                    </p>
                  )}
                  <p className="mt-1 text-sm font-medium break-words text-subtle">{p.reason}</p>
                  {p.campaign_link && (
                    <p className="mt-1 text-sm font-medium break-words">
                      <span className="font-semibold text-accent-text">Campaign: </span>
                      {p.campaign_link}
                    </p>
                  )}
                  <ol className="mt-2 space-y-1.5 border-l-2 border-line pl-2.5">
                    {p.segments.map((s, j) => (
                      <li key={j} className="text-sm">
                        <p className="min-w-0 font-semibold break-words">
                          <span className="font-mono text-xs text-muted">{j + 1}.</span> {fileName(s.source_asset_id)}{" "}
                          <span className="font-mono text-xs font-bold whitespace-nowrap text-muted">
                            · {stampTenths(s.start_ms)}–{stampTenths(s.end_ms)}
                          </span>
                        </p>
                        <FlagList flags={s.flags} />
                      </li>
                    ))}
                  </ol>
                  {canRender && (
                    <button
                      type="button"
                      onClick={() => onMake(p, i)}
                      className="btn btn-primary mt-2 min-h-11 px-3 text-sm"
                      aria-label={`Make plan ${i + 1}: ${p.title}`}
                    >
                      Make this
                    </button>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}
      <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2">
        {canEdit && (
          <button type="button" onClick={onAgain} className="btn btn-secondary min-h-11 px-3 text-sm">
            Plan again
          </button>
        )}
        <span className="min-w-0 text-xs font-semibold break-words text-muted">
          {run.used_brief ? <>Following the creator brief · </> : null}
          {run.objective ? <>Asked for “{run.objective}” · </> : null}
          {lengths.find((l) => l.min === run.min_seconds && l.max === run.max_seconds)?.label ?? `${run.min_seconds}–${run.max_seconds}s`}
          {" "}· Suggested by {run.model_label}. Check each plan before you use it.
        </span>
      </div>
    </>
  );
}

function RequestForm({
  project,
  run,
  onDone,
  onCancel,
}: {
  project: Project;
  run: PlanRun | null;
  onDone: () => void;
  onCancel?: () => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const brief = useBrief(project);
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "plans")?.model;
  const [objective, setObjective] = useState(run?.objective ?? "");
  const [series, setSeries] = useState(run?.series ?? false);
  const [length, setLength] = useState(() => {
    const i = lengths.findIndex((l) => l.min === run?.min_seconds && l.max === run?.max_seconds);
    return i >= 0 ? i : defaultLength;
  });

  const request = useMutation({
    mutationFn: async () => {
      const { min, max } = lengths[length];
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/plans", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { objective: objective.trim(), min_seconds: min, max_seconds: max, series },
      });
      if (!data) raise(error, response.status, "Could not start planning.");
      return data;
    },
    onSuccess: (p) => {
      qc.setQueryData(["project", project.id, "plans"], p);
      onDone();
    },
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const where = model ? `${model.provider === "gemini" ? "Google " : ""}${model.label}` : "your organisation’s AI provider";
    const what = brief.data?.status === "ready" ? " and the creator brief" : "";
    const replacing = run?.status === "ready" ? " New plans replace the current ones." : "";
    if (window.confirm(`The transcripts of your newest transcribed files${what} (not your media) will be sent to ${where} to plan montages.${replacing} Continue?`)) {
      request.mutate();
    }
  }

  return (
    <form onSubmit={submit} className="mt-3 space-y-3">
      {request.isError && <Banner>{request.error.message}</Banner>}
      <div>
        <label htmlFor={`${id}-objective`} className="text-sm font-semibold">
          What should the montages do? <span className="font-semibold text-muted">(optional)</span>
        </label>
        <textarea
          id={`${id}-objective`}
          value={objective}
          onChange={(e) => setObjective(e.target.value)}
          maxLength={500}
          rows={2}
          placeholder="e.g. Short TikToks on how diamonds fund education, each opening on a strong line"
          className="mt-1 block w-full border border-line bg-surface px-3 py-2 text-base font-medium"
        />
      </div>
      <fieldset>
        <legend className="text-sm font-semibold">Montage length</legend>
        <div className="mt-1 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {lengths.map((l, i) => (
            <label
              key={l.label}
              className={`flex min-h-11 min-w-0 cursor-pointer flex-col justify-center border border-line px-2.5 py-1.5 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                length === i ? "bg-accent-soft text-accent-text" : "bg-white"
              }`}
            >
              <input type="radio" name={`${id}-length`} checked={length === i} onChange={() => setLength(i)} className="sr-only" />
              <span className="text-sm font-semibold">{l.label}</span>
              <span className="text-xs font-semibold text-muted">{l.hint}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <label className="flex min-h-11 cursor-pointer items-start gap-2 text-sm font-semibold">
        <input type="checkbox" checked={series} onChange={(e) => setSeries(e.target.checked)} className="mt-0.5 size-5 shrink-0 accent-[var(--ink)]" />
        <span>
          One story in parts (a series)
          <span className="block text-xs text-muted">
            Part 1, Part 2… each ending on a cliffhanger the next one answers, so viewers follow you for the rest. Each part keeps the
            length above.
          </span>
        </span>
      </label>
      <p className="text-xs font-semibold text-muted">
        Plans use your newest transcribed files (up to 5){brief.data?.status === "ready" ? " and follow the creator brief" : ""}. Moments the
        brief says never to use are left out.
      </p>
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={request.isPending} className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60">
          {request.isPending ? "Starting…" : run?.status === "failed" ? "Try again" : "Plan montages"}
        </button>
        {onCancel && (
          <button type="button" onClick={onCancel} className="btn btn-secondary min-h-11 px-3 text-sm">
            Cancel
          </button>
        )}
      </div>
    </form>
  );
}
