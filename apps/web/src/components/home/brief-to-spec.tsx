"use client";

import { useEffect, useRef, useState } from "react";

const brief =
  "Turn this podcast into a fast vertical clip. Keep the funniest exchange, open on the laugh, and don’t change what they meant.";

const spec: [string, string][] = [
  ["source", "ep-12.mp4 · 00:12:04 → 00:12:51"],
  ["why", "setup at 12:04, payoff at 12:43"],
  ["hook", "open on the laugh (12:43), then rewind"],
  ["shape", "9:16 · framed on the speaker"],
  ["captions", "word by word · bold"],
  ["length", "47 s"],
  ["status", "waiting for your approval"],
];

/**
 * A brief typed in plain words, turned into a readable plan line by line.
 * Illustrative: the Creative Director is in development.
 */
export function BriefToSpec() {
  const root = useRef<HTMLDivElement>(null);
  // Complete by default (server render, no JavaScript); reset for the
  // animation once it runs, while still below the fold.
  const [typed, setTyped] = useState(brief.length);
  const [lines, setLines] = useState(spec.length);

  useEffect(() => {
    const el = root.current;
    if (!el) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    if (el.getBoundingClientRect().top < window.innerHeight) return; // already on screen: leave it
    setTyped(0);
    setLines(0);
    let timer = 0;
    const io = new IntersectionObserver(
      ([e]) => {
        if (!e.isIntersecting) return;
        io.disconnect();
        let i = 0;
        const type = () => {
          i += 1;
          setTyped(i);
          if (i < brief.length) {
            timer = window.setTimeout(type, 22);
            return;
          }
          let j = 0;
          const line = () => {
            j += 1;
            setLines(j);
            if (j < spec.length) timer = window.setTimeout(line, 260);
          };
          timer = window.setTimeout(line, 400);
        };
        timer = window.setTimeout(type, 300);
      },
      { threshold: 0.4 },
    );
    io.observe(el);
    return () => {
      io.disconnect();
      window.clearTimeout(timer);
    };
  }, []);

  return (
    <div ref={root} className="grid grid-cols-1 border-2 border-current lg:grid-cols-2">
      <div className="border-b-2 border-current p-5 sm:p-8 lg:border-r-2 lg:border-b-0">
        <p className="mono text-xs uppercase opacity-80">Your brief</p>
        <p className="serif mt-4 text-3xl leading-tight sm:text-4xl">
          <span className="sr-only">{brief}</span>
          <span aria-hidden="true">
            “{brief.slice(0, typed)}
            {typed < brief.length ? <span className="caret ml-0.5 inline-block h-[0.9em] w-[3px] translate-y-[0.1em] bg-current" /> : "”"}
          </span>
        </p>
      </div>
      <div className="p-5 sm:p-8">
        <p className="mono text-xs uppercase opacity-80">The plan · you can edit every line</p>
        <dl className="mono mt-4 space-y-2 text-sm sm:text-base">
          {spec.map(([k, v], i) => (
            <div
              key={k}
              className={`grid grid-cols-[6.5rem_minmax(0,1fr)] gap-3 border-b border-current/40 pb-2 transition-[opacity,transform] duration-500 ${
                i < lines ? "opacity-100" : "translate-x-4 opacity-0"
              }`}
            >
              <dt className="uppercase opacity-80">{k}</dt>
              <dd className={k === "status" ? "font-bold" : ""}>{v}</dd>
            </div>
          ))}
        </dl>
        <p className="mono mt-5 text-xs uppercase opacity-80">Illustration · the Creative Director is in development</p>
      </div>
    </div>
  );
}
