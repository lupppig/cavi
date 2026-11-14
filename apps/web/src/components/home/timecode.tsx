"use client";

import { useEffect, useState } from "react";

const pad = (n: number) => String(n).padStart(2, "0");

/** A recording timecode (HH:MM:SS:FF at 25 fps) counting from when the page opened. */
export function Timecode() {
  const [ms, setMs] = useState(0);
  useEffect(() => {
    const start = performance.now();
    let raf = 0;
    const tick = () => {
      setMs(performance.now() - start);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, []);
  const s = Math.floor(ms / 1000);
  const frames = Math.floor((ms % 1000) / 40);
  return (
    <span className="tabular-nums" aria-hidden="true">
      {pad(Math.floor(s / 3600))}:{pad(Math.floor(s / 60) % 60)}:{pad(s % 60)}:{pad(frames)}
    </span>
  );
}
