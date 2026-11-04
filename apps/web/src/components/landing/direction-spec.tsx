import type { CSSProperties } from "react";
import { Character, INK } from "./cartoon";

/**
 * An illustrative brief and the structured creative plan CAVI's Creative
 * Director is being designed to produce from it.
 */

const spec: { key: string; value: string; note?: string }[] = [
  { key: "Format", value: "9:16 · 1080×1920" },
  { key: "Length", value: "45 seconds" },
  { key: "Hook", value: "31:12 → 31:19", note: "“So you’d charge less?”" },
  { key: "Moments", value: "3 · 31:12–31:58", note: "her full answer kept" },
  { key: "Captions", value: "Brand type · word by word" },
  { key: "Audio", value: "Speech only · no music under dialogue" },
];

export function DirectionSpec() {
  return (
    <figure className="grid w-full grid-cols-1 items-start gap-8 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-10">
      {/* Brief from a person */}
      <div className="flex items-end gap-3">
        <svg viewBox="-40 -40 80 104" className="art bob w-20 shrink-0 sm:w-24" aria-hidden="true">
          <Character x={0} y={0} skin="var(--skin-1)" hair="bun" hairColor="#1d1410" shirt="var(--tomato)" mouth="talk" blinkDelay="0.6s" />
        </svg>
        <div className="sticker relative mb-10 rounded-3xl rounded-bl-md bg-white p-5 sm:p-6">
          <p className="mb-2 text-xs font-extrabold tracking-wider text-tomato uppercase">Your brief</p>
          <blockquote className="text-xl leading-snug font-bold sm:text-2xl">
            &ldquo;Turn the second half of this interview into a 45-second vertical cut. Lead with the
            pricing disagreement, keep her full answer, captions in our brand type, no music under
            speech.&rdquo;
          </blockquote>
        </div>
      </div>

      {/* The plan */}
      <div className="sticker-lg rotate-1 rounded-3xl bg-white p-5 sm:p-7">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs font-extrabold tracking-wider uppercase">Creative plan · v1</p>
          <p className="flex items-center gap-1.5 rounded-full border-2 border-ink bg-sun px-3 py-1 text-xs font-extrabold">
            <span aria-hidden className="rec-dot size-2 rounded-full bg-tomato" />
            Waiting for your OK
          </p>
        </div>
        <dl className="mt-5 space-y-2">
          {spec.map((row, i) => (
            <div
              key={row.key}
              style={{ "--i": i } as CSSProperties}
              className="reveal grid grid-cols-[1.75rem_5.5rem_minmax(0,1fr)] items-start gap-2 rounded-2xl border-2 border-ink bg-paper px-3 py-2.5 sm:grid-cols-[1.75rem_7rem_minmax(0,1fr)]"
            >
              <svg viewBox="0 0 24 24" className="mt-0.5 size-6" aria-hidden="true">
                <rect x="2" y="2" width="20" height="20" rx="6" fill="var(--mint)" stroke={INK} strokeWidth="2.5" />
                <path d="M7 12.5 L10.5 16 L17 8.5" fill="none" stroke={INK} strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              <dt className="pt-0.5 text-sm font-extrabold">{row.key}</dt>
              <dd className="min-w-0 pt-0.5 text-sm font-semibold">
                {row.value}
                {row.note && <span className="block text-ink-soft">{row.note}</span>}
              </dd>
            </div>
          ))}
        </dl>
      </div>
      <figcaption className="text-xs font-semibold text-ink-soft lg:col-span-2">
        Illustrative example, not output from a live system
      </figcaption>
    </figure>
  );
}
