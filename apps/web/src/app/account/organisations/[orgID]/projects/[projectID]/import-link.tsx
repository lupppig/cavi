"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type MediaAsset, type Project } from "@/lib/queries";

/** Whether a file is still coming in from a link (and hasn't given up). */
export const importing = (a: MediaAsset) => a.status === "importing" && !a.import_error;

/** Where a link points, shortened for display: "youtube.com/watch?v=…". */
export function shortLink(link: string) {
  try {
    const u = new URL(link);
    return `${u.hostname.replace(/^www\./, "")}${u.pathname}${u.search}`;
  } catch {
    return link;
  }
}

/**
 * Brings in a video from YouTube, Twitch or Kick: the media worker
 * downloads it, then it's processed like an upload. The person confirms
 * they have the rights to use it.
 */
export function ImportLinkForm({ project, onDone }: { project: Project; onDone: (asset: MediaAsset) => void }) {
  const id = useId();
  const qc = useQueryClient();
  const [link, setLink] = useState("");
  const [rights, setRights] = useState(false);
  const start = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/media/imports", {
        params: { path: { orgID: project.organisation_id, projectID: project.id } },
        body: { url: link.trim(), rights_confirmed: rights },
      });
      if (!data) raise(error, response.status, "Could not start the import.");
      return data;
    },
    onSuccess: (asset) => {
      qc.invalidateQueries({ queryKey: ["project", project.id, "media"] });
      setLink("");
      setRights(false);
      onDone(asset);
    },
  });

  return (
    <form
      aria-label="Import a video from a link"
      onSubmit={(e) => {
        e.preventDefault();
        if (link.trim() && rights) start.mutate();
      }}
      className="space-y-2 border-b border-line p-3"
    >
      <label htmlFor={`${id}-url`} className="block text-sm font-semibold">
        Video link <span className="font-semibold text-muted">· YouTube, Twitch or Kick</span>
      </label>
      <input
        id={`${id}-url`}
        type="url"
        inputMode="url"
        value={link}
        onChange={(e) => setLink(e.target.value)}
        placeholder="https://www.youtube.com/watch?v=…"
        maxLength={2048}
        required
        className="block min-h-11 w-full border border-line bg-white px-3 text-base font-medium"
      />
      <label className="flex cursor-pointer items-start gap-2 text-sm font-semibold">
        <input type="checkbox" checked={rights} onChange={(e) => setRights(e.target.checked)} className="mt-0.5 size-5 shrink-0 accent-[var(--ink)]" />
        <span>I own this video or have the creator’s permission to clip it.</span>
      </label>
      <p className="text-xs font-semibold text-muted">
        One video or clip, up to 4 hours, downloaded at up to 1080p. Live streams can be imported once the recording is up.
      </p>
      {start.isError && <Banner>{start.error.message}</Banner>}
      <button
        type="submit"
        disabled={start.isPending || !link.trim() || !rights}
        className="btn btn-primary min-h-11 px-3 text-sm disabled:opacity-60"
      >
        {start.isPending ? "Starting…" : "Import"}
      </button>
    </form>
  );
}
