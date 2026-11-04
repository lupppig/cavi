import createFetchClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type { components, paths };

export type ErrorDetail = components["schemas"]["ErrorDetail"];
export type Discipline = components["schemas"]["Discipline"];
export type Me = components["schemas"]["Me"];
export type OrganisationMembership = components["schemas"]["OrganisationMembership"];

/**
 * Creates a typed client for the CAVI API at baseUrl. Requests include
 * credentials so the HttpOnly session cookie is sent cross-origin.
 */
export function createApiClient(baseUrl: string) {
  return createFetchClient<paths>({ baseUrl, credentials: "include" });
}

/** URL that starts browser sign-in (a navigation, not a fetch). */
export function signInUrl(baseUrl: string, screen: "sign-in" | "sign-up" = "sign-in") {
  return `${baseUrl.replace(/\/$/, "")}/api/v1/auth/login?screen=${screen}`;
}

export type ApiClient = ReturnType<typeof createApiClient>;
