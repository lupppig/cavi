"use client";

import { useEffect, useRef, type ReactNode } from "react";

/**
 * Restarts the CSS animations inside it from the beginning the first time
 * it scrolls into view, and pauses them while it is off-screen. Without
 * JavaScript the animations simply run.
 */
export function PlayWhenVisible({ children, className }: { children: ReactNode; className?: string }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    let started = false;
    const animations = () => el.getAnimations({ subtree: true });
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          if (!started) {
            started = true;
            animations().forEach((a) => (a.currentTime = 0));
          }
          animations().forEach((a) => a.play());
        } else {
          animations().forEach((a) => a.pause());
        }
      },
      { threshold: 0.35 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  return (
    <div ref={ref} className={className}>
      {children}
    </div>
  );
}
