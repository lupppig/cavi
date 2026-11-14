"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";

/** Monochrome motifs for the frames: abstract production marks, not screenshots. */
function Frame({ i, children, invert }: { i: number; children: ReactNode; invert?: boolean }) {
  return (
    <div
      className={`reel-frame flex flex-col justify-between border-2 p-2 sm:p-3 ${
        invert ? "border-black bg-[var(--white)] text-black" : "border-[var(--white)] bg-black text-[var(--white)]"
      }`}
      style={{ "--i": i } as CSSProperties}
    >
      <div className="mono flex justify-between text-[11px] uppercase">
        <span>F{String(i + 1).padStart(2, "0")}</span>
        <span>●</span>
      </div>
      {children}
    </div>
  );
}

const bars = [0.4, 0.9, 0.6, 1, 0.3, 0.8, 0.5, 0.95, 0.35, 0.7, 0.55, 0.85];

const frames: { invert?: boolean; body: ReactNode }[] = [
  { body: <p className="display text-2xl sm:text-4xl">Hook</p> },
  {
    invert: true,
    body: (
      <div className="bars flex h-10 items-center gap-0.5 sm:h-16 sm:gap-1">
        {bars.map((h, j) => (
          <span key={j} className="block w-full origin-center bg-current" style={{ height: `${h * 100}%`, "--i": j } as CSSProperties} />
        ))}
      </div>
    ),
  },
  { body: <p className="mono text-sm tabular-nums sm:text-xl">00:12:04:17</p> },
  {
    invert: true,
    body: (
      <div className="relative mx-auto h-12 w-20 border-2 border-current sm:h-20 sm:w-36">
        <div className="absolute inset-y-0 left-1/2 w-[30%] -translate-x-1/2 bg-current" />
        <span className="mono absolute -bottom-4 left-0 text-[10px] sm:-bottom-5 sm:text-[11px]">16:9 → 9:16</span>
      </div>
    ),
  },
  { body: <p className="serif text-xl leading-tight sm:text-3xl">keep the setup.</p> },
  {
    invert: true,
    body: (
      <div className="mx-auto grid size-12 place-items-center border-2 border-current sm:size-20">
        <div className="grid size-7 place-items-center border-2 border-current sm:size-12">
          <div className="size-2.5 bg-current sm:size-4" />
        </div>
      </div>
    ),
  },
  { body: <p className="display text-2xl sm:text-4xl">Cut</p> },
  {
    invert: true,
    body: (
      <div className="mono flex items-center justify-between text-[11px] font-bold sm:text-sm">
        <span className="border-2 border-current px-1.5">IN</span>
        <span className="mx-2 h-0.5 flex-1 bg-current" />
        <span className="border-2 border-current px-1.5">OUT</span>
      </div>
    ),
  },
  { body: <p className="serif text-xl leading-tight sm:text-3xl">land the payoff.</p> },
  {
    invert: true,
    body: (
      <div className="h-10 sm:h-16 bg-[repeating-linear-gradient(135deg,currentColor_0_6px,transparent_6px_14px)]" />
    ),
  },
];

/**
 * A ring of ten frames spinning in 3D. The ring tilts toward the pointer
 * and leans as the page scrolls. Decorative.
 */
export function Reel() {
  const tilt = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = tilt.current;
    if (!el || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    let px = 0;
    let py = 0;
    let raf = 0;
    const apply = () => {
      raf = 0;
      const scroll = Math.min(window.scrollY / window.innerHeight, 1);
      el.style.setProperty("--rx", `${-py * 14 + scroll * 22}deg`);
      el.style.setProperty("--ry", `${px * 22}deg`);
    };
    const queue = () => {
      if (!raf) raf = requestAnimationFrame(apply);
    };
    const move = (e: PointerEvent) => {
      px = e.clientX / window.innerWidth - 0.5;
      py = e.clientY / window.innerHeight - 0.5;
      queue();
    };
    window.addEventListener("pointermove", move, { passive: true });
    window.addEventListener("scroll", queue, { passive: true });
    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("scroll", queue);
      cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <div aria-hidden="true" className="reel-scene grid h-[18rem] place-items-center sm:h-[27rem] xl:h-[30rem]">
      <div ref={tilt} className="reel-tilt">
        <div className="reel-ring h-28 w-36 [--r:14rem] sm:h-44 sm:w-60 sm:[--r:23rem]">
          {frames.map((f, i) => (
            <Frame key={i} i={i} invert={f.invert}>
              {f.body}
            </Frame>
          ))}
        </div>
      </div>
    </div>
  );
}
