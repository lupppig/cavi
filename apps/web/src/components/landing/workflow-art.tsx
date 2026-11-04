/**
 * Animated artwork for each workflow stage. Pure SVG + CSS (see the `wf-*`
 * classes in globals.css). Every element's un-animated style is the
 * finished frame, so reduced-motion visitors see a complete still.
 *
 * All artwork shares one 320×240 canvas and one 8-second loop.
 */

import type { ReactNode } from "react";

function Canvas({ children }: { children: ReactNode }) {
  return (
    <svg
      viewBox="0 0 320 240"
      className="wf h-full w-full"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

function Label({
  x,
  y,
  children,
  className = "fill-faint",
  anchor,
  size = 8,
}: {
  x: number;
  y: number;
  children: ReactNode;
  className?: string;
  anchor?: "start" | "middle" | "end";
  size?: number;
}) {
  return (
    <text
      x={x}
      y={y}
      fontSize={size}
      letterSpacing={size <= 8 ? 1 : 0.2}
      textAnchor={anchor}
      className={`font-mono ${className}`}
    >
      {children}
    </text>
  );
}

function Person({ cx, top, scale = 1, tone = "a" }: { cx: number; top: number; scale?: number; tone?: "a" | "b" }) {
  const fill = tone === "a" ? "fill-speaker-a" : "fill-speaker-b";
  const r = 10 * scale;
  const w = 26 * scale;
  const h = 22 * scale;
  return (
    <g>
      <circle cx={cx} cy={top + r} r={r} className={`${fill} opacity-70`} />
      <path
        d={`M${cx - w} ${top + 2 * r + 4 + h} Q${cx - w} ${top + 2 * r + 4} ${cx} ${top + 2 * r + 4} Q${cx + w} ${top + 2 * r + 4} ${cx + w} ${top + 2 * r + 4 + h} Z`}
        className={`${fill} opacity-45`}
      />
    </g>
  );
}

// Deterministic pseudo-waveform.
function bars(count: number, seed: number) {
  return Array.from({ length: count }, (_, i) => {
    const v = Math.abs(Math.sin(i * 0.41 + seed) * 0.6 + Math.sin(i * 1.7 + seed) * 0.4);
    return i % 17 < 2 ? 0.1 : 0.2 + v * 0.8;
  });
}

/* 01 — Direction: a brief becomes a storyboard. */
export function DirectionArt() {
  const frames = [
    { x: 36, label: "HOOK", cls: "wf-in-35" },
    { x: 128, label: "ANSWER", cls: "wf-in-45" },
    { x: 220, label: "OUT", cls: "wf-in-55" },
  ];
  return (
    <Canvas>
      <defs>
        <clipPath id="wf-direction-brief">
          <rect x="17" y="19" width="286" height="42" />
        </clipPath>
      </defs>
      <rect x="16" y="18" width="288" height="44" className="fill-ink stroke-line-strong" />
      <Label x={28} y={34}>BRIEF</Label>
      <g clipPath="url(#wf-direction-brief)">
        <text x="28" y="51" fontSize="10.5" className="font-mono fill-paper">
          fast cut · lead with pricing · 9:16
        </text>
        <g className="wf-type">
          <rect x="26" y="40" width="300" height="16" className="fill-ink" />
          <rect x="26" y="41" width="1.5" height="13" className="fill-tally" />
        </g>
      </g>

      <path d="M160 64 V84" pathLength={100} className="wf-draw-28 fill-none stroke-tally" strokeWidth="1.5" />
      <path d="M155 79 L160 85 L165 79" className="wf-in-30 fill-none stroke-tally" strokeWidth="1.5" />

      {frames.map((f, i) => (
        <g key={f.label} className={f.cls}>
          <rect x={f.x} y="92" width="64" height="112" className="fill-ink stroke-paper/70" />
          {i < 2 ? (
            <Person cx={f.x + 32} top={106} tone={i === 0 ? "b" : "a"} />
          ) : (
            <g>
              <rect x={f.x + 14} y="136" width="36" height="5" className="fill-paper/80" />
              <rect x={f.x + 20} y="146" width="24" height="3" className="fill-paper/40" />
            </g>
          )}
          {i === 0 && <rect x={f.x + 10} y="176" width="44" height="7" className="fill-tally" />}
          {i === 1 && (
            <g>
              <rect x={f.x + 8} y="176" width="22" height="7" className="fill-paper/80" />
              <rect x={f.x + 33} y="176" width="22" height="7" className="fill-paper/40" />
            </g>
          )}
          <Label x={f.x + 32} y={222} anchor="middle" className="fill-muted">
            {f.label}
          </Label>
        </g>
      ))}
      <path d="M104 148 H124" pathLength={100} className="wf-draw-40 fill-none stroke-paper/50" />
      <path d="M196 148 H216" pathLength={100} className="wf-draw-50 fill-none stroke-paper/50" />
    </Canvas>
  );
}

/* 02 — Selection: scan a long recording, find moments with context. */
export function SelectionArt() {
  const wave = bars(64, 1);
  const step = 4.5;
  const x0 = 16;
  const candidates = [
    { from: 7, to: 13, label: "01", cls: "wf-in-15" },
    { from: 27, to: 35, label: "02", cls: "wf-in-30" },
    { from: 47, to: 53, label: "03", cls: "wf-in-50" },
  ];
  const span = (c: (typeof candidates)[number]) => ({
    x: x0 + c.from * step - 2,
    w: (c.to - c.from) * step + 4,
  });
  const c2 = span(candidates[1]);
  return (
    <Canvas>
      <Label x={16} y={22}>SOURCE · 1:52:08</Label>
      {wave.map((h, i) => (
        <rect
          key={i}
          x={x0 + i * step}
          y={66 - h * 22}
          width="2.5"
          height={h * 44}
          className="fill-muted/50"
        />
      ))}
      {candidates.map((c) => {
        const s = span(c);
        return (
          <g key={c.label} className={c.cls}>
            <rect x={s.x} y="36" width={s.w} height="60" className="fill-tally/10 stroke-tally" />
            <Label x={s.x + s.w / 2} y={110} anchor="middle" className="fill-tally">
              {c.label}
            </Label>
          </g>
        );
      })}
      <rect x="16" y="30" width="34" height="72" className="wf-scan fill-paper/5 stroke-paper/80" />

      <path
        d={`M${c2.x + c2.w / 2} 116 V144`}
        pathLength={100}
        className="wf-draw-62 fill-none stroke-tally"
        strokeWidth="1.5"
      />
      <g className="wf-in-70">
        <rect x="40" y="144" width="240" height="72" className="fill-ink stroke-tally" />
        <Label x={54} y={164} className="fill-tally">CANDIDATE 02</Label>
        <Label x={54} y={184} size={11} className="fill-paper">31:17 → 31:30</Label>
        <Label x={54} y={203} className="fill-muted">SETUP KEPT · SENTENCE WHOLE</Label>
      </g>
    </Canvas>
  );
}

/* 03 — Edit: trim, ripple, split on a real multi-track timeline. */
export function EditArt() {
  const wave = bars(58, 3);
  return (
    <Canvas>
      <Label x={16} y={66}>V1</Label>
      <Label x={16} y={106}>V2</Label>
      <Label x={16} y={146}>A1</Label>
      {[48, 88, 128].map((y) => (
        <rect key={y} x="40" y={y} width="264" height="28" className="fill-ink" />
      ))}

      {/* V1: trim the first clip, then ripple the next one into the gap */}
      <rect x="44" y="49" width="100" height="26" className="wf-trim fill-track-video" />
      <rect x="142" y="46" width="3" height="32" className="wf-handle fill-tally" />
      <rect x="146" y="49" width="120" height="26" className="wf-ripple fill-track-video-alt" />

      {/* V2: split with the razor */}
      <rect x="96" y="89" width="100" height="26" className="fill-track-video-alt" />
      <rect x="196" y="89" width="80" height="26" className="wf-split fill-track-video-alt" />
      <g className="wf-in-45">
        <path d="M199 82 V122" className="stroke-tally" strokeDasharray="3 2" />
        <path d="M195 80 L199 85 L203 80 Z" className="fill-tally" />
      </g>

      {/* A1 */}
      {wave.map((h, i) => (
        <rect
          key={i}
          x={44 + i * 4.4}
          y={142 - h * 11}
          width="2.4"
          height={h * 22}
          className="fill-muted/50"
        />
      ))}

      <g className="wf-playhead">
        <path d="M44 38 V162" className="stroke-tally" />
        <path d="M40 34 L44 39 L48 34 Z" className="fill-tally" />
      </g>

      {[
        { x: 40, label: "TRIM", cls: "wf-in-10" },
        { x: 112, label: "RIPPLE", cls: "wf-in-30" },
        { x: 196, label: "SPLIT", cls: "wf-in-45" },
      ].map((t) => (
        <g key={t.label}>
          <rect x={t.x} y="186" width="68" height="26" className="fill-none stroke-line-strong" />
          <Label x={t.x + 34} y={203} anchor="middle" className="fill-faint">
            {t.label}
          </Label>
          <g className={t.cls}>
            <rect x={t.x} y="186" width="68" height="26" className="fill-tally/10 stroke-tally" />
            <Label x={t.x + 34} y={203} anchor="middle" className="fill-tally">
              {t.label}
            </Label>
          </g>
        </g>
      ))}
    </Canvas>
  );
}

/* 04 — Review: comments on the timeline, versions, approval. */
export function ReviewArt() {
  return (
    <Canvas>
      <rect x="16" y="16" width="188" height="112" className="fill-ink stroke-line-strong" />
      <Person cx={110} top={36} scale={1.25} tone="a" />
      <rect x="40" y="108" width="140" height="7" className="fill-paper/60" />

      <path d="M16 142 H204" className="stroke-line-strong" />
      <rect x="16" y="141" width="128" height="2" className="fill-paper/60" />
      <g className="wf-in-15">
        <circle cx="52" cy="142" r="4.5" className="fill-tally" />
      </g>
      <g className="wf-in-40">
        <circle cx="132" cy="142" r="4.5" className="fill-tally" />
      </g>

      <g className="wf-in-20">
        <rect x="16" y="156" width="188" height="28" className="fill-ink stroke-line" />
        <circle cx="30" cy="170" r="6" className="fill-speaker-b/70" />
        <Label x={42} y={173} size={9} className="fill-paper">00:14  Tighten the open</Label>
      </g>
      <g className="wf-in-45">
        <rect x="16" y="190" width="188" height="28" className="fill-ink stroke-line" />
        <circle cx="30" cy="204" r="6" className="fill-speaker-a/70" />
        <Label x={42} y={207} size={9} className="fill-paper">00:31  Keep this reaction</Label>
      </g>

      <Label x={216} y={28}>VERSIONS</Label>
      {["v1  ROUGH", "v2  NOTES", "v3  FINAL"].map((v, i) => (
        <g key={v}>
          <rect x="216" y={38 + i * 22} width="88" height="18" className="fill-ink stroke-line" />
          <Label x={224} y={50 + i * 22} className="fill-muted">{v}</Label>
        </g>
      ))}
      <rect x="216" y="38" width="88" height="18" className="wf-version fill-tally/15 stroke-tally" />

      <g className="wf-in-80">
        <path d="M218 124 L222 128 L229 119" className="fill-none stroke-tally" strokeWidth="1.5" />
        <Label x={234} y={127} className="fill-paper">CLIENT</Label>
        <Label x={216} y={142} className="fill-muted">SIGNED OFF</Label>
      </g>

      <g className="wf-stamp">
        <rect x="58" y="48" width="104" height="32" className="fill-ink/70 stroke-tally" strokeWidth="2" />
        <text x="110" y="69" fontSize="13" textAnchor="middle" letterSpacing="2" className="font-mono font-bold fill-tally">
          APPROVED
        </text>
      </g>
    </Canvas>
  );
}

/* 05 — Render: frames render, the output is verified. */
export function RenderArt() {
  const frames = Array.from({ length: 8 }, (_, i) => i);
  const sprockets = Array.from({ length: 24 }, (_, i) => i);
  return (
    <Canvas>
      <Label x={16} y={30}>EXPORT · 9:16 · H.264</Label>
      <rect x="16" y="44" width="288" height="84" className="fill-ink-raised" />
      {sprockets.map((i) => (
        <g key={i}>
          <rect x={20 + i * 12} y="48" width="6" height="5" className="fill-ink-sunken" />
          <rect x={20 + i * 12} y="119" width="6" height="5" className="fill-ink-sunken" />
        </g>
      ))}
      {frames.map((i) => {
        const x = 22 + i * 35.5;
        return (
          <g key={i}>
            <rect x={x} y="60" width="31" height="52" className="fill-ink-sunken stroke-line-strong" />
            <g className={`wf-in-${10 + i * 10}`}>
              <rect x={x} y="60" width="31" height="52" className="fill-track-video-alt" />
              <circle cx={x + 15.5} cy="76" r="5" className={i % 3 === 1 ? "fill-speaker-b/80" : "fill-speaker-a/80"} />
              <rect x={x + 6} y="88" width="19" height="12" rx="5" className={i % 3 === 1 ? "fill-speaker-b/50" : "fill-speaker-a/50"} />
              <rect x={x + 5} y="103" width="21" height="3" className={i === 3 ? "fill-tally" : "fill-paper/80"} />
            </g>
          </g>
        );
      })}

      <rect x="16" y="146" width="288" height="4" className="fill-line" />
      <rect x="16" y="146" width="288" height="4" className="wf-progress fill-tally" />

      <g className="wf-until-85">
        <Label x={16} y={172} className="fill-muted">RENDERING…</Label>
      </g>
      <g className="wf-in-85">
        <Label x={16} y={172} className="fill-tally">VERIFIED</Label>
        {["1080×1920", "00:45.00 DURATION", "AUDIO IN SYNC"].map((t, i) => (
          <g key={t}>
            <path
              d={`M16 ${189 + i * 16} l4 4 l7 -8`}
              className="fill-none stroke-tally"
              strokeWidth="1.5"
            />
            <Label x={34} y={194 + i * 16} className="fill-paper">{t}</Label>
          </g>
        ))}
      </g>
    </Canvas>
  );
}

/* 06 — Distribute: one master, every format, scheduled and confirmed. */
export function DistributeArt() {
  const outputs = [
    { y: 46, w: 28, h: 50, ratio: "9:16", slot: "TUE 18:00", draw: "wf-draw-10", show: "wf-in-15", sched: "wf-in-40", pub: "wf-in-60" },
    { y: 118, w: 44, h: 44, ratio: "1:1", slot: "WED 12:30", draw: "wf-draw-20", show: "wf-in-25", sched: "wf-in-45", pub: "wf-in-70" },
    { y: 190, w: 64, h: 36, ratio: "16:9", slot: "FRI 09:00", draw: "wf-draw-30", show: "wf-in-35", sched: "wf-in-50", pub: "wf-in-80" },
  ];
  return (
    <Canvas>
      <rect x="16" y="90" width="58" height="58" className="fill-track-video stroke-paper/70" />
      <path d="M39 108 L55 119 L39 130 Z" className="fill-paper/80" />
      <Label x={16} y={164} className="fill-muted">MASTER</Label>

      {outputs.map((o) => (
        <g key={o.ratio}>
          <path
            d={`M74 119 C104 119 100 ${o.y} 126 ${o.y}`}
            pathLength={100}
            className={`${o.draw} fill-none stroke-paper/40`}
          />
          <g className={o.show}>
            <rect
              x={126}
              y={o.y - o.h / 2}
              width={o.w}
              height={o.h}
              className="fill-track-video-alt stroke-paper/70"
            />
            <rect x={130} y={o.y + o.h / 2 - 8} width={o.w - 8} height="3" className="fill-paper/70" />
          </g>
          <g className={o.show}>
            <Label x={204} y={o.y - 6} className="fill-tally">{o.ratio}</Label>
            <Label x={232} y={o.y - 6} className="fill-paper">{o.slot}</Label>
          </g>
          <g className={o.sched}>
            <g className={`wf-until-${o.pub.slice(6)}`}>
              <Label x={204} y={o.y + 10} className="fill-muted">SCHEDULED</Label>
            </g>
          </g>
          <g className={o.pub}>
            <circle cx="207" cy={o.y + 7} r="3" className="fill-tally" />
            <Label x={214} y={o.y + 10} className="fill-paper">PUBLISHED</Label>
          </g>
        </g>
      ))}
    </Canvas>
  );
}
