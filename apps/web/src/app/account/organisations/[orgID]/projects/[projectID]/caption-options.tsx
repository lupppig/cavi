"use client";

import { Inter } from "next/font/google";
import { useId } from "react";
import type { CaptionOptions, Transcript } from "@/lib/queries";

// The caption font the render worker burns in (Inter Black).
const inter = Inter({ subsets: ["latin"], weight: "900" });

type Colors = NonNullable<CaptionOptions["colors"]>;

/** Ten ready-made colour combinations: text, the highlighted word, and the outline or box. */
export const colorPresets: { name: string; colors: Colors }[] = [
  { name: "Classic", colors: { text: "#FFFFFF", highlight: "#FFE600", stroke: "#000000" } },
  { name: "Yellow", colors: { text: "#FFE600", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Green", colors: { text: "#39FF14", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Blue", colors: { text: "#3D9BFF", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Orange", colors: { text: "#FF8A00", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Red", colors: { text: "#FF3B30", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Pink", colors: { text: "#FF5FCF", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Purple", colors: { text: "#A970FF", highlight: "#FFFFFF", stroke: "#000000" } },
  { name: "Brown", colors: { text: "#FFFFFF", highlight: "#FFD08A", stroke: "#6B3E1F" } },
  { name: "Ink", colors: { text: "#000000", highlight: "#FF3B30", stroke: "#FFFFFF" } },
];

/** Colours offered per speaker: the presets' text colours. */
const speakerSwatches = colorPresets.map((p) => p.colors.text).filter((c, i, all) => all.indexOf(c) === i);

export const defaultCaptionOptions: CaptionOptions = {
  style: "bold",
  mode: "lines",
  position: "bottom",
  uppercase: false,
  colors: colorPresets[0].colors,
};

const voiceNames: Record<string, string> = { woman: "woman’s voice", man: "man’s voice", unclear: "voice unclear" };

/**
 * Caption size and placement in output pixels. Mirrors layoutFor in the
 * render worker (internal/render/captions.go) so the preview matches.
 */
export function captionLayout(outW: number, outH: number, mode: string, position: string, style = "") {
  const size = Math.round(Math.min(outW, outH) * (style === "pop" ? 0.085 : mode === "words" ? 0.095 : 0.075));
  const vertical = outH > outW;
  const marginV =
    position === "middle" ? 0 : Math.round(outH * (position === "top" ? (vertical ? 0.14 : 0.08) : vertical ? 0.22 : 0.08));
  return {
    size,
    marginV,
    maxChars: Math.floor((outW * 0.84) / (size * 0.62)),
    outline: Math.round(size * 0.09),
    pad: Math.round(size * 0.2),
    side: Math.floor(outW / 12),
  };
}

/**
 * The first caption the clip will show, as lines of words: the words of
 * the first spoken segment that fall inside the clip (times estimated by
 * length, as the worker does when it has no word times), wrapped and
 * grouped like the worker's chunk().
 */
export function firstCaption(
  segment: Transcript["segments"][number],
  clipStartMS: number,
  maxChars: number,
  mode: string,
): string[][] {
  const fields = segment.text.split(/\s+/).filter(Boolean);
  const total = fields.reduce((n, f) => n + f.length + 1, 0);
  let at = segment.start_ms;
  const span = segment.end_ms - segment.start_ms;
  const inside: string[] = [];
  for (const f of fields) {
    const d = (span * (f.length + 1)) / Math.max(total, 1);
    if (at + d / 2 >= clipStartMS) inside.push(f);
    at += d;
  }
  const lines: string[][] = [];
  let line: string[] = [];
  let len = 0;
  for (const w of inside) {
    if (mode === "words" && lines.flat().length + line.length === 3) break;
    if (line.length && len + 1 + w.length > maxChars) {
      lines.push(line);
      if (lines.length === 2) return lines;
      line = [];
      len = 0;
    }
    len += (line.length ? 1 : 0) + w.length;
    line.push(w);
  }
  if (line.length) lines.push(line);
  return lines;
}

/**
 * The spoken word's colour over text of colour base: the chosen highlight,
 * or, when that would be invisible against the text, the caption's text
 * colour, yellow or green. Mirrors highlightFor in the worker.
 */
export function highlightFor(base: string, colors: Colors) {
  for (const c of [colors.highlight, colors.text, "#FFE600", "#39FF14"]) {
    if (c.toLowerCase() !== base.toLowerCase()) return c;
  }
  return colors.highlight;
}

/** The first caption drawn over the framing preview, at the worker's size and place. */
export function CaptionPreview({
  options,
  lines,
  speaker,
  outW,
  outH,
}: {
  options: CaptionOptions;
  lines: string[][];
  speaker: string;
  outW: number;
  outH: number;
}) {
  const pop = options.style === "pop";
  const lay = captionLayout(outW, outH, options.mode, options.position, options.style);
  const colors = options.colors ?? colorPresets[0].colors;
  const base = options.speaker_colors?.[speaker] ?? colors.text;
  const px = (n: number) => `${(n / outW) * 100}cqw`;
  const place =
    options.position === "middle"
      ? { top: "50%", transform: "translateY(-50%)" }
      : options.position === "top"
        ? { top: `${(lay.marginV / outH) * 100}%` }
        : { bottom: `${(lay.marginV / outH) * 100}%` };
  let k = 0;
  return (
    <div
      aria-hidden="true"
      className={`${inter.className} pointer-events-none absolute inset-x-0 text-center leading-[1.2]`}
      style={{
        ...place,
        paddingInline: px(lay.side),
        fontSize: px(lay.size),
        fontWeight: 900,
        letterSpacing: "normal",
        textTransform: options.uppercase || pop ? "uppercase" : "none",
      }}
    >
      {lines.map((line, i) => (
        <div key={i}>
          <span
            style={
              options.style === "boxed"
                ? { background: colors.stroke, padding: `0 ${px(lay.pad)}`, boxDecorationBreak: "clone" }
                : pop
                  ? {
                      WebkitTextStroke: `${px(Math.round(lay.size * 0.12) * 2)} ${colors.stroke}`,
                      paintOrder: "stroke fill",
                      textShadow: `${px(Math.round(lay.size * 0.07))} ${px(Math.round(lay.size * 0.07))} 0 rgb(0 0 0 / 0.5)`,
                    }
                  : { WebkitTextStroke: `${px(lay.outline * 2)} ${colors.stroke}`, paintOrder: "stroke fill" }
            }
          >
            {line.map((w, j) => {
              const lit = (options.mode === "words" || pop) && k++ === 0;
              return (
                <span key={j}>
                  {j > 0 && " "}
                  <span
                    style={{
                      color: lit ? highlightFor(base, colors) : base,
                      display: "inline-block",
                      transform: lit ? (pop ? "scale(1.12)" : "scale(1.1)") : undefined,
                    }}
                  >
                    {w}
                  </span>
                </span>
              );
            })}
          </span>
        </div>
      ))}
    </div>
  );
}

function Swatch({ colors, label }: { colors: Colors; label: string }) {
  return (
    <span aria-hidden="true" className="flex size-7 shrink-0 items-center justify-center border border-line" style={{ background: colors.stroke }} title={label}>
      <span className={`${inter.className} text-sm font-black`} style={{ color: colors.text }}>
        A
      </span>
    </span>
  );
}

function Segmented<T extends string>({
  legend,
  value,
  options,
  onChange,
}: {
  legend: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (v: T) => void;
}) {
  const id = useId();
  return (
    <fieldset>
      <legend className="text-xs font-semibold uppercase text-muted">{legend}</legend>
      {/* Three choices stack when the composer is very narrow. */}
      <div className={`mt-1 grid gap-1.5 ${options.length > 2 ? "grid-cols-1 @[14rem]:grid-cols-3" : "grid-cols-2"}`}>
        {options.map((o) => (
          <label
            key={o.value}
            className={`flex min-h-11 min-w-0 cursor-pointer items-center justify-center border border-line px-1 text-center text-sm font-semibold has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${value === o.value ? "bg-accent-soft text-accent-text" : "bg-white"}`}
          >
            <input type="radio" name={id} checked={value === o.value} onChange={() => onChange(o.value)} className="sr-only" />
            {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/** The caption choices for one clip: look, timing, colours, place and per-speaker colours. */
export function CaptionOptionsPicker({
  value,
  onChange,
  speakers,
}: {
  value: CaptionOptions;
  onChange: (v: CaptionOptions) => void;
  /** Speakers heard in the clip, with how their voice sounds. */
  speakers: Transcript["speakers"];
}) {
  const id = useId();
  const colors = value.colors ?? colorPresets[0].colors;
  const preset = colorPresets.findIndex(
    (p) => p.colors.text === colors.text && p.colors.highlight === colors.highlight && p.colors.stroke === colors.stroke,
  );
  const set = (patch: Partial<CaptionOptions>) => onChange({ ...value, ...patch });
  const bySpeaker = !!value.speaker_colors && Object.keys(value.speaker_colors).length > 0;

  return (
    <div className="space-y-3 border border-line p-2.5">
      <Segmented
        legend="Look"
        value={value.style}
        options={[
          { value: "bold", label: "Outline" },
          { value: "boxed", label: "Box" },
          { value: "pop", label: "Pop" },
        ]}
        // Pop is always word by word in capitals.
        onChange={(style) => set(style === "pop" ? { style, mode: "words", uppercase: true } : { style })}
      />
      {value.style === "pop" ? (
        <p className="text-xs font-semibold text-muted">Pop: a few words at a time in capitals, each word bouncing in as it’s said.</p>
      ) : (
        <Segmented
          legend="Timing"
          value={value.mode}
          options={[
            { value: "lines", label: "Lines" },
            { value: "words", label: "Word by word" },
          ]}
          onChange={(mode) => set({ mode })}
        />
      )}
      <fieldset>
        <legend className="text-xs font-semibold uppercase text-muted">
          Colours <span className="font-semibold normal-case">· {preset >= 0 ? colorPresets[preset].name : "Custom"}</span>
        </legend>
        <div className="mt-1 grid grid-cols-3 gap-1.5 @[13rem]:grid-cols-5 @[32rem]:grid-cols-10">
          {colorPresets.map((p, i) => (
            <label
              key={p.name}
              className={`flex min-h-11 min-w-11 cursor-pointer items-center justify-center border has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 ${
                preset === i ? "rounded-lg border-accent bg-accent-soft" : "rounded-lg border-transparent"
              }`}
            >
              <input type="radio" name={`${id}-preset`} checked={preset === i} onChange={() => set({ colors: p.colors })} aria-label={p.name} className="sr-only" />
              <Swatch colors={p.colors} label={p.name} />
            </label>
          ))}
        </div>
        <div className="mt-2 grid grid-cols-1 gap-1.5 @[30rem]:grid-cols-3">
          {(
            [
              ["text", "Text"],
              ["highlight", "Highlight"],
              ["stroke", value.style === "boxed" ? "Box" : "Outline"],
            ] as const
          ).map(([k, label]) => (
            <label key={k} className="flex min-h-11 min-w-0 cursor-pointer items-center gap-1.5 border border-line bg-white px-1.5 text-xs font-semibold">
              <input
                type="color"
                value={colors[k].toLowerCase()}
                onChange={(e) => set({ colors: { ...colors, [k]: e.target.value.toUpperCase() } })}
                className="size-6 shrink-0 cursor-pointer border-0 bg-transparent p-0"
              />
              <span className="min-w-0 truncate">{label}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <Segmented
        legend="Position"
        value={value.position}
        options={[
          { value: "bottom", label: "Bottom" },
          { value: "middle", label: "Middle" },
          { value: "top", label: "Top" },
        ]}
        onChange={(position) => set({ position })}
      />
      {value.style !== "pop" && (
        <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
          <input type="checkbox" checked={value.uppercase} onChange={(e) => set({ uppercase: e.target.checked })} className="size-5 accent-[var(--ink)]" />
          ALL CAPS
        </label>
      )}
      {speakers.length > 1 && (
        <div>
          <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-semibold">
            <input
              type="checkbox"
              checked={bySpeaker}
              onChange={(e) =>
                set({
                  // The first speaker keeps the chosen text colour; others get
                  // distinct preset colours that aren't the highlight.
                  speaker_colors: e.target.checked
                    ? Object.fromEntries(
                        speakers.map((s, i) => {
                          const others = speakerSwatches.filter(
                            (c) => c !== colors.text && c.toLowerCase() !== colors.highlight.toLowerCase(),
                          );
                          return [s.label, i === 0 ? colors.text : others[(i - 1) % others.length]];
                        }),
                      )
                    : undefined,
                })
              }
              className="size-5 accent-[var(--ink)]"
            />
            Colour by speaker
          </label>
          {bySpeaker && (
            <ul className="mt-1 space-y-1.5">
              {speakers.map((s) => (
                <li key={s.label} className="flex flex-wrap items-center gap-2">
                  <span className="min-w-0 text-sm font-semibold">
                    {s.label} <span className="font-semibold text-muted">· {voiceNames[s.voice]}</span>
                  </span>
                  <label className="ml-auto flex min-h-11 cursor-pointer items-center gap-1.5 border border-line bg-white px-1.5 text-xs font-semibold">
                    <input
                      type="color"
                      value={(value.speaker_colors?.[s.label] ?? colors.text).toLowerCase()}
                      onChange={(e) => set({ speaker_colors: { ...value.speaker_colors, [s.label]: e.target.value.toUpperCase() } })}
                      aria-label={`Caption colour for ${s.label}`}
                      className="size-6 cursor-pointer border-0 bg-transparent p-0"
                    />
                    Colour
                  </label>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
