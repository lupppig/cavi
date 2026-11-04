/**
 * Cartoon kit shared by the landing-page artwork: characters with
 * blinking eyes and talking mouths, plus small sticker props.
 *
 * Everything here is SVG <g> content drawn around a local origin; place
 * it with a transform. Outlines are ink, 3 units wide.
 */

import type { CSSProperties } from "react";

export const INK = "var(--ink)";

type Hair = "afro" | "bun" | "cap" | "bob" | "buzz" | "beanie";
type Mouth = "smile" | "talk" | "grin" | "o";

export type CharacterProps = {
  x: number;
  y: number;
  scale?: number;
  skin?: string;
  hair?: Hair;
  hairColor?: string;
  shirt?: string;
  mouth?: Mouth;
  /** Extra class on the talking mouth, e.g. to switch it on and off. */
  mouthClass?: string;
  blinkDelay?: string;
  body?: boolean;
};

function HairBack({ hair, color }: { hair: Hair; color: string }) {
  if (hair === "afro")
    return <circle cx="0" cy="-6" r="29" fill={color} stroke={INK} strokeWidth="3" />;
  if (hair === "bob")
    return (
      <path
        d="M-27 10 Q-30 -30 0 -30 Q30 -30 27 10 L18 12 L-18 12 Z"
        fill={color}
        stroke={INK}
        strokeWidth="3"
        strokeLinejoin="round"
      />
    );
  return null;
}

function HairFront({ hair, color }: { hair: Hair; color: string }) {
  switch (hair) {
    case "bun":
      return (
        <g>
          <circle cx="0" cy="-30" r="9" fill={color} stroke={INK} strokeWidth="3" />
          <path
            d="M-22 -6 Q-20 -26 0 -24 Q20 -26 22 -6 Q10 -16 0 -14 Q-10 -16 -22 -6 Z"
            fill={color}
            stroke={INK}
            strokeWidth="3"
            strokeLinejoin="round"
          />
        </g>
      );
    case "cap":
      return (
        <g>
          <path
            d="M-23 -6 Q-23 -28 0 -28 Q23 -28 23 -6 Z"
            fill={color}
            stroke={INK}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <path d="M8 -8 Q30 -10 34 -4 L10 -2 Z" fill={color} stroke={INK} strokeWidth="3" strokeLinejoin="round" />
        </g>
      );
    case "beanie":
      return (
        <g>
          <path
            d="M-24 -4 Q-24 -32 0 -32 Q24 -32 24 -4 Z"
            fill={color}
            stroke={INK}
            strokeWidth="3"
            strokeLinejoin="round"
          />
          <rect x="-25" y="-10" width="50" height="9" rx="4" fill={color} stroke={INK} strokeWidth="3" />
          <circle cx="0" cy="-34" r="5" fill={color} stroke={INK} strokeWidth="3" />
        </g>
      );
    case "buzz":
      return (
        <path
          d="M-21 -8 Q-20 -24 0 -24 Q20 -24 21 -8 Q10 -14 0 -13 Q-10 -14 -21 -8 Z"
          fill={color}
          stroke={INK}
          strokeWidth="3"
          strokeLinejoin="round"
        />
      );
    case "bob":
      return (
        <path
          d="M-22 -4 Q-18 -24 0 -22 Q18 -24 22 -4 Q6 -12 -4 -16 Q-12 -8 -22 -4 Z"
          fill={color}
          stroke={INK}
          strokeWidth="3"
          strokeLinejoin="round"
        />
      );
    default:
      return null;
  }
}

export function Eyes({ y = -2, gap = 8, blinkDelay = "0s" }: { y?: number; gap?: number; blinkDelay?: string }) {
  return (
    <g className="blink" style={{ "--d": blinkDelay } as CSSProperties}>
      <ellipse cx={-gap} cy={y} rx="3.2" ry="4" fill={INK} />
      <ellipse cx={gap} cy={y} rx="3.2" ry="4" fill={INK} />
      <circle cx={-gap + 1} cy={y - 1.5} r="1" fill="white" />
      <circle cx={gap + 1} cy={y - 1.5} r="1" fill="white" />
    </g>
  );
}

export function MouthShape({ mouth, className }: { mouth: Mouth; className?: string }) {
  if (mouth === "talk")
    return (
      <g className={className}>
        <ellipse className="talk" cx="0" cy="10" rx="5.5" ry="5" fill={INK} />
      </g>
    );
  if (mouth === "o") return <ellipse cx="0" cy="10" rx="4" ry="5" fill={INK} className={className} />;
  if (mouth === "grin")
    return (
      <path
        d="M-8 6 Q0 18 8 6 Z"
        fill={INK}
        stroke={INK}
        strokeWidth="2"
        strokeLinejoin="round"
        className={className}
      />
    );
  return (
    <path d="M-7 7 Q0 14 7 7" fill="none" stroke={INK} strokeWidth="3" strokeLinecap="round" className={className} />
  );
}

export function Character({
  x,
  y,
  scale = 1,
  skin = "var(--skin-2)",
  hair = "buzz",
  hairColor = "#2b1d14",
  shirt = "var(--cobalt)",
  mouth = "smile",
  mouthClass,
  blinkDelay,
  body = true,
}: CharacterProps) {
  return (
    <g transform={`translate(${x} ${y}) scale(${scale})`}>
      {body && (
        <path
          d="M-32 60 Q-32 28 0 28 Q32 28 32 60 Z"
          fill={shirt}
          stroke={INK}
          strokeWidth="3"
          strokeLinejoin="round"
        />
      )}
      <HairBack hair={hair} color={hairColor} />
      <circle cx="0" cy="0" r="22" fill={skin} stroke={INK} strokeWidth="3" />
      <HairFront hair={hair} color={hairColor} />
      <circle cx="-13" cy="7" r="3.5" fill="var(--pink)" opacity="0.6" />
      <circle cx="13" cy="7" r="3.5" fill="var(--pink)" opacity="0.6" />
      <Eyes blinkDelay={blinkDelay} />
      <MouthShape mouth={mouth} className={mouthClass} />
    </g>
  );
}

/** A four-point sparkle. */
export function Sparkle({ x, y, size = 10, fill = "var(--sun)", className }: { x: number; y: number; size?: number; fill?: string; className?: string }) {
  const s = size;
  return (
    <path
      className={className}
      d={`M${x} ${y - s} Q${x + s * 0.18} ${y - s * 0.18} ${x + s} ${y} Q${x + s * 0.18} ${y + s * 0.18} ${x} ${y + s} Q${x - s * 0.18} ${y + s * 0.18} ${x - s} ${y} Q${x - s * 0.18} ${y - s * 0.18} ${x} ${y - s} Z`}
      fill={fill}
      stroke={INK}
      strokeWidth="2.5"
      strokeLinejoin="round"
    />
  );
}

/** A heart. */
export function Heart({ x, y, size = 1, fill = "var(--tomato)", className }: { x: number; y: number; size?: number; fill?: string; className?: string }) {
  return (
    <g transform={`translate(${x} ${y}) scale(${size})`}>
      <path
        className={className}
        d="M0 8 C-14 -2 -10 -14 -2 -12 C0 -11 0 -9 0 -8 C0 -9 0 -11 2 -12 C10 -14 14 -2 0 8 Z"
        fill={fill}
        stroke={INK}
        strokeWidth="2.5"
        strokeLinejoin="round"
      />
    </g>
  );
}
