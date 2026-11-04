import createFetchClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type { components, paths };

export type ErrorDetail = components["schemas"]["ErrorDetail"];
export type Discipline = components["schemas"]["Discipline"];

/** Creates a typed client for the CAVI API at baseUrl. */
export function createApiClient(baseUrl: string) {
  return createFetchClient<paths>({ baseUrl });
}

export type ApiClient = ReturnType<typeof createApiClient>;
