/**
 * Cartoon portraits for each audience. Decorative SVG; the card text
 * carries the meaning.
 */

import type { CSSProperties, ReactNode } from "react";
import { Character, Heart, INK, Sparkle } from "./cartoon";

function Art({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 200 150" className="art h-full w-full overflow-visible" aria-hidden="true" focusable="false">
      {children}
    </svg>
  );
}

const d = (s: string) => ({ "--d": s }) as CSSProperties;

export function ClipperArt() {
  return (
    <Art>
      <Character x={80} y={68} scale={1.2} skin="var(--skin-3)" hair="cap" hairColor="var(--tomato)" shirt="var(--sky)" mouth="grin" blinkDelay="0.3s" />
      <g className="bob" style={d("-0.5s")}>
        <rect x="128" y="40" width="44" height="78" rx="10" fill="var(--pink)" stroke={INK} strokeWidth="3" />
        <rect x="134" y="50" width="32" height="50" rx="5" fill="white" stroke={INK} strokeWidth="2.5" />
        <path d="M145 66 L157 75 L145 84 Z" fill="var(--tomato)" stroke={INK} strokeWidth="2" strokeLinejoin="round" />
        <rect x="142" y="106" width="16" height="4" rx="2" fill={INK} />
      </g>
      <g className="wobble" style={d("-1s")}>
        <circle cx="34" cy="34" r="8" fill="none" stroke={INK} strokeWidth="3" />
        <circle cx="50" cy="34" r="8" fill="none" stroke={INK} strokeWidth="3" />
        <path d="M38 40 L56 64 M46 40 L28 64" stroke={INK} strokeWidth="4" strokeLinecap="round" />
      </g>
    </Art>
  );
}

export function PodcasterArt() {
  return (
    <Art>
      <Character x={92} y={70} scale={1.2} skin="var(--skin-1)" hair="buzz" shirt="var(--tomato)" mouth="talk" blinkDelay="1.2s" />
      {/* Headphones */}
      <path d="M62 66 Q62 30 92 30 Q122 30 122 66" fill="none" stroke={INK} strokeWidth="6" strokeLinecap="round" />
      <rect x="56" y="58" width="12" height="22" rx="6" fill="var(--sun)" stroke={INK} strokeWidth="3" />
      <rect x="116" y="58" width="12" height="22" rx="6" fill="var(--sun)" stroke={INK} strokeWidth="3" />
      {/* Mic */}
      <path d="M150 140 V108" stroke={INK} strokeWidth="5" strokeLinecap="round" />
      <rect x="138" y="70" width="24" height="42" rx="12" fill={INK} />
      <rect x="142" y="76" width="16" height="22" rx="8" fill="var(--cobalt)" />
      {[0, 1].map((i) => (
        <path
          key={i}
          className="pulse-ring"
          style={d(`${i * 0.9}s`)}
          d={`M${170 + i * 8} 72 Q${180 + i * 10} 90 ${170 + i * 8} 108`}
          fill="none"
          stroke={INK}
          strokeWidth="3"
          strokeLinecap="round"
        />
      ))}
    </Art>
  );
}

export function MusicianArt() {
  return (
    <Art>
      <g className="spin-slow" style={{ transformOrigin: "center" }}>
        <circle cx="148" cy="80" r="40" fill={INK} />
        <circle cx="148" cy="80" r="30" fill="none" stroke="#3a3a3a" strokeWidth="2" />
        <circle cx="148" cy="80" r="14" fill="var(--pink)" stroke={INK} strokeWidth="2" />
        <circle cx="148" cy="80" r="3" fill={INK} />
      </g>
      <Character x={74} y={70} scale={1.2} skin="var(--skin-2)" hair="afro" hairColor="#4a2a1a" shirt="var(--sun)" mouth="o" blinkDelay="2s" />
      <g className="float-note" style={d("0s")}>
        <path d="M22 50 V24 L40 20 V44" fill="none" stroke={INK} strokeWidth="3" />
        <circle cx="18" cy="50" r="6" fill="var(--cobalt)" stroke={INK} strokeWidth="2.5" />
        <circle cx="36" cy="46" r="6" fill="var(--cobalt)" stroke={INK} strokeWidth="2.5" />
      </g>
      <g className="float-note" style={d("-1.3s")}>
        <path d="M186 22 V6" fill="none" stroke={INK} strokeWidth="3" />
        <circle cx="182" cy="22" r="5" fill="var(--tomato)" stroke={INK} strokeWidth="2.5" />
      </g>
    </Art>
  );
}

export function StudioArt() {
  return (
    <Art>
      <Character x={60} y={70} scale={1.15} skin="var(--skin-2)" hair="beanie" hairColor="var(--mint)" shirt="var(--cobalt)" mouth="smile" blinkDelay="0.9s" />
      <g>
        <rect x="98" y="34" width="96" height="66" rx="8" fill="white" stroke={INK} strokeWidth="3" />
        <rect x="106" y="44" width="80" height="18" rx="4" fill="var(--sky)" stroke={INK} strokeWidth="2" />
        <rect x="106" y="68" width="34" height="10" rx="3" fill="var(--pink)" stroke={INK} strokeWidth="2" />
        <rect x="142" y="68" width="44" height="10" rx="3" fill="var(--sun)" stroke={INK} strokeWidth="2" />
        <rect x="106" y="82" width="56" height="10" rx="3" fill="var(--mint)" stroke={INK} strokeWidth="2" />
        <path className="wf-playhead-mini" d="M130 40 V96" stroke="var(--tomato)" strokeWidth="3" />
        <path d="M88 108 H204 L196 118 H96 Z" fill="var(--paper-deep)" stroke={INK} strokeWidth="3" strokeLinejoin="round" />
      </g>
      <g className="bob" style={d("-1.5s")}>
        <Sparkle x={186} y={22} size={10} fill="var(--sun)" />
      </g>
    </Art>
  );
}

export function BrandArt() {
  return (
    <Art>
      <Character x={64} y={70} scale={1.15} skin="var(--skin-3)" hair="bob" hairColor="#2b1d14" shirt="var(--pink)" mouth="grin" blinkDelay="1.6s" />
      <g className="bob" style={d("-0.8s")}>
        <path d="M110 58 H170 L164 128 H116 Z" fill="var(--sun)" stroke={INK} strokeWidth="3" strokeLinejoin="round" />
        <path d="M126 58 Q126 36 140 36 Q154 36 154 58" fill="none" stroke={INK} strokeWidth="3" />
        <Sparkle x={140} y={92} size={14} fill="var(--tomato)" />
      </g>
      <g className="wobble" style={d("-0.4s")}>
        <Heart x={184} y={34} size={1} fill="var(--tomato)" />
      </g>
    </Art>
  );
}
