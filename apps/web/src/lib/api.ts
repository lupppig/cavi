import { createApiClient, signInUrl } from "@cavi/api-client";

/** Public base URL of the CAVI API (inlined at build time). */
export const apiBaseUrl = process.env.NEXT_PUBLIC_CAVI_API_URL ?? "";

/** Typed API client, or null when the API URL is not configured. */
export const api = apiBaseUrl ? createApiClient(apiBaseUrl) : null;

/** Starts Google sign-in, the only supported method. */
export function signInHref() {
  return apiBaseUrl ? signInUrl(apiBaseUrl) : "#";
}
