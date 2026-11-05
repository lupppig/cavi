"use client";

import { useQuery } from "@tanstack/react-query";
import type { components } from "@cavi/api-client";
import { api } from "./api";

export type Me = components["schemas"]["Me"];
export type Organisation = components["schemas"]["Organisation"];
export type Member = components["schemas"]["Member"];
export type Invitation = components["schemas"]["Invitation"];
export type Role = components["schemas"]["Role"];
export type Project = components["schemas"]["Project"];
export type ProjectAccess = components["schemas"]["ProjectAccess"];
export type MediaAsset = components["schemas"]["MediaAsset"];

export class NotSignedIn extends Error {}
export class NotFound extends Error {}

type ApiError = { error: { message: string; code?: string; fields?: { field: string; message: string }[] } };

/** Turns an API error envelope (or a failed response) into a thrown Error. */
export function raise(error: ApiError | undefined, status: number, fallback: string): never {
  if (status === 401) throw new NotSignedIn("You are not signed in.");
  if (status === 404) throw new NotFound(error?.error.message ?? "Not found.");
  const field = error?.error.fields?.[0]?.message;
  throw new Error(field ?? error?.error.message ?? fallback);
}

export function client() {
  if (!api) throw new Error("The CAVI API is not configured.");
  return api;
}

export const roles: { value: Role; label: string; hint: string }[] = [
  { value: "owner", label: "Owner", hint: "Everything, including billing and owners" },
  { value: "admin", label: "Admin", hint: "Manages people and everything else" },
  { value: "manager", label: "Manager", hint: "Runs workspaces, projects and publishing" },
  { value: "editor", label: "Editor", hint: "Edits, renders and reviews" },
  { value: "creator", label: "Creator", hint: "Creates and edits their projects" },
  { value: "reviewer", label: "Reviewer", hint: "Reviews and approves" },
  { value: "viewer", label: "Viewer", hint: "Can look, not touch" },
  { value: "client", label: "Client", hint: "External; sees only what’s shared" },
];

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: async (): Promise<Me> => {
      const { data, error, response } = await client().GET("/api/v1/me");
      if (!data) raise(error, response.status, "Could not load your account.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotSignedIn) && n < 2,
  });
}

export function useOrganisation(orgID: string) {
  return useQuery({
    queryKey: ["organisation", orgID],
    queryFn: async (): Promise<Organisation> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}", {
        params: { path: { orgID } },
      });
      if (!data) raise(error, response.status, "Could not load this organisation.");
      return data;
    },
    retry: (n, e) => !(e instanceof NotFound) && !(e instanceof NotSignedIn) && n < 2,
  });
}

export function useMembers(orgID: string, enabled = true) {
  return useQuery({
    queryKey: ["organisation", orgID, "members"],
    enabled,
    queryFn: async (): Promise<Member[]> => {
      const { data, error, response } = await client().GET("/api/v1/organisations/{orgID}/members", {
        params: { path: { orgID } },
      });
      if (!data) raise(error, response.status, "Could not load members.");
      return data.data;
    },
  });
}

/** Whether a permission list (organisation or project) includes p. */
export function can(subject: { permissions: string[] }, p: string) {
  return subject.permissions.includes(p);
}

export function useMyInvitations(enabled: boolean) {
  return useQuery({
    queryKey: ["invitations", "mine"],
    enabled,
    queryFn: async (): Promise<Invitation[]> => {
      const { data, error, response } = await client().GET("/api/v1/invitations");
      if (!data) raise(error, response.status, "Could not load your invitations.");
      return data.data;
    },
  });
}

export function displayName(p: { first_name: string; last_name: string; email: string }) {
  return [p.first_name, p.last_name].filter(Boolean).join(" ") || p.email;
}

export function initials(p: { first_name: string; last_name: string; email: string }) {
  const a = p.first_name.trim()[0] ?? p.email[0] ?? "?";
  const b = p.last_name.trim()[0] ?? "";
  return (a + b).toUpperCase();
}
