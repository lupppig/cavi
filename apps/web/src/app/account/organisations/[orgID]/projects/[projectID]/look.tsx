"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { client, raise, type CaptionOptions } from "@/lib/queries";
import type { components } from "@cavi/api-client";

export type Look = components["schemas"]["ClipLook"];
export type SavedLook = components["schemas"]["ClipLookResponse"];

/** The organisation's look: the house style its clips start from. */
export function useLook(orgID: string) {
  return useQuery({
    queryKey: ["organisation", orgID, "clip-look"],
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/clip-look", { params: { path: { orgID } } });
      if (!data) raise(error, response.status, "Could not load your look.");
      return data;
    },
  });
}

export function useSaveLook(orgID: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (look: Look) => {
      const { data, error, response } = await client().PUT("/api/v1/organisations/{orgID}/clip-look", {
        params: { path: { orgID } },
        body: look,
      });
      if (!data) raise(error, response.status, "Could not save your look.");
      return data;
    },
    onSuccess: (l) => qc.setQueryData(["organisation", orgID, "clip-look"], l),
  });
}

/** Caption options from a look, ready for a render request. */
export function lookCaptions(l: Look | undefined): CaptionOptions | null {
  return l?.captions ? { ...l.captions } : null;
}
