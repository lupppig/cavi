"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useId } from "react";
import { Banner } from "@/components/account/account-header";
import { can, client, raise, type Organisation } from "@/lib/queries";

const busy = (s: string | null | undefined) => s === "queued" || s === "running";

function useConnection(orgID: string) {
  return useQuery({
    queryKey: ["organisation", orgID, "youtube"],
    queryFn: async () => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/youtube", { params: { path: { orgID } } });
      if (!data) raise(error, response.status, "Could not load the YouTube connection.");
      return data;
    },
    refetchInterval: (q) => (busy(q.state.data?.sync_status) ? 3000 : false),
  });
}

function useVideos(orgID: string, enabled: boolean, syncedAt: string | null | undefined) {
  return useQuery({
    queryKey: ["organisation", orgID, "youtube", "videos", syncedAt],
    enabled,
    queryFn: async () => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/youtube/videos", { params: { path: { orgID } } });
      if (!data) raise(error, response.status, "Could not load your videos.");
      return data.data;
    },
  });
}

const fmt = (n: number) => new Intl.NumberFormat(undefined, { notation: n >= 10_000 ? "compact" : "standard" }).format(n);
const secs = (ms: number) => `${Math.round(ms / 1000)}s`;

/**
 * The organisation's YouTube channel: connect it (read-only), and see how
 * each upload did, matched to the clip it was made from.
 */
export function Channel({ org }: { org: Organisation }) {
  const id = useId();
  const qc = useQueryClient();
  const search = useSearchParams();
  const conn = useConnection(org.id);
  const c = conn.data;
  const videos = useVideos(org.id, !!c?.connected, c?.synced_at);
  const manage = can(org, "organisation.manage");
  const key = ["organisation", org.id, "youtube"];

  const connect = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/youtube/connect", { params: { path: { orgID: org.id } } });
      if (!data) raise(error, response.status, "Could not start connecting YouTube.");
      window.location.assign(data.url);
    },
  });
  const sync = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await client().POST("/api/v1/organisations/{orgID}/youtube/sync", { params: { path: { orgID: org.id } } });
      if (!data) raise(error, response.status, "Could not start syncing.");
      return data;
    },
    onSuccess: (d) => qc.setQueryData(key, d),
  });
  const disconnect = useMutation({
    mutationFn: async () => {
      const { error, response } = await client().DELETE("/api/v1/organisations/{orgID}/youtube", { params: { path: { orgID: org.id } } });
      if (!response.ok) raise(error, response.status, "Could not disconnect.");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  const returned = search.get("youtube");
  const rows = videos.data ?? [];
  const watched = rows.filter((v) => v.views >= 50);
  const best = [...watched].sort((a, b) => b.avg_view_percentage - a.avg_view_percentage)[0];

  return (
    <section aria-labelledby={`${id}-h`} className="card p-5 sm:p-6">
      <h2 id={`${id}-h`} className="text-2xl font-semibold tracking-tight">YouTube channel</h2>
      <p className="mt-1 font-medium text-muted">
        Connect your channel so CAVI can see which clips people watched to the end and which won subscribers, and plan more like
        them. CAVI can only read: it can’t upload, edit or delete anything.
      </p>
      {returned === "failed" && <div className="mt-4"><Banner>Connecting YouTube didn’t finish. Try again, and allow both permissions Google asks for.</Banner></div>}
      {returned === "connected" && c?.connected && (
        <p className="mt-4 rounded-xl border border-accent-line bg-accent-soft p-3 text-sm font-semibold" role="status">
          Connected {c.channel_title}. Its numbers are being read now.
        </p>
      )}
      {conn.isPending && <div className="mt-4 h-20 animate-pulse bg-surface-2" />}
      {conn.isError && <div className="mt-4"><Banner>{conn.error.message}</Banner></div>}
      {[connect, sync, disconnect].map((m, i) => m.isError && <div key={i} className="mt-4"><Banner>{m.error.message}</Banner></div>)}

      {c && !c.available && <p className="mt-4 text-sm font-semibold text-muted">Connecting YouTube isn’t set up on this server yet.</p>}

      {c?.available && !c.connected && (
        <div className="mt-4">
          {manage ? (
            <button type="button" onClick={() => connect.mutate()} disabled={connect.isPending} className="btn btn-primary min-h-11 px-4 disabled:opacity-60">
              {connect.isPending ? "Opening Google…" : "Connect YouTube"}
            </button>
          ) : (
            <p className="text-sm font-semibold text-muted">An owner or admin can connect your channel.</p>
          )}
        </div>
      )}

      {c?.connected && (
        <div className="mt-4 space-y-4">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <p className="mr-auto min-w-0 font-semibold break-words">
              {c.channel_title}
              <span className="block text-xs font-semibold text-muted">
                {busy(c.sync_status)
                  ? "Reading your numbers…"
                  : c.synced_at
                    ? `${c.videos} videos · updated ${new Date(c.synced_at).toLocaleString()} · YouTube’s numbers lag a day or two`
                    : "Not read yet"}
              </span>
            </p>
            <button type="button" onClick={() => sync.mutate()} disabled={sync.isPending || busy(c.sync_status)} className="btn btn-secondary min-h-11 px-3 text-sm disabled:opacity-60">
              Refresh numbers
            </button>
            {manage && (
              <button
                type="button"
                onClick={() => window.confirm(`Disconnect ${c.channel_title}? Its numbers are removed from CAVI.`) && disconnect.mutate()}
                disabled={disconnect.isPending}
                className="btn btn-ghost min-h-11 px-3 text-sm disabled:opacity-60"
              >
                Disconnect
              </button>
            )}
          </div>
          {c.sync_status === "failed" && (
            <div className="space-y-2">
              <Banner>{c.sync_error ?? "Reading your numbers failed."}</Banner>
              {manage && (
                <button type="button" onClick={() => connect.mutate()} className="btn btn-secondary min-h-11 px-3 text-sm">
                  Connect again
                </button>
              )}
            </div>
          )}
          {best && (
            <p className="text-sm font-semibold">
              Best at holding viewers: “{best.title}”, watched {Math.round(best.avg_view_percentage)}% through on average.
            </p>
          )}
          {videos.isError && <Banner>{videos.error.message}</Banner>}
          {rows.length > 0 && (
            <div className="overflow-x-auto rounded-xl border border-line">
              <table className="w-full min-w-[40rem] text-left text-sm">
                <thead className="bg-surface-2 text-xs font-semibold text-muted">
                  <tr>
                    <th scope="col" className="px-3 py-2">Video</th>
                    <th scope="col" className="px-3 py-2 text-right">Views</th>
                    <th scope="col" className="px-3 py-2 text-right">Watched</th>
                    <th scope="col" className="px-3 py-2 text-right">Avg. view</th>
                    <th scope="col" className="px-3 py-2 text-right">Subs</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((v) => (
                    <tr key={v.video_id} className="border-t border-line align-top">
                      <td className="px-3 py-2">
                        <a href={`https://www.youtube.com/watch?v=${v.video_id}`} target="_blank" rel="noreferrer" className="font-semibold break-words underline decoration-line underline-offset-4">
                          {v.title}
                        </a>
                        <span className="block font-mono text-xs text-muted">
                          {secs(v.duration_ms)}
                          {v.published_at ? ` · ${new Date(v.published_at).toLocaleDateString()}` : ""}
                          {v.clip ? (
                            <>
                              {" · from "}
                              <Link href={`/account/organisations/${org.id}/projects/${v.clip.project_id}`} className="underline underline-offset-2">
                                {v.clip.name}
                              </Link>
                            </>
                          ) : null}
                        </span>
                      </td>
                      <td className="px-3 py-2 text-right font-mono">{fmt(v.views)}</td>
                      <td className="px-3 py-2 text-right font-mono">{v.views ? `${Math.round(v.avg_view_percentage)}%` : "–"}</td>
                      <td className="px-3 py-2 text-right font-mono">{v.views ? `${v.avg_view_seconds.toFixed(0)}s` : "–"}</td>
                      <td className="px-3 py-2 text-right font-mono">{v.subscribers_gained}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
