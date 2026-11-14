"use client";

import { useEffect, useId, useState } from "react";

const shapes = [
  { aspect: "16:9", w: 16, h: 9, use: "YouTube, web" },
  { aspect: "1:1", w: 1, h: 1, use: "Square feed" },
  { aspect: "4:5", w: 4, h: 5, use: "Portrait feed" },
  { aspect: "9:16", w: 9, h: 16, use: "Reels, TikTok, Shorts" },
];

/** Who is talking: the frame sits on them. Positions are % of the scene width. */
const speakers = [30, 70];

/**
 * One wide shot, reframed into each shape around whoever is talking.
 * Cycles on its own until someone picks a shape.
 */
export function ReframeStage() {
  const id = useId();
  const [shape, setShape] = useState(0);
  const [speaker, setSpeaker] = useState(0);
  const [auto, setAuto] = useState(true);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const t = window.setInterval(() => {
      if (auto) setShape((s) => (s + 1) % shapes.length);
      setSpeaker((s) => (s + 1) % speakers.length);
    }, 2600);
    return () => window.clearInterval(t);
  }, [auto]);

  const s = shapes[shape];
  // The scene is 16:9. The frame is the largest box of the chosen shape,
  // centred on the speaker and kept inside the scene.
  const a = s.w / s.h;
  const width = a >= 16 / 9 ? 100 : (a / (16 / 9)) * 100;
  const left = Math.min(Math.max(speakers[speaker] - width / 2, 0), 100 - width);

  return (
    <div>
      <div className="relative aspect-video w-full overflow-hidden border-2 border-current" aria-hidden="true">
        {/* The wide shot: two people at a table, drawn in line. */}
        <svg viewBox="0 0 160 90" className="absolute inset-0 h-full w-full" fill="none" stroke="currentColor" strokeWidth="1.2">
          <path d="M0 70 H160" />
          {speakers.map((x, i) => {
            const cx = (x / 100) * 160;
            const talking = i === speaker;
            return (
              <g key={i}>
                <circle cx={cx} cy="34" r="9" fill={talking ? "currentColor" : "none"} />
                <path d={`M${cx - 15} 70 Q${cx - 15} 48 ${cx} 48 Q${cx + 15} 48 ${cx + 15} 70`} />
                {talking && (
                  <g className="bars" strokeWidth="1.6">
                    {[0, 1, 2].map((j) => (
                      <path key={j} d={`M${cx + 14 + j * 3.5} 24 V32`} style={{ transformBox: "fill-box", transformOrigin: "center", ["--i" as string]: j }} />
                    ))}
                  </g>
                )}
              </g>
            );
          })}
          <rect x="62" y="56" width="36" height="8" />
        </svg>
        {/* Everything outside the frame is cut. */}
        <div
          className="reframe-box absolute inset-y-0 border-2 border-current shadow-[0_0_0_100vmax_color-mix(in_srgb,var(--bg)_78%,transparent)]"
          style={{ width: `${width}%`, left: `${left}%` }}
        >
          <span className="mono absolute top-1.5 left-1.5 bg-[var(--fg)] px-1.5 text-[11px] font-bold text-[var(--bg)] sm:text-xs">
            {s.aspect}
          </span>
          {["top-0 left-0 border-t-4 border-l-4", "top-0 right-0 border-t-4 border-r-4", "bottom-0 left-0 border-b-4 border-l-4", "right-0 bottom-0 border-r-4 border-b-4"].map((c) => (
            <span key={c} className={`absolute size-4 border-current ${c}`} />
          ))}
        </div>
      </div>

      <fieldset className="mt-4">
        <legend className="mono text-xs uppercase opacity-85">Pick a shape</legend>
        <div className="mt-2 grid grid-cols-4 gap-2">
          {shapes.map((sh, i) => (
            <label
              key={sh.aspect}
              className={`flex min-h-14 cursor-pointer flex-col items-center justify-center gap-1 border-2 border-current px-1 transition-colors has-[:focus-visible]:outline-3 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-current ${
                i === shape ? "bg-[var(--fg)] text-[var(--bg)]" : ""
              }`}
            >
              <input
                type="radio"
                name={`${id}-shape`}
                checked={i === shape}
                onChange={() => {
                  setAuto(false);
                  setShape(i);
                }}
                aria-label={`${sh.aspect}, ${sh.use}`}
                className="sr-only"
              />
              <span className="mono text-sm font-bold">{sh.aspect}</span>
            </label>
          ))}
        </div>
      </fieldset>
      <p className="mono mt-2 text-xs uppercase opacity-80">
        {s.use} · {auto ? "Auto-cycling" : "Your pick"} · Illustration
      </p>
    </div>
  );
}
