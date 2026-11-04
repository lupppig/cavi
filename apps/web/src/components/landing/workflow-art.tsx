/**
 * Cartoon artwork for each workflow stage. SVG + CSS on a shared 8s loop
 * (the `wf-*` classes in globals.css). An element's un-animated style is
 * its finished frame, so reduced-motion visitors see a complete still.
 *
 * Pattern: static placement goes on an outer <g transform>, animation on
 * an inner <g className>, because CSS transforms replace SVG transforms.
 */

import type { CSSProperties, ReactNode } from "react";
import { Character, Eyes, Heart, INK, MouthShape, Sparkle } from "./cartoon";

function Canvas({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 320 240" className="art wf h-full w-full overflow-visible" aria-hidden="true" focusable="false">
      {children}
    </svg>
  );
}

function T({
  x,
  y,
  children,
  size = 12,
  anchor,
  fill = INK,
  weight = 800,
}: {
  x: number;
  y: number;
  children: ReactNode;
  size?: number;
  anchor?: "start" | "middle" | "end";
  fill?: string;
  weight?: number;
}) {
  return (
    <text x={x} y={y} fontSize={size} fontWeight={weight} textAnchor={anchor} fill={fill} className="font-sans">
      {children}
    </text>
  );
}

/** A rounded sticker rect with a hard offset shadow. */
function Card({ x, y, w, h, r = 14, fill = "white", shadow = 4 }: { x: number; y: number; w: number; h: number; r?: number; fill?: string; shadow?: number }) {
  return (
    <g>
      {shadow > 0 && <rect x={x + shadow} y={y + shadow} width={w} height={h} rx={r} fill={INK} />}
      <rect x={x} y={y} width={w} height={h} rx={r} fill={fill} stroke={INK} strokeWidth="3" />
    </g>
  );
}

function Pill({ x, y, w, label, fill = "white", color = INK, size = 11 }: { x: number; y: number; w: number; label: string; fill?: string; color?: string; size?: number }) {
  return (
    <g>
      <rect x={x} y={y} width={w} height="22" rx="11" fill={fill} stroke={INK} strokeWidth="2.5" />
      <T x={x + w / 2} y={y + 15} size={size} anchor="middle" fill={color}>
        {label}
      </T>
    </g>
  );
}

const viewBoxOrigin = (x: number, y: number): CSSProperties => ({
  transformBox: "view-box",
  transformOrigin: `${x}px ${y}px`,
});

/* 01 Direction: a megaphone shouts the brief; storyboards pop out. */
export function DirectionArt() {
  const boards = [
    { x: 134, rot: -6, label: "hook", cls: "wf-in-45", fill: "var(--pink)" },
    { x: 192, rot: 3, label: "answer", cls: "wf-in-55", fill: "var(--sky)" },
    { x: 250, rot: 8, label: "out", cls: "wf-in-65", fill: "var(--mint)" },
  ];
  return (
    <Canvas>
      {/* Speech bubble */}
      <path d="M128 98 L104 136 L150 102 Z" fill="white" stroke={INK} strokeWidth="3" strokeLinejoin="round" />
      <Card x={112} y={12} w={194} h={92} r={20} />
      <rect x="140" y="96" width="14" height="8" fill="white" />
      <g className="wf-in-10">
        <T x={128} y={40} size={15}>fast cut!</T>
      </g>
      <g className="wf-in-20">
        <T x={128} y={63} size={15}>lead with pricing!</T>
      </g>
      <g className="wf-in-30">
        <T x={128} y={86} size={15} fill="var(--tomato)">make it 9:16!</T>
      </g>

      {/* Megaphone buddy */}
      <g className="wf-shout" style={viewBoxOrigin(60, 176)}>
        <rect x="34" y="190" width="16" height="26" rx="6" fill="var(--sun)" stroke={INK} strokeWidth="3" />
        <path d="M18 158 L86 132 L86 222 L18 196 Z" fill="var(--cobalt)" stroke={INK} strokeWidth="3.5" strokeLinejoin="round" />
        <ellipse cx="88" cy="177" rx="12" ry="46" fill="white" stroke={INK} strokeWidth="3.5" />
        <g transform="translate(48 172)">
          <circle r="17" fill="white" stroke={INK} strokeWidth="3" />
          <Eyes y={-3} gap={6} blinkDelay="0.3s" />
          <MouthShape mouth="grin" />
        </g>
      </g>
      {[0, 1, 2].map((i) => (
        <g key={i} className={`wf-in-${10 + i * 10}`}>
          <path
            d={`M${106 + i * 6} ${150 - i * 4} Q${116 + i * 8} ${177} ${106 + i * 6} ${204 + i * 4}`}
            fill="none"
            stroke={INK}
            strokeWidth="3"
            strokeLinecap="round"
          />
        </g>
      ))}

      {/* Storyboards */}
      {boards.map((b, i) => (
        <g key={b.label} transform={`rotate(${b.rot} ${b.x + 26} 168)`}>
          <g className={b.cls}>
            <Card x={b.x} y={126} w={52} h={78} r={10} shadow={3} />
            <rect x={b.x + 6} y={132} width="40" height="46" rx="6" fill={b.fill} stroke={INK} strokeWidth="2.5" />
            {i < 2 ? (
              <g transform={`translate(${b.x + 26} 152)`}>
                <circle r="10" fill={i === 0 ? "var(--skin-2)" : "var(--skin-1)"} stroke={INK} strokeWidth="2.5" />
                <circle cx="-3.5" cy="-1" r="1.6" fill={INK} />
                <circle cx="3.5" cy="-1" r="1.6" fill={INK} />
                <path d="M-3 4 Q0 7 3 4" fill="none" stroke={INK} strokeWidth="2" strokeLinecap="round" />
              </g>
            ) : (
              <Sparkle x={b.x + 26} y={155} size={12} fill="var(--sun)" />
            )}
            <T x={b.x + 26} y={195} size={11} anchor="middle">
              {b.label}
            </T>
          </g>
        </g>
      ))}
    </Canvas>
  );
}

/* 02 Selection: a magnifier with eyes finds the moments worth cutting. */
export function SelectionArt() {
  const bars = Array.from({ length: 38 }, (_, i) => {
    const v = Math.abs(Math.sin(i * 0.55 + 1) * 0.6 + Math.sin(i * 1.9) * 0.4);
    return 0.25 + v * 0.75;
  });
  const finds = [
    { from: 4, to: 8, cls: "wf-in-15" },
    { from: 16, to: 22, cls: "wf-in-30" },
    { from: 29, to: 33, cls: "wf-in-50" },
  ];
  const bx = (i: number) => 26 + i * 7.3;
  return (
    <Canvas>
      <Card x={14} y={22} w={292} h={78} r={18} />
      {finds.map((f) => (
        <g key={f.from} className={f.cls}>
          <rect x={bx(f.from) - 5} y="30" width={(f.to - f.from) * 7.3 + 10} height="62" rx="10" fill="var(--mint)" stroke={INK} strokeWidth="2.5" />
          <Sparkle x={bx(f.to) + 4} y={30} size={9} fill="var(--sun)" />
        </g>
      ))}
      {bars.map((h, i) => (
        <rect key={i} x={bx(i)} y={61 - h * 24} width="4" height={h * 48} rx="2" fill={INK} />
      ))}

      {/* Magnifier buddy */}
      <g className="wf-scan">
        <path d="M60 86 L82 110" stroke={INK} strokeWidth="11" strokeLinecap="round" />
        <path d="M60 86 L82 110" stroke="var(--tomato)" strokeWidth="5" strokeLinecap="round" />
        <circle cx="42" cy="62" r="28" fill="rgb(255 255 255 / 0.55)" stroke={INK} strokeWidth="4" />
        <g transform="translate(42 60)">
          <Eyes y={-2} gap={8} blinkDelay="1s" />
          <MouthShape mouth="o" />
        </g>
      </g>

      <path d="M144 100 V128" pathLength={100} stroke={INK} strokeWidth="3" strokeDasharray="5 5" className="wf-draw-62" fill="none" />
      <g className="wf-in-70">
        <Card x={36} y={128} w={248} h={92} r={20} fill="var(--sun)" shadow={5} />
        <rect x="52" y="142" width="38" height="64" rx="8" fill="var(--sky)" stroke={INK} strokeWidth="2.5" />
        <g transform="translate(71 168)">
          <circle r="10" fill="var(--skin-3)" stroke={INK} strokeWidth="2.5" />
          <circle cx="-3.5" cy="-1" r="1.6" fill={INK} />
          <circle cx="3.5" cy="-1" r="1.6" fill={INK} />
          <ellipse cx="0" cy="5" rx="3" ry="2.5" fill={INK} />
        </g>
        <Pill x={102} y={142} w={92} label="moment #2" fill="var(--tomato)" color="white" />
        <T x={102} y={184} size={19}>31:17 → 31:30</T>
        <T x={102} y={204} size={11} weight={700}>setup kept · no cut-offs</T>
      </g>
    </Canvas>
  );
}

/* 03 Edit: snip, trim and ripple. */
export function EditArt() {
  const frameFills = ["var(--sky)", "var(--sun)", "var(--pink)", "white", "var(--sky)", "var(--sun)"];
  const frame = (i: number) => (
    <rect key={i} x={24 + i * 48} y="62" width="38" height="32" rx="5" fill={frameFills[i]} stroke={INK} strokeWidth="2" />
  );
  const holes = (from: number, to: number) =>
    Array.from({ length: Math.floor((to - from) / 12) }, (_, k) => (
      <g key={k}>
        <rect x={from + 4 + k * 12} y="54" width="6" height="5" rx="1.5" fill="white" />
        <rect x={from + 4 + k * 12} y="97" width="6" height="5" rx="1.5" fill="white" />
      </g>
    ));
  return (
    <Canvas>
      {/* Film strip, cut between frames 3 and 4 */}
      <g>
        <rect x="14" y="50" width="152" height="56" rx="8" fill={INK} />
        {holes(14, 166)}
        {[0, 1, 2].map(frame)}
      </g>
      <g className="wf-split">
        <rect x="160" y="50" width="146" height="56" rx="8" fill={INK} />
        {holes(166, 306)}
        {[3, 4, 5].map(frame)}
      </g>

      {/* Scissors buddy (pivot at 163,34) */}
      <g className="wf-snip-l" style={viewBoxOrigin(163, 34)}>
        <path d="M150 12 L163 34 L157 92" fill="none" stroke={INK} strokeWidth="10" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M163 34 L157 90" fill="none" stroke="white" strokeWidth="4" strokeLinecap="round" />
        <circle cx="146" cy="10" r="9" fill="var(--tomato)" stroke={INK} strokeWidth="3" />
      </g>
      <g className="wf-snip-r" style={viewBoxOrigin(163, 34)}>
        <path d="M176 12 L163 34 L169 92" fill="none" stroke={INK} strokeWidth="10" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M163 34 L169 90" fill="none" stroke="white" strokeWidth="4" strokeLinecap="round" />
        <circle cx="180" cy="10" r="9" fill="var(--tomato)" stroke={INK} strokeWidth="3" />
      </g>
      <g transform="translate(163 34)">
        <circle r="10" fill="var(--sun)" stroke={INK} strokeWidth="3" />
        <circle cx="-3.5" cy="-1" r="1.7" fill={INK} />
        <circle cx="3.5" cy="-1" r="1.7" fill={INK} />
      </g>

      {/* Timeline: trim + ripple */}
      <Card x={14} y={130} w={292} h={44} r={12} shadow={3} />
      <rect x="22" y="138" width="120" height="28" rx="7" fill="var(--pink)" stroke={INK} strokeWidth="2.5" className="wf-trim" />
      <rect x="138" y="134" width="6" height="36" rx="3" fill="var(--tomato)" stroke={INK} strokeWidth="2" className="wf-handle" />
      <rect x="146" y="138" width="110" height="28" rx="7" fill="var(--sky)" stroke={INK} strokeWidth="2.5" className="wf-ripple" />
      <g className="wf-playhead">
        <path d="M24 124 V180" stroke={INK} strokeWidth="3" strokeLinecap="round" />
        <circle cx="24" cy="124" r="5" fill="var(--tomato)" stroke={INK} strokeWidth="2.5" />
      </g>

      {[
        { x: 30, w: 76, label: "snip!", cls: "wf-in-20" },
        { x: 120, w: 76, label: "trim", cls: "wf-in-45" },
        { x: 210, w: 76, label: "ripple", cls: "wf-in-60" },
      ].map((t) => (
        <g key={t.label}>
          <Pill x={t.x} y={192} w={t.w} label={t.label} />
          <g className={t.cls}>
            <Pill x={t.x} y={192} w={t.w} label={t.label} fill="var(--tomato)" color="white" />
          </g>
        </g>
      ))}
    </Canvas>
  );
}

/* 04 Review: comments, versions, and a big approval stamp. */
export function ReviewArt() {
  return (
    <Canvas>
      <g className="wf-shake">
        <Card x={14} y={16} w={180} h={124} r={18} />
        <rect x="24" y="26" width="160" height="86" rx="10" fill="var(--sun)" stroke={INK} strokeWidth="2.5" />
        <Character x={104} y={62} scale={0.82} skin="var(--skin-3)" hair="cap" hairColor="var(--cobalt)" shirt="var(--pink)" mouth="grin" blinkDelay="0.8s" />
        <rect x="24" y="96" width="160" height="16" fill="var(--sun)" />
        <path d="M30 124 H178" stroke={INK} strokeWidth="3" strokeLinecap="round" />
        <g className="wf-in-15">
          <circle cx="58" cy="124" r="6" fill="var(--tomato)" stroke={INK} strokeWidth="2.5" />
        </g>
        <g className="wf-in-40">
          <circle cx="140" cy="124" r="6" fill="var(--tomato)" stroke={INK} strokeWidth="2.5" />
        </g>
      </g>

      <g className="wf-in-20">
        <path d="M206 50 L196 60 L214 52 Z" fill="white" stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
        <Card x={204} y={16} w={104} h={40} r={16} shadow={3} />
        <T x={256} y={41} size={12} anchor="middle">tighten intro</T>
      </g>
      <g className="wf-in-40">
        <path d="M206 108 L196 118 L214 110 Z" fill="white" stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
        <Card x={204} y={74} w={104} h={40} r={16} shadow={3} />
        <T x={248} y={99} size={12} anchor="middle">love this</T>
        <Heart x={291} y={95} size={0.7} />
      </g>
      <g className="wf-heart">
        <Heart x={290} y={70} size={0.9} fill="var(--pink)" />
      </g>

      {/* Versions */}
      <T x={16} y={168} size={11} weight={700}>versions</T>
      {["v1", "v2", "v3"].map((v, i) => (
        <Pill key={v} x={16 + i * 50} y={176} w={42} label={v} />
      ))}
      <g className="wf-version">
        <Pill x={16} y={176} w={42} label="" fill="var(--tomato)" />
      </g>
      <g className="wf-in-80">
        <Pill x={170} y={176} w={136} label="client signed off ✓" fill="var(--mint)" size={11} />
      </g>

      {/* Stamp */}
      <g transform="rotate(-12 104 70)">
        <g className="wf-stamp">
          <rect x="34" y="50" width="140" height="40" rx="12" fill="var(--tomato)" stroke={INK} strokeWidth="3.5" />
          <T x={104} y={78} size={22} anchor="middle" fill="white">
            APPROVED!
          </T>
        </g>
      </g>
    </Canvas>
  );
}

/* 05 Render: a toaster pops out finished frames. */
export function RenderArt() {
  const toast = (x: number, fill: string, cls: string, skin: string) => (
    <g className={cls}>
      <rect x={x} y="64" width="46" height="66" rx="8" fill="white" stroke={INK} strokeWidth="3" />
      <rect x={x + 6} y="70" width="34" height="44" rx="5" fill={fill} stroke={INK} strokeWidth="2" />
      <g transform={`translate(${x + 23} 90)`}>
        <circle r="8" fill={skin} stroke={INK} strokeWidth="2" />
        <circle cx="-3" cy="-1" r="1.4" fill={INK} />
        <circle cx="3" cy="-1" r="1.4" fill={INK} />
      </g>
      <rect x={x + 9} y="106" width="28" height="4" rx="2" fill={INK} />
    </g>
  );
  return (
    <Canvas>
      {/* Frames start inside the toaster and pop up */}
      {toast(98, "var(--pink)", "wf-rise-1", "var(--skin-2)")}
      {toast(176, "var(--sky)", "wf-rise-2", "var(--skin-1)")}

      <g className="wf-in-40">
        <Sparkle x={86} y={56} size={8} fill="var(--sun)" />
      </g>
      <g className="wf-in-60">
        <Sparkle x={234} y={48} size={9} fill="white" />
      </g>

      {/* Toaster buddy */}
      <rect x="66" y="122" width="190" height="96" rx="34" fill={INK} transform="translate(5 5)" />
      <rect x="66" y="122" width="190" height="96" rx="34" fill="var(--cobalt)" stroke={INK} strokeWidth="3.5" />
      <rect x="92" y="116" width="58" height="14" rx="7" fill={INK} />
      <rect x="170" y="116" width="58" height="14" rx="7" fill={INK} />
      <rect x="254" y="148" width="20" height="14" rx="5" fill="var(--sun)" stroke={INK} strokeWidth="3" />
      <ellipse cx="160" cy="172" rx="46" ry="30" fill="white" stroke={INK} strokeWidth="3" />
      <g transform="translate(160 170)">
        <Eyes y={-4} gap={13} blinkDelay="0.4s" />
        <MouthShape mouth="grin" />
      </g>

      <rect x="66" y="226" width="190" height="10" rx="5" fill="white" stroke={INK} strokeWidth="2.5" />
      <rect x="68" y="228" width="186" height="6" rx="3" fill="var(--tomato)" className="wf-progress" />

      <g transform="rotate(8 274 28)">
        <g className="wf-in-85">
          <Card x={232} y={12} w={84} h={32} r={16} fill="var(--mint)" shadow={3} />
          <T x={274} y={33} size={13} anchor="middle">verified ✓</T>
        </g>
      </g>
      <g className="wf-in-85">
        <T x={20} y={36} size={11} weight={700}>1080×1920 ✓</T>
        <T x={20} y={54} size={11} weight={700}>audio in sync ✓</T>
      </g>
    </Canvas>
  );
}

/* 06 Distribute: paper planes deliver one master to every format. */
export function DistributeArt() {
  const plane = (
    <g>
      <path d="M-12 -8 L14 0 L-12 8 L-6 0 Z" fill="white" stroke={INK} strokeWidth="2.5" strokeLinejoin="round" />
      <path d="M-6 0 L14 0" stroke={INK} strokeWidth="2" />
    </g>
  );
  const outputs = [
    { label: "9:16", sched: "wf-until-30", pub: "wf-in-30", y: 34 },
    { label: "1:1", sched: "wf-until-48", pub: "wf-in-48", y: 112 },
    { label: "16:9", sched: "wf-until-66", pub: "wf-in-66", y: 190 },
  ];
  return (
    <Canvas>
      {/* Master */}
      <Card x={14} y={84} w={74} h={74} r={18} />
      <path d="M40 104 L66 121 L40 138 Z" fill="var(--tomato)" stroke={INK} strokeWidth="3" strokeLinejoin="round" />
      <T x={51} y={178} size={13} anchor="middle" fill="white">master</T>

      {/* Destinations */}
      <Card x={262} y={8} w={36} h={60} r={8} fill="var(--sun)" shadow={3} />
      <rect x="272" y="13" width="16" height="4" rx="2" fill={INK} />
      <Card x={248} y={86} w={56} h={56} r={12} fill="var(--sky)" shadow={3} />
      <Card x={226} y={170} w={82} h={46} r={10} fill="var(--mint)" shadow={3} />

      {outputs.map((o) => (
        <g key={o.label}>
          <g className={o.sched}>
            <Pill x={140} y={o.y - 11} w={76} label="scheduled" size={10.5} />
          </g>
          <g className={o.pub}>
            <Pill x={140} y={o.y - 11} w={76} label="published" fill="var(--sun)" size={10.5} />
          </g>
          <T x={178} y={o.y + 26} size={11} anchor="middle" fill="white">
            {o.label}
          </T>
        </g>
      ))}
      <g className="wf-in-35">
        <Heart x={256} y={20} size={0.75} fill="var(--pink)" />
      </g>
      <g className="wf-in-55">
        <Heart x={242} y={98} size={0.75} fill="var(--pink)" />
      </g>
      <g className="wf-in-70">
        <Heart x={222} y={178} size={0.75} fill="var(--pink)" />
      </g>

      {/* Planes launch from the master */}
      {["wf-plane-1", "wf-plane-2", "wf-plane-3"].map((cls) => (
        <g key={cls} transform="translate(92 121)">
          <g className={cls}>{plane}</g>
        </g>
      ))}
    </Canvas>
  );
}
