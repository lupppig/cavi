import type { CSSProperties, ReactNode } from "react";
import { Eyes, Heart, INK, MouthShape, Sparkle } from "./cartoon";

/** Floating cartoon stickers around the hero headline. Decorative. */

function Sticker({
  className,
  style,
  children,
  viewBox,
}: {
  className: string;
  style?: CSSProperties;
  children: ReactNode;
  viewBox: string;
}) {
  return (
    <div aria-hidden="true" className={`pointer-events-none absolute ${className}`} style={style}>
      <svg viewBox={viewBox} className="art bob h-full w-full overflow-visible">
        {children}
      </svg>
    </div>
  );
}

function CameraBuddy() {
  return (
    <g>
      <rect x="6" y="22" width="92" height="64" rx="14" fill="var(--cobalt)" stroke={INK} strokeWidth="3.5" />
      <rect x="26" y="10" width="30" height="16" rx="5" fill="var(--cobalt)" stroke={INK} strokeWidth="3.5" />
      <circle cx="52" cy="54" r="22" fill="white" stroke={INK} strokeWidth="3.5" />
      <g transform="translate(52 52)">
        <Eyes y={-3} gap={7} blinkDelay="0.7s" />
        <MouthShape mouth="grin" />
      </g>
      <circle className="rec-dot" cx="86" cy="34" r="5" fill="var(--tomato)" stroke={INK} strokeWidth="2.5" />
    </g>
  );
}

function Clapper() {
  return (
    <g>
      <rect x="8" y="34" width="84" height="56" rx="8" fill="white" stroke={INK} strokeWidth="3.5" />
      <g className="wobble" style={{ transformOrigin: "10px 34px" } as CSSProperties}>
        <rect x="8" y="16" width="84" height="18" rx="4" fill={INK} stroke={INK} strokeWidth="3.5" transform="rotate(-12 8 34)" />
        {[0, 1, 2, 3].map((i) => (
          <path key={i} d={`M${20 + i * 20} 16 l10 0 l-8 18 l-10 0 Z`} fill="white" transform="rotate(-12 8 34)" />
        ))}
      </g>
      <g transform="translate(50 64)">
        <Eyes y={-4} gap={9} blinkDelay="1.6s" />
        <MouthShape mouth="smile" />
      </g>
    </g>
  );
}

function PlayBurst() {
  const points = Array.from({ length: 12 }, (_, i) => {
    const a = (i / 12) * Math.PI * 2;
    const r = i % 2 === 0 ? 46 : 36;
    return `${50 + Math.cos(a) * r},${50 + Math.sin(a) * r}`;
  }).join(" ");
  return (
    <g>
      <g className="spin-slow">
        <polygon points={points} fill="var(--sun)" stroke={INK} strokeWidth="3.5" strokeLinejoin="round" />
      </g>
      <path d="M42 34 L68 50 L42 66 Z" fill="var(--tomato)" stroke={INK} strokeWidth="3.5" strokeLinejoin="round" />
    </g>
  );
}

function PhoneBuddy() {
  return (
    <g>
      <rect x="20" y="4" width="60" height="104" rx="14" fill="var(--pink)" stroke={INK} strokeWidth="3.5" />
      <rect x="40" y="10" width="20" height="5" rx="2.5" fill={INK} />
      <g transform="translate(50 50)">
        <Eyes y={-6} gap={9} blinkDelay="2.4s" />
        <MouthShape mouth="o" />
      </g>
      <rect x="30" y="80" width="40" height="8" rx="4" fill="white" stroke={INK} strokeWidth="2.5" />
    </g>
  );
}

export function HeroStickers() {
  return (
    <>
      <Sticker viewBox="0 0 104 96" className="top-2 right-3 h-16 w-16 sm:top-6 sm:right-8 sm:h-24 sm:w-24 lg:top-10 lg:right-[6%] lg:h-32 lg:w-32" style={{ "--rot": "8deg" } as CSSProperties}>
        <CameraBuddy />
      </Sticker>
      <Sticker viewBox="0 0 100 100" className="hidden md:block md:top-[46%] md:right-[3%] md:h-20 md:w-20 lg:top-[38%] lg:right-[2%] lg:h-24 lg:w-24" style={{ "--d": "-1.2s" } as CSSProperties}>
        <PlayBurst />
      </Sticker>
      <Sticker viewBox="0 0 100 100" className="hidden lg:block lg:top-[33%] lg:right-[26%] lg:h-20 lg:w-20" style={{ "--d": "-2s", "--rot": "-10deg" } as CSSProperties}>
        <Clapper />
      </Sticker>
      <Sticker viewBox="0 0 100 112" className="hidden xl:block xl:top-[14%] xl:right-[23%] xl:h-24 xl:w-20" style={{ "--d": "-0.6s", "--rot": "6deg" } as CSSProperties}>
        <PhoneBuddy />
      </Sticker>
      <Sticker viewBox="0 0 40 40" className="hidden sm:block sm:top-[3%] sm:right-[42%] sm:h-8 sm:w-8 lg:top-[5%] lg:right-[46%]" style={{ "--d": "-1.8s" } as CSSProperties}>
        <Sparkle x={20} y={20} size={16} fill="var(--mint)" />
      </Sticker>
      <Sticker viewBox="-20 -20 40 40" className="hidden lg:block lg:top-[52%] lg:right-[13%] lg:h-10 lg:w-10" style={{ "--d": "-2.6s" } as CSSProperties}>
        <Heart x={0} y={0} size={1.4} />
      </Sticker>
    </>
  );
}
