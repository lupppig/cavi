"use client";

import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";

/** Monochrome line motifs for each stage. */
const motif: Record<string, ReactNode> = {
  Direction: (
    <g>
      <path d="M10 18 H86 M10 32 H70 M10 46 H80" />
      <path d="M10 60 H46" />
      <path d="M50 54 V66" strokeWidth="4" />
    </g>
  ),
  Selection: (
    <g>
      {[8, 22, 14, 30, 10, 26, 34, 18, 28, 12, 24, 16].map((h, i) => (
        <path key={i} d={`M${12 + i * 7} ${40 - h / 2} V${40 + h / 2}`} />
      ))}
      <path d="M38 8 H30 V72 H38 M70 8 H78 V72 H70" strokeWidth="3.5" />
    </g>
  ),
  Edit: (
    <g>
      <rect x="8" y="14" width="40" height="14" />
      <rect x="52" y="14" width="34" height="14" fill="currentColor" />
      <rect x="20" y="34" width="56" height="14" />
      <rect x="8" y="54" width="70" height="10" />
      <path d="M58 6 V74" strokeWidth="3" />
    </g>
  ),
  Review: (
    <g>
      <rect x="8" y="10" width="78" height="50" />
      <circle cx="30" cy="30" r="6" fill="currentColor" />
      <circle cx="62" cy="40" r="6" />
      <path d="M24 70 H70" />
    </g>
  ),
  Render: (
    <g>
      <rect x="8" y="30" width="78" height="16" />
      <rect x="8" y="30" width="56" height="16" fill="currentColor" />
      <path d="M30 60 L40 70 L62 52" strokeWidth="3.5" />
    </g>
  ),
  Distribute: (
    <g>
      <rect x="6" y="20" width="36" height="22" />
      <rect x="48" y="16" width="26" height="26" />
      <rect x="10" y="48" width="18" height="30" />
      <rect x="36" y="50" width="22" height="28" fill="currentColor" />
    </g>
  ),
};

export type Stage = { title: string; body: string };

function Motif({ title }: { title: string }) {
  return (
    <svg viewBox="0 0 94 80" className="h-20 w-24 sm:h-24 sm:w-28" fill="none" stroke="currentColor" strokeWidth="2.5" aria-hidden="true">
      {motif[title]}
    </svg>
  );
}

/**
 * The six stages on the faces of a hexagonal prism that turns one face
 * per scroll step, with a "scenes unlocked" counter. With reduced motion
 * it is a plain list.
 */
export function StagePrism({ stages }: { stages: Stage[] }) {
  const wrap = useRef<HTMLDivElement>(null);
  const prism = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);
  const [unlocked, setUnlocked] = useState(1);

  useEffect(() => {
    const el = wrap.current;
    if (!el || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    let raf = 0;
    const update = () => {
      raf = 0;
      const r = el.getBoundingClientRect();
      const span = r.height - window.innerHeight;
      const progress = span > 0 ? Math.min(Math.max(-r.top / span, 0), 1) : 0;
      const p = progress * (stages.length - 1);
      prism.current?.style.setProperty("--p", p.toFixed(4));
      const i = Math.round(p);
      setActive(i);
      setUnlocked((u) => Math.max(u, i + 1));
    };
    const queue = () => {
      if (!raf) raf = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", queue, { passive: true });
    window.addEventListener("resize", queue);
    return () => {
      window.removeEventListener("scroll", queue);
      window.removeEventListener("resize", queue);
      cancelAnimationFrame(raf);
    };
  }, [stages.length]);

  return (
    <>
      <div ref={wrap} className="motion-only relative" style={{ height: `${stages.length * 75 + 40}vh` }}>
        <div className="sticky top-0 flex h-dvh items-center overflow-hidden">
          <div className="mx-auto grid w-full max-w-[90rem] grid-cols-1 items-center gap-24 px-4 sm:gap-28 sm:px-8 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
            <div>
              <p className="mono text-xs uppercase opacity-85" aria-live="polite">
                Scenes unlocked {unlocked}/{stages.length}
              </p>
              <div className="mt-2 flex gap-1.5" aria-hidden="true">
                {stages.map((s, i) => (
                  <span key={s.title} className={`h-1.5 flex-1 ${i < unlocked ? "bg-current" : "bg-current/30"}`} />
                ))}
              </div>
              <ol className="mt-6 hidden space-y-1 lg:block">
                {stages.map((s, i) => (
                  <li
                    key={s.title}
                    className={`display flex items-baseline gap-4 text-4xl transition-opacity duration-300 xl:text-5xl ${
                      i === active ? "opacity-100" : "opacity-20"
                    }`}
                    aria-current={i === active ? "step" : undefined}
                  >
                    <span className="mono text-sm font-normal">{String(i + 1).padStart(2, "0")}</span>
                    {s.title}
                  </li>
                ))}
              </ol>
            </div>
            <div className="prism-scene mx-auto w-full max-w-xl" aria-hidden="true">
              <div
                ref={prism}
                className="prism h-[16rem] [--apothem:13.86rem] sm:h-[22rem] sm:[--apothem:19.05rem]"
              >
                {stages.map((s, i) => (
                  <div
                    key={s.title}
                    className="prism-face flex flex-col justify-between border-2 border-current bg-[var(--bg)] p-5 sm:p-7"
                    style={{ "--i": i } as CSSProperties}
                  >
                    <div className="flex items-start justify-between gap-4">
                      <span className="display text-5xl sm:text-8xl">{String(i + 1).padStart(2, "0")}</span>
                      <Motif title={s.title} />
                    </div>
                    <div>
                      <p className="display text-2xl sm:text-5xl">{s.title}</p>
                      <p className="mt-2 text-sm leading-snug sm:text-base">{s.body}</p>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
        {/* The stage text, for assistive tech (the prism is decorative). */}
        <ol className="sr-only">
          {stages.map((s) => (
            <li key={s.title}>
              {s.title}: {s.body}
            </li>
          ))}
        </ol>
      </div>

      <ol className="still-only mx-auto grid max-w-[90rem] grid-cols-1 gap-px px-4 pb-20 sm:grid-cols-2 sm:px-8 lg:grid-cols-3">
        {stages.map((s, i) => (
          <li key={s.title} className="border-2 border-current p-6">
            <div className="flex items-start justify-between">
              <span className="display text-5xl">{String(i + 1).padStart(2, "0")}</span>
              <Motif title={s.title} />
            </div>
            <p className="display mt-4 text-3xl">{s.title}</p>
            <p className="mt-2">{s.body}</p>
          </li>
        ))}
      </ol>
    </>
  );
}
