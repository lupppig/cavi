"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import {
  client,
  NotFound,
  raise,
  useAIModelSettings,
  type Brief,
  type BriefFlag,
  type BriefRule,
  type BriefRuleKind,
  type MediaAsset,
  type Project,
} from "@/lib/queries";
import { useProjectMedia } from "./media-section";
import { stamp } from "./transcript-panel";

/** What each rule kind means, in display order. */
export const ruleKinds: { kind: BriefRuleKind; label: string; hint: string; chip: string }[] = [
  { kind: "priority", label: "Feature", hint: "A moment the creator wants clipped", chip: "bg-accent-soft text-accent-text border-accent-line" },
  { kind: "exclude", label: "Never use", hint: "Clips that include it can’t be made", chip: "bg-danger-soft text-danger border-danger/30" },
  { kind: "blur", label: "Blur or crop", hint: "Something on screen must be hidden", chip: "bg-surface-2 text-ink border-line-strong" },
  { kind: "mute", label: "Don’t use audio", hint: "e.g. uncleared music", chip: "bg-surface-2 text-ink border-line-strong" },
  { kind: "caution", label: "Caution", hint: "Sensitive topic or third-party branding", chip: "bg-surface-2 text-ink border-line-strong" },
  { kind: "review", label: "Needs review", hint: "A person must check it first", chip: "bg-surface-2 text-ink border-line-strong" },
];
const kindInfo = (k: BriefRuleKind) => ruleKinds.find((r) => r.kind === k) ?? ruleKinds[0];

export function span(r: { start_ms: number; end_ms: number }) {
  return r.end_ms > r.start_ms ? `${stamp(r.start_ms)}–${stamp(r.end_ms)}` : stamp(r.start_ms);
}

/** Reads M:SS or H:MM:SS into milliseconds, or null. */
function parseTime(s: string): number | null {
  const m = s.trim().match(/^(?:(\d+):)?(\d{1,3}):([0-5]\d)$/);
  if (!m) return null;
  return ((Number(m[1] ?? 0) * 60 + Number(m[2])) * 60 + Number(m[3])) * 1000;
}

/** A project's creator brief, or null if it has none. Polls while it's being read. */
export function useBrief(project: Project) {
  return useQuery({
    queryKey: ["project", project.id, "brief"],
    queryFn: async (): Promise<Brief | null> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/projects/{projectID}/brief", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
      });
      if (response.status === 404) return null;
      if (!data) raise(error, response.status, "Could not load the brief.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && n < 2,
    refetchInterval: (q) => (q.state.data && (q.state.data.status === "queued" || q.state.data.status === "running") ? 2500 : false),
  });
}

/** Small labels for the brief rules a clip touches. */
export function FlagList({ flags }: { flags: BriefFlag[] }) {
  if (flags.length === 0) return null;
  return (
    <ul className="mt-2 flex flex-wrap gap-1.5" aria-label="Creator brief flags">
      {flags.map((f, i) => {
        const k = kindInfo(f.kind);
        return (
          <li key={i} className={`max-w-full rounded-md border px-2 py-1 text-xs font-semibold break-words ${k.chip}`}>
            {k.label} · {span(f)} · {f.note}
          </li>
        );
      })}
    </ul>
  );
}

/**
 * The creator's brief for this project. People with project.update paste
 * it and choose the video its timestamps refer to; AI reads it into rules
 * they then check and correct. Everyone with access can read it.
 */
export function BriefSection({ project, canEdit }: { project: Project; canEdit: boolean }) {
  const id = useId();
  const brief = useBrief(project);
  const [mode, setMode] = useState<"view" | "paste" | "edit">("view");
  const [open, setOpen] = useState(false);

  if (brief.isPending) return null;
  if (brief.isError) {
    return (
      <section className="card p-4 sm:p-5">
        <Banner>{brief.error.message}</Banner>
      </section>
    );
  }
  const b = brief.data;
  if (!b && !canEdit) return null;
  const busy = !!b && (b.status === "queued" || b.status === "running");
  const pasting = canEdit && !busy && (b === null || b.status === "failed" || mode === "paste");

  return (
    <section aria-labelledby={`${id}-h`} className="card p-4 sm:p-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h2 id={`${id}-h`} className="text-lg font-semibold">Creator brief</h2>
        {b?.status === "ready" && <RuleSummary rules={b.rules} />}
        {b?.status === "ready" && mode === "view" && (
          <button
            type="button"
            onClick={() => setOpen((o) => !o)}
            aria-expanded={open}
            aria-controls={`${id}-body`}
            className="btn btn-ghost ml-auto min-h-11 px-3 text-sm"
          >
            {open ? "Hide" : "Show rules"}
          </button>
        )}
      </div>

      {b === null && (
        <p className="mt-1 text-sm font-medium text-muted">
          Paste the creator’s guidelines. CAVI reads them into rules every clip is checked against: moments to feature,
          moments that must never be used, and things to blur, mute or review.
        </p>
      )}

      {busy && (
        <p className="mt-3 flex items-center gap-2 text-sm font-semibold" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
          Reading the brief with {b.model_label}…
        </p>
      )}

      {b?.status === "failed" && (
        <p className="mt-3 border border-line bg-surface-2 p-2 text-sm font-semibold" role="status">
          {b.error ?? "Reading the brief failed."}
        </p>
      )}

      {pasting && (
        <PasteForm project={project} brief={b} onDone={() => setMode("view")} onCancel={b?.status === "ready" ? () => setMode("view") : undefined} />
      )}

      {b?.status === "ready" && mode === "edit" && (
        <RulesEditor project={project} brief={b} onDone={() => { setMode("view"); setOpen(true); }} />
      )}

      {b?.status === "ready" && mode === "view" && (
        <div id={`${id}-body`}>
          {b.campaign_message && (
            <p className="mt-3 text-sm font-medium break-words">
              <span className="font-semibold">Campaign: </span>
              {b.campaign_message}
            </p>
          )}
          {open && (
            <>
              {b.guidance && <p className="mt-2 text-sm font-medium break-words text-subtle">{b.guidance}</p>}
              <RuleList rules={b.rules} />
              <p className="mt-3 text-xs font-semibold text-muted">
                Read by {b.model_label}. Every time comes from the brief’s text; check the rules against the brief before relying on them.
                Clips that include a “Never use” moment can’t be made.
              </p>
              {canEdit && (
                <div className="mt-3 flex flex-wrap gap-2 border-t border-line pt-3">
                  <button type="button" onClick={() => setMode("edit")} className="btn btn-primary min-h-11 px-3 text-sm">
                    Edit rules
                  </button>
                  <button type="button" onClick={() => setMode("paste")} className="btn btn-secondary min-h-11 px-3 text-sm">
                    Replace brief
                  </button>
                  <DeleteBrief project={project} />
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  );
}

function RuleSummary({ rules }: { rules: BriefRule[] }) {
  const counts = ruleKinds.map((k) => ({ ...k, n: rules.filter((r) => r.kind === k.kind).length })).filter((k) => k.n > 0);
  if (counts.length === 0) return <span className="text-sm font-semibold text-muted">No timed rules</span>;
  return (
    <ul className="flex flex-wrap gap-1.5" aria-label="Rules in this brief">
      {counts.map((k) => (
        <li key={k.kind} className={`rounded-md border px-2 py-0.5 text-xs font-semibold ${k.chip}`}>
          {k.n} {k.label.toLowerCase()}
        </li>
      ))}
    </ul>
  );
}

function RuleList({ rules }: { rules: BriefRule[] }) {
  if (rules.length === 0) return <p className="mt-3 text-sm font-semibold text-muted">This brief has no timestamps.</p>;
  return (
    <ol className="mt-3 max-h-[26rem] divide-y divide-line overflow-y-auto rounded-xl border border-line">
      {rules.map((r, i) => {
        const k = kindInfo(r.kind);
        return (
          <li key={i} className="flex flex-col gap-1 px-3 py-2 sm:flex-row sm:items-center sm:gap-3">
            <span className="w-28 shrink-0 font-mono text-sm font-bold">{span(r)}</span>
            <span className={`w-fit shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold ${k.chip}`}>{k.label}</span>
            <span className="min-w-0 text-sm font-medium break-words">{r.note}</span>
          </li>
        );
      })}
    </ol>
  );
}

function usable(a: MediaAsset) {
  return (a.kind === "video" || a.kind === "audio") && a.status === "uploaded" && a.processing.status === "ready";
}

function PasteForm({
  project,
  brief,
  onDone,
  onCancel,
}: {
  project: Project;
  brief: Brief | null;
  onDone: () => void;
  onCancel?: () => void;
}) {
  const id = useId();
  const qc = useQueryClient();
  const media = useProjectMedia(project);
  const files = (media.data?.pages.flatMap((p) => p.data) ?? []).filter(usable);
  const [text, setText] = useState(brief?.text ?? "");
  const [assetID, setAssetID] = useState(brief?.media_asset_id ?? "");
  const chosen = files.find((f) => f.id === assetID) ?? (files.length === 1 ? files[0] : undefined);
  const settings = useAIModelSettings(project.organisation_id);
  const model = settings.data?.find((s) => s.task === "briefs")?.model;

  const submit = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().PUT("/api/v1/organisations/{orgID}/projects/{projectID}/brief", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { text: text.trim(), media_asset_id: chosen!.id },
      });
      if (!data) raise(error, response.status, "Could not submit the brief.");
      return data;
    },
    onSuccess: (b) => {
      qc.setQueryData(["project", project.id, "brief"], b);
      onDone();
    },
  });

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!chosen || !text.trim()) return;
    const where = model ? `${model.provider === "gemini" ? "Google " : ""}${model.label}` : "your organisation’s AI provider";
    const replacing = brief?.status === "ready" ? " It replaces the current brief and its rules, including your edits." : "";
    if (window.confirm(`The brief’s text (not your media) will be sent to ${where} to read it.${replacing} Continue?`)) {
      submit.mutate();
    }
  }

  return (
    <form onSubmit={onSubmit} className="mt-3 space-y-3">
      {submit.isError && <Banner>{submit.error.message}</Banner>}
      <div>
        <label htmlFor={`${id}-text`} className="text-sm font-semibold">Brief</label>
        <textarea
          id={`${id}-text`}
          value={text}
          onChange={(e) => setText(e.target.value)}
          required
          maxLength={20000}
          rows={8}
          placeholder={"Paste the creator’s guidelines, with their timestamps. For example:\n01:30 – How it started\n0:47 - 0:53 (must not be used)\nLicence plates: 03:13 – blur"}
          className="mt-1 block w-full border border-line bg-surface px-3 py-2 text-base font-medium"
        />
      </div>
      <fieldset>
        <legend className="text-sm font-semibold">Which video do the timestamps refer to?</legend>
        {media.isPending ? (
          <p className="mt-1 text-sm font-semibold text-muted">Loading files…</p>
        ) : files.length === 0 ? (
          <p className="mt-1 text-sm font-semibold text-muted">
            Upload the full-length video below and wait for it to finish processing, then submit the brief.
          </p>
        ) : (
          <div className="mt-1 grid gap-2 sm:grid-cols-2">
            {files.map((f) => (
              <label
                key={f.id}
                className={`flex min-h-11 min-w-0 cursor-pointer flex-col justify-center border border-line px-3 py-2 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                  chosen?.id === f.id ? "bg-accent-soft text-accent-text" : "bg-white"
                }`}
              >
                <input
                  type="radio"
                  name={`${id}-video`}
                  checked={chosen?.id === f.id}
                  onChange={() => setAssetID(f.id)}
                  className="sr-only"
                />
                <span className="truncate text-sm font-semibold">{f.filename}</span>
                {f.metadata?.duration_ms != null && (
                  <span className="text-xs font-semibold text-muted">{stamp(f.metadata.duration_ms)} long</span>
                )}
              </label>
            ))}
          </div>
        )}
      </fieldset>
      <div className="flex flex-wrap gap-2">
        <button
          type="submit"
          disabled={submit.isPending || !chosen || !text.trim()}
          className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60"
        >
          {submit.isPending ? "Sending…" : brief?.status === "failed" ? "Try again" : "Read brief"}
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

type Draft = { kind: BriefRuleKind; start: string; end: string; note: string };

function RulesEditor({ project, brief, onDone }: { project: Project; brief: Brief; onDone: () => void }) {
  const id = useId();
  const qc = useQueryClient();
  const [campaign, setCampaign] = useState(brief.campaign_message);
  const [guidance, setGuidance] = useState(brief.guidance);
  const [rules, setRules] = useState<Draft[]>(() =>
    brief.rules.map((r) => ({ kind: r.kind, start: stamp(r.start_ms), end: r.end_ms > r.start_ms ? stamp(r.end_ms) : "", note: r.note })),
  );
  const [problem, setProblem] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: async (body: { campaign_message: string; guidance: string; rules: BriefRule[] }) => {
      const { data, error, response } = await client().PATCH("/api/v1/organisations/{orgID}/projects/{projectID}/brief", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body,
      });
      if (!data) raise(error, response.status, "Could not save the brief.");
      return data;
    },
    onSuccess: (b) => {
      qc.setQueryData(["project", project.id, "brief"], b);
      // Flags on highlights and clips come from the rules.
      qc.invalidateQueries({ queryKey: ["media"] });
      qc.invalidateQueries({ queryKey: ["project", project.id, "renders"] });
      onDone();
    },
  });

  const set = (i: number, patch: Partial<Draft>) => setRules((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)));

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setProblem(null);
    const out: BriefRule[] = [];
    for (const [i, r] of rules.entries()) {
      const start = parseTime(r.start);
      const end = r.end.trim() ? parseTime(r.end) : start;
      if (start === null || end === null) return setProblem(`Rule ${i + 1}: write times as minutes:seconds, like 3:18.`);
      if (end < start) return setProblem(`Rule ${i + 1}: the end is before the start.`);
      if (!r.note.trim()) return setProblem(`Rule ${i + 1}: describe what happens there.`);
      out.push({ media_asset_id: brief.media_asset_id!, kind: r.kind, start_ms: start, end_ms: end, note: r.note.trim() });
    }
    save.mutate({ campaign_message: campaign.trim(), guidance: guidance.trim(), rules: out });
  }

  if (!brief.media_asset_id) {
    return (
      <p className="mt-3 text-sm font-semibold text-muted">
        The video this brief referred to was deleted. Replace the brief and choose another file to edit its rules.
      </p>
    );
  }

  return (
    <form onSubmit={onSubmit} className="mt-3 space-y-3">
      {(problem || save.isError) && <Banner>{problem ?? save.error?.message}</Banner>}
      <div>
        <label htmlFor={`${id}-campaign`} className="text-sm font-semibold">Campaign message</label>
        <textarea
          id={`${id}-campaign`}
          value={campaign}
          onChange={(e) => setCampaign(e.target.value)}
          maxLength={2000}
          rows={2}
          className="mt-1 block w-full border border-line bg-surface px-3 py-2 text-base font-medium"
        />
      </div>
      <div>
        <label htmlFor={`${id}-guidance`} className="text-sm font-semibold">Guidance</label>
        <textarea
          id={`${id}-guidance`}
          value={guidance}
          onChange={(e) => setGuidance(e.target.value)}
          maxLength={2000}
          rows={2}
          className="mt-1 block w-full border border-line bg-surface px-3 py-2 text-base font-medium"
        />
      </div>
      <fieldset>
        <legend className="text-sm font-semibold">Rules</legend>
        <p className="text-xs font-semibold text-muted">Times are minutes:seconds. Leave “to” empty for a single moment.</p>
        <ol className="mt-2 space-y-2">
          {rules.map((r, i) => (
            <li key={i} className="grid grid-cols-2 gap-2 rounded-xl border border-line bg-surface-2 p-2 sm:grid-cols-[10rem_5.5rem_5.5rem_1fr_auto] sm:items-end">
              <label className="col-span-2 text-xs font-semibold sm:col-span-1">
                Kind
                <select
                  value={r.kind}
                  onChange={(e) => set(i, { kind: e.target.value as BriefRuleKind })}
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 text-sm font-semibold"
                >
                  {ruleKinds.map((k) => (
                    <option key={k.kind} value={k.kind}>{k.label}</option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-semibold">
                From
                <input
                  value={r.start}
                  onChange={(e) => set(i, { start: e.target.value })}
                  inputMode="numeric"
                  placeholder="3:18"
                  aria-label={`Rule ${i + 1} start`}
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 font-mono text-base"
                />
              </label>
              <label className="text-xs font-semibold">
                To
                <input
                  value={r.end}
                  onChange={(e) => set(i, { end: e.target.value })}
                  inputMode="numeric"
                  placeholder="—"
                  aria-label={`Rule ${i + 1} end`}
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 font-mono text-base"
                />
              </label>
              <label className="col-span-2 text-xs font-semibold sm:col-span-1">
                Note
                <input
                  value={r.note}
                  onChange={(e) => set(i, { note: e.target.value })}
                  maxLength={300}
                  className="mt-1 block min-h-11 w-full border border-line bg-white px-2 text-base font-medium"
                />
              </label>
              <button
                type="button"
                onClick={() => setRules((rs) => rs.filter((_, j) => j !== i))}
                className="btn btn-ghost col-span-2 min-h-11 px-3 text-sm sm:col-span-1"
                aria-label={`Remove rule ${i + 1}`}
              >
                Remove
              </button>
            </li>
          ))}
        </ol>
        <button
          type="button"
          onClick={() => setRules((rs) => [...rs, { kind: "priority", start: "", end: "", note: "" }])}
          className="btn btn-secondary mt-2 min-h-11 px-3 text-sm"
        >
          Add rule
        </button>
      </fieldset>
      <div className="flex flex-wrap gap-2 border-t border-line pt-3">
        <button type="submit" disabled={save.isPending} className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60">
          {save.isPending ? "Saving…" : "Save rules"}
        </button>
        <button type="button" onClick={onDone} className="btn btn-secondary min-h-11 px-3 text-sm">
          Cancel
        </button>
      </div>
    </form>
  );
}

function DeleteBrief({ project }: { project: Project }) {
  const qc = useQueryClient();
  const del = useMutation({
    mutationFn: async () => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/projects/{projectID}/brief", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
      });
      if (!response.ok) raise(error, response.status, "Could not delete the brief.");
    },
    onSuccess: () => {
      qc.setQueryData(["project", project.id, "brief"], null);
      qc.invalidateQueries({ queryKey: ["media"] });
      qc.invalidateQueries({ queryKey: ["project", project.id, "renders"] });
    },
  });
  return (
    <>
      <button
        type="button"
        disabled={del.isPending}
        onClick={() => window.confirm("Delete this brief and its rules? Clips will no longer be checked against it.") && del.mutate()}
        className="btn btn-ghost min-h-11 px-3 text-sm text-danger disabled:opacity-60"
      >
        {del.isPending ? "Deleting…" : "Delete brief"}
      </button>
      {del.isError && <Banner>{del.error.message}</Banner>}
    </>
  );
}

/**
 * How near a clip must come to a single-moment rule to be flagged. Briefs
 * name the moment something appears; plates and logos usually stay on
 * screen around it.
 */
export const FLAG_WINDOW_MS = 10_000;

/** Same rule as the API: a single moment counts if it's inside the clip; a range if they overlap. */
function overlaps(r: { start_ms: number; end_ms: number }, startMS: number, endMS: number) {
  return r.start_ms === r.end_ms ? r.start_ms >= startMS && r.start_ms < endMS : r.start_ms < endMS && startMS < r.end_ms;
}

/**
 * Same rule as the API for flags: single moments count within
 * FLAG_WINDOW_MS either side; ranges when they overlap. Priorities are
 * never flags. (Exclusions are refused on the exact test, overlaps.)
 */
export function flagged(r: { kind: BriefRuleKind; start_ms: number; end_ms: number }, startMS: number, endMS: number) {
  if (r.kind === "priority") return false;
  if (r.start_ms !== r.end_ms) return overlaps(r, startMS, endMS);
  return Math.max(0, r.start_ms - FLAG_WINDOW_MS) < endMS && startMS < r.end_ms + FLAG_WINDOW_MS;
}

/**
 * Checks a clip being composed against the brief as you move its in and
 * out points: an excluded moment is called out (the API refuses it), and
 * other rules show as flags.
 */
export function BriefCheck({ project, asset, startMS, endMS }: { project: Project; asset: MediaAsset; startMS: number; endMS: number }) {
  const brief = useBrief(project);
  const b = brief.data;
  if (!b || b.status !== "ready") return null;
  const mine = b.rules.filter((r) => r.media_asset_id === asset.id);
  const excluded = mine.filter((r) => r.kind === "exclude" && overlaps(r, startMS, endMS));
  // An exclusion near the clip (not in it) still shows as a flag.
  const flags = mine.filter((r) => flagged(r, startMS, endMS) && !excluded.includes(r));
  if (excluded.length === 0 && flags.length === 0) return null;
  return (
    <div role="status" className={`rounded-xl border p-2.5 text-sm font-semibold ${excluded.length ? "border-danger/30 bg-danger-soft" : "border-line bg-surface-2"}`}>
      {excluded.length > 0 ? (
        <p className="text-danger">
          This clip includes {excluded.map(span).join(", ")}, which the brief says must never be used. Move the in or out point to make it.
        </p>
      ) : (
        <p>This clip touches the brief:</p>
      )}
      <FlagList flags={flags} />
    </div>
  );
}
