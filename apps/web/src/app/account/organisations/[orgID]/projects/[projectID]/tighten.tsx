"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Banner } from "@/components/account/account-header";
import { client, raise, type MediaAsset, type Project } from "@/lib/queries";

/** Files whose pauses must be found before clips from them can cut them. */
export const needsPauses = (files: MediaAsset[]) => files.filter((f) => !f.pauses_found && !!f.metadata?.audio_codec);

/**
 * Videos processed before pause cutting: says so, and processes them
 * again to find their pauses (a minute or two each).
 */
export function PrepareForPauses({ project, files }: { project: Project; files: MediaAsset[] }) {
  const qc = useQueryClient();
  const prepare = useMutation({
    mutationFn: async () => {
      for (const f of files) {
        const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/projects/{projectID}/media/{assetID}/process", {
          params: { path: { orgID: project.organisation_id, projectID: project.id, assetID: f.id } },
        });
        if (!data) raise(error, response.status, `Could not prepare ${f.filename}.`);
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["project", project.id, "media"] }),
  });
  const busy = files.some((f) => f.processing.status === "pending" || f.processing.status === "processing");
  return (
    <div className="space-y-1.5 border border-line bg-white p-2 text-xs font-semibold">
      {prepare.isError && <Banner>{prepare.error.message}</Banner>}
      {busy || prepare.isSuccess ? (
        <p className="flex items-center gap-2" role="status">
          <span aria-hidden="true" className="size-2 animate-pulse rounded-full bg-accent" />
          Finding the pauses… (a minute or two)
        </p>
      ) : (
        <>
          <p>
            {files.map((f) => f.filename).join(", ")} {files.length === 1 ? "was" : "were"} processed before pause cutting. Prepare{" "}
            {files.length === 1 ? "it" : "them"} first, or untick Cut pauses.
          </p>
          <button
            type="button"
            onClick={() => prepare.mutate()}
            disabled={prepare.isPending}
            className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60"
          >
            {prepare.isPending ? "Starting…" : "Prepare"}
          </button>
        </>
      )}
    </div>
  );
}
