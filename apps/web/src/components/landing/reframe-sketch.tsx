import type { CSSProperties } from "react";
import { Character, INK, MouthShape, Sparkle } from "./cartoon";

/**
 * Cartoon reframing: one 16:9 podcast shot, cropped to 1:1, 4:5 and 9:16
 * while following whoever is talking. Pure CSS; see `reframe` and the
 * `mouth-*` rules in globals.css.
 */

const formats = [
  { ratio: "16:9", note: "the whole set" },
  { ratio: "1:1", note: "square, on the guest" },
  { ratio: "4:5", note: "portrait, on the guest" },
  { ratio: "9:16", note: "vertical, follows the host" },
];

function Speaker({ x, who }: { x: number; who: "a" | "b" }) {
  const a = who === "a";
  return (
    <g>
      <Character
        x={x}
        y={88}
        scale={1.15}
        skin={a ? "var(--skin-1)" : "var(--skin-2)"}
        hair={a ? "afro" : "bob"}
        hairColor={a ? "#1d1410" : "#d9534f"}
        shirt={a ? "var(--sun)" : "var(--mint)"}
        mouth="smile"
        mouthClass={a ? "mouth-a-still" : "mouth-b-still"}
        blinkDelay={a ? "0.2s" : "1.9s"}
      />
      <g transform={`translate(${x} 88) scale(1.15)`}>
        <MouthShape mouth="talk" className={a ? "mouth-a-talk" : "mouth-b-talk"} />
      </g>
      {/* Mic */}
      <g transform={`translate(${x + (a ? 22 : -22)} 0)`}>
        <path d="M0 150 V128" stroke={INK} strokeWidth="4" strokeLinecap="round" />
        <rect x="-7" y="108" width="14" height="24" rx="7" fill={INK} />
      </g>
    </g>
  );
}

export function ReframeSketch() {
  return (
    <figure className="w-full">
      <div
        role="img"
        aria-label="Cartoon illustration: a two-person podcast shot reframed to square, portrait and vertical, following whoever is talking."
        className="sticker-lg relative aspect-video w-full overflow-hidden rounded-3xl bg-paper-deep"
      >
        <svg aria-hidden="true" viewBox="0 0 320 180" className="art absolute inset-0 h-full w-full" preserveAspectRatio="xMidYMid slice">
          {/* Set */}
          <rect x="0" y="0" width="320" height="180" fill="var(--paper-deep)" />
          <rect x="20" y="18" width="70" height="44" rx="6" fill="var(--sky)" stroke={INK} strokeWidth="3" />
          <path d="M20 46 L42 32 L58 44 L72 36 L90 48 V62 H20 Z" fill="var(--mint)" stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
          <g transform="translate(160 24)">
            <rect x="-38" y="-12" width="76" height="24" rx="12" fill="var(--tomato)" stroke={INK} strokeWidth="3" />
            <circle className="rec-dot" cx="-24" cy="0" r="4" fill="white" />
            <text x="6" y="5" fontSize="12" fontWeight={800} textAnchor="middle" fill="white" className="font-sans">
              ON AIR
            </text>
          </g>
          <rect x="236" y="56" width="62" height="6" rx="3" fill={INK} />
          <rect x="244" y="34" width="12" height="22" rx="2" fill="var(--pink)" stroke={INK} strokeWidth="2" />
          <rect x="258" y="28" width="12" height="28" rx="2" fill="var(--sun)" stroke={INK} strokeWidth="2" />
          <rect x="272" y="38" width="16" height="18" rx="2" fill="var(--cobalt)" stroke={INK} strokeWidth="2" />
          <Sparkle x={300} y={22} size={7} fill="var(--sun)" className="spin-slow" />

          <Speaker x={102} who="a" />
          <Speaker x={218} who="b" />

          {/* Desk */}
          <rect x="40" y="146" width="240" height="40" rx="10" fill="var(--cobalt)" stroke={INK} strokeWidth="3.5" />
          <rect x="132" y="154" width="56" height="12" rx="6" fill="white" stroke={INK} strokeWidth="2.5" />
        </svg>

        {/* Moving crop frame; outside is tinted */}
        <div aria-hidden="true" className="reframe absolute inset-y-0 rounded-xl border-[4px] border-white shadow-[0_0_0_200vmax_rgb(20_20_20/0.55)]">
          <span className="absolute top-2 left-2 flex items-center gap-1 rounded-full border-2 border-ink bg-tomato px-1.5 py-0.5 text-[9px] font-extrabold text-white sm:text-[10px]">
            <span className="rec-dot size-1.5 rounded-full bg-white" /> REC
          </span>
        </div>
      </div>

      <div aria-hidden="true" className="relative mt-5 h-9">
        {formats.map((f, k) => (
          <p
            key={f.ratio}
            className="reframe-label absolute inset-0 flex items-center gap-3"
            style={{ "--delay": `${-((4 - k) % 4) * 3.5}s` } as CSSProperties}
          >
            <span className="sticker rounded-full bg-sun px-3 py-1 text-lg font-extrabold text-ink">{f.ratio}</span>
            <span className="text-lg font-bold text-white">{f.note}</span>
          </p>
        ))}
      </div>
      <figcaption className="mt-2 text-xs font-semibold text-white/80">
        Illustration of what we&rsquo;re building, not a screenshot
      </figcaption>
    </figure>
  );
}
