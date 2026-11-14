import type { Project } from "./queries";

export function projectHref(p: Pick<Project, "id" | "organisation_id">) {
  return `/account/organisations/${p.organisation_id}/projects/${p.id}`;
}
