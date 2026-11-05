import type { Project } from "./queries";

const colours = ["bg-sun", "bg-mint", "bg-pink", "bg-sky", "bg-lime", "bg-tomato"];

/** A stable accent colour for a project, so cards are easy to tell apart. */
export function projectColour(id: string) {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) | 0;
  return colours[Math.abs(h) % colours.length];
}

export function projectHref(p: Pick<Project, "id" | "organisation_id">) {
  return `/account/organisations/${p.organisation_id}/projects/${p.id}`;
}
