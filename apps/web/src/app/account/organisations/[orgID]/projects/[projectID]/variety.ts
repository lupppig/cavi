import type { CaptionOptions } from "@/lib/queries";

/**
 * Variety across a batch: each clip keeps the channel's look (caption
 * style, text colour, outline) but varies the details a viewer notices
 * from clip to clip, so a run of uploads doesn't look stamped from one
 * template. Rotations continue from offset (clips already made), so the
 * next batch doesn't start where the last one did.
 */

const highlights = [
  { name: "yellow", hex: "#FFE600" },
  { name: "green", hex: "#39FF14" },
  { name: "blue", hex: "#3D9BFF" },
  { name: "orange", hex: "#FF8A00" },
  { name: "pink", hex: "#FF5FCF" },
  { name: "purple", hex: "#A970FF" },
];
// Mostly at the bottom, where short-form captions sit; sometimes the middle.
const positions = ["bottom", "bottom", "middle"] as const;
const hookMS = [3000, 2500, 3500, 2800];
const signOffs = ["Follow for more!", "Follow for daily clips", "Want more like this? Follow", "Subscribe for the next one"];

export type Variant = { captions: CaptionOptions | null; hookMS: number; signOff: string; describe: string };

/** The i-th clip's variant of the look. */
export function variant(i: number, offset: number, captions: CaptionOptions | null, signOff: string): Variant {
  const n = i + offset;
  let c = captions;
  const parts: string[] = [];
  if (captions) {
    const text = (captions.colors?.text ?? "#FFFFFF").toUpperCase();
    const pick = highlights.filter((h) => h.hex !== text);
    const h = pick[n % pick.length];
    const position = positions[n % positions.length];
    c = { ...captions, position, colors: { text, highlight: h.hex, stroke: captions.colors?.stroke ?? "#000000" } };
    parts.push(`${h.name} highlight`, `captions ${position === "middle" ? "in the middle" : "at the bottom"}`);
  }
  // The person's own sign-off comes first in the rotation.
  const choices = [signOff, ...signOffs.filter((s) => s !== signOff)].filter(Boolean);
  const so = signOff ? choices[n % choices.length] : "";
  if (so) parts.push(`“${so}”`);
  return { captions: c, hookMS: hookMS[n % hookMS.length], signOff: so, describe: parts.join(", ") };
}
