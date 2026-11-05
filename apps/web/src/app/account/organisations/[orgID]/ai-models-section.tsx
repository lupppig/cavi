"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Banner } from "@/components/account/account-header";
import { client, raise, type Organisation } from "@/lib/queries";

const taskNames: Record<string, { title: string; hint: string }> = {
  transcription: {
    title: "Transcription",
    hint: "Turns speech in your videos and audio into timed text.",
  },
};

/** Which AI model this organisation uses for each task. Owners and admins can change it. */
export function AIModels({ org }: { org: Organisation }) {
  const id = useId();
  const qc = useQueryClient();
  const key = ["organisation", org.id, "ai-models"];
  const settings = useQuery({
    queryKey: key,
    queryFn: async () => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/ai-models", {
        params: { path: { orgID: org.id } },
      });
      if (!data) raise(error, response.status, "Could not load AI settings.");
      return data.data;
    },
  });
  const save = useMutation({
    mutationFn: async ({ task, value }: { task: string; value: string }) => {
      const [provider, model] = value ? value.split("/") : ["", ""];
      const { error, response } = await client().PUT("/api/v1/organisations/{orgID}/ai-models/{task}", {
        params: { path: { orgID: org.id, task: task as "transcription" } },
        body: { provider, model },
      });
      if (!response.ok) raise(error, response.status, "Could not save the model.");
    },
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });

  return (
    <section aria-labelledby={`${id}-h`} className="sticker-lg rounded-[28px] bg-lime p-2">
      <div className="rounded-[20px] border-[3px] border-ink bg-white p-5 sm:p-6">
        <h2 id={`${id}-h`} className="text-2xl font-extrabold tracking-tight">AI models</h2>
        <p className="mt-1 font-medium text-ink-soft">
          Choose which model CAVI uses for this {org.personal ? "studio" : "team"}. When someone uses an AI feature, the
          relevant media is sent to that model’s provider. Work already started keeps the model it began with.
        </p>
        {save.isError && <div className="mt-4"><Banner>{save.error.message}</Banner></div>}
        {settings.isPending && <div className="mt-4 h-14 animate-pulse rounded-2xl bg-paper-deep" />}
        {settings.isError && <div className="mt-4"><Banner>{settings.error.message}</Banner></div>}
        {settings.data && (
          <ul className="mt-4 space-y-4">
            {settings.data.map((s) => {
              const name = taskNames[s.task] ?? { title: s.task, hint: "" };
              const current = s.custom ? `${s.model.provider}/${s.model.id}` : "";
              const busy = save.isPending && save.variables?.task === s.task;
              return (
                <li key={s.task} className="flex flex-col gap-3 sm:flex-row sm:items-center">
                  <div className="min-w-0 flex-1">
                    <label htmlFor={`${id}-${s.task}`} className="font-extrabold">{name.title}</label>
                    <p className="text-sm font-medium text-ink-soft">{name.hint}</p>
                  </div>
                  <select
                    id={`${id}-${s.task}`}
                    value={current}
                    disabled={busy}
                    onChange={(e) => save.mutate({ task: s.task, value: e.target.value })}
                    className="min-h-12 w-full rounded-2xl border-[3px] border-ink bg-paper px-3 font-bold disabled:opacity-60 sm:w-auto"
                  >
                    <option value="">
                      Platform default ({s.options.find((o) => o.default)?.label ?? "recommended"})
                    </option>
                    {s.options.map((o) => (
                      <option key={o.id} value={`${o.provider}/${o.id}`}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}
