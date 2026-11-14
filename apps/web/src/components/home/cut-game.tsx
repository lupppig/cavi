"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";

type Kind = "filler" | "setup" | "payoff";
type Line = { who: string; text: string; kind: Kind };
type Round = { name: string; lines: Line[] };

/** Original scripts. Each has dead air, a setup and a payoff. */
const rounds: Round[] = [
  {
    name: "Podcast",
    lines: [
      { who: "Host", text: "Okay, mics are on. Where were we?", kind: "filler" },
      { who: "Guest", text: "Honestly, I’ve lost track.", kind: "filler" },
      { who: "Host", text: "Your first ever gig. What happened?", kind: "setup" },
      { who: "Guest", text: "I plug in, and the whole pub’s power goes out.", kind: "setup" },
      { who: "Guest", text: "So I stood on a chair and played it unplugged.", kind: "payoff" },
      { who: "Guest", text: "Still the best gig I’ve ever played.", kind: "payoff" },
      { who: "Host", text: "Love it. Right, quick sponsor bit.", kind: "filler" },
    ],
  },
  {
    name: "Live stream",
    lines: [
      { who: "Streamer", text: "Give me a sec, chat, grabbing water.", kind: "filler" },
      { who: "Streamer", text: "One HP. Three of them left. No heals.", kind: "setup" },
      { who: "Streamer", text: "If I miss this jump, it’s over.", kind: "setup" },
      { who: "Streamer", text: "I made it. I actually made it!", kind: "payoff" },
      { who: "Streamer", text: "Somebody clip that. Clip that now.", kind: "payoff" },
      { who: "Streamer", text: "Okay, back to the menu.", kind: "filler" },
    ],
  },
  {
    name: "Tutorial",
    lines: [
      { who: "Baker", text: "Let me just share my screen.", kind: "filler" },
      { who: "Baker", text: "Everyone asks why their sourdough comes out flat.", kind: "setup" },
      { who: "Baker", text: "It’s almost never the flour.", kind: "setup" },
      { who: "Baker", text: "It’s the water. Keep it warm and it rises.", kind: "payoff" },
      { who: "Baker", text: "Anyway, links are in the description.", kind: "filler" },
    ],
  },
];

const LEAD_IN = 1800; // ms of silence before the first line
const TAIL = 1400;
const PX_PER_MS = 0.11;
const TOLERANCE = 300; // human reaction time is free
const speeds = [1, 1.25, 1.5];
const BEST_KEY = "cavi.cut-game.best";

type Timed = Line & { start: number; end: number };

function timeline(r: Round) {
  let t = LEAD_IN;
  const lines: Timed[] = r.lines.map((l) => {
    const d = l.text.split(/\s+/).length * 290 + 450;
    const out = { ...l, start: t, end: t + d };
    t += d;
    return out;
  });
  const setupStart = lines.find((l) => l.kind === "setup")!.start;
  const payoffEnd = [...lines].reverse().find((l) => l.kind === "payoff")!.end;
  return { lines, total: t + TAIL, setupStart, payoffEnd };
}

type Result = { score: number; verdict: string; inMS: number | null; outMS: number | null };

function judge(inMS: number | null, outMS: number | null, setupStart: number, payoffEnd: number): Result {
  if (inMS === null) return { score: 0, verdict: "No cut. The moment came and went.", inMS, outMS };
  const out = outMS ?? Infinity;
  const late = inMS - setupStart; // > 0 cuts into the setup
  const early = payoffEnd - out; // > 0 cuts off the payoff
  const pen = (err: number, hard: number, soft: number) =>
    err > TOLERANCE ? (err - TOLERANCE) / hard : err < -TOLERANCE ? (-err - TOLERANCE) / soft : 0;
  const inPen = pen(late, 22, 90);
  const outPen = out === Infinity ? 35 : pen(early, 22, 90);
  const score = Math.max(0, Math.min(100, Math.round(100 - inPen - outPen)));
  let verdict: string;
  if (late > 1500) verdict = "You cut into the setup. Nobody will know why it matters.";
  else if (out === Infinity) verdict = "You never called the out point. Clips need an ending.";
  else if (early > 1500) verdict = "You cut before the payoff landed.";
  else if (score >= 92) verdict = "Frame-perfect. Setup kept, payoff landed.";
  else if (late < -1500 || early < -1500) verdict = "Right moment, but trim the dead air.";
  else verdict = "Clean cut. A touch tighter and it’s perfect.";
  return { score, verdict, inMS, outMS };
}

function rank(total: number) {
  if (total >= 270) return "Frame-perfect editor";
  if (total >= 210) return "Clean cutter";
  if (total >= 140) return "Rough cut";
  return "Still in the edit";
}

const noSubscribe = () => () => {};

function readBest() {
  try {
    return Number(window.localStorage.getItem(BEST_KEY)) || 0;
  } catch {
    return 0;
  }
}

type Phase = "intro" | "playing" | "result" | "done";

/**
 * "Cut the moment": a transcript scrolls past the playhead; mark in and
 * out to keep the setup and land the payoff. Three rounds, faster each
 * time. A game about the instinct CAVI is built around, not a product demo.
 */
export function CutGame() {
  const [phase, setPhase] = useState<Phase>("intro");
  const [round, setRound] = useState(0);
  const [results, setResults] = useState<Result[]>([]);
  const [caption, setCaption] = useState<Timed | null>(null);
  const [marks, setMarks] = useState<{ in: number | null; out: number | null }>({ in: null, out: null });
  // The stored best (none on the server), raised by games played here.
  const storedBest = useSyncExternalStore(noSubscribe, readBest, () => 0);
  const [sessionBest, setSessionBest] = useState(0);
  const best = Math.max(storedBest, sessionBest);
  const [newBest, setNewBest] = useState(false);
  const track = useRef<HTMLDivElement>(null);
  const stage = useRef<HTMLDivElement>(null);
  const cutButton = useRef<HTMLButtonElement>(null);
  const clock = useRef({ start: 0, raf: 0, t: 0 });
  const marksRef = useRef(marks);
  const tl = timeline(rounds[round]);
  const speed = speeds[round];

  const finish = useCallback(() => {
    cancelAnimationFrame(clock.current.raf);
    const { setupStart, payoffEnd } = timeline(rounds[round]);
    const m = marksRef.current;
    const r = judge(m.in, m.out, setupStart, payoffEnd);
    setResults((rs) => [...rs.slice(0, round), r]);
    setCaption(null);
    setPhase("result");
  }, [round]);

  const place = useCallback((t: number) => {
    const el = track.current;
    if (!el) return;
    el.style.transition = "none";
    el.style.transform = `translateX(${-t * PX_PER_MS}px)`;
  }, []);

  // After a round, zoom out so the whole timeline (and your cut) fits.
  useEffect(() => {
    if (phase !== "result") return;
    const el = track.current;
    const w = stage.current?.clientWidth ?? 0;
    if (!el || !w) return;
    const width = timeline(rounds[round]).total * PX_PER_MS;
    const scale = Math.min(1, (w - 16) / width);
    el.style.transition = "transform 0.9s cubic-bezier(0.7, 0, 0.2, 1)";
    el.style.transformOrigin = "0 50%";
    el.style.transform = `translateX(${-w / 2 + 8}px) scale(${scale}, 1)`;
  }, [phase, round]);

  // Keyboard players land on the Cut button when a round starts.
  useEffect(() => {
    if (phase === "playing") cutButton.current?.focus({ preventScroll: true });
  }, [phase]);

  const start = useCallback(() => {
    marksRef.current = { in: null, out: null };
    setMarks(marksRef.current);
    setCaption(null);
    setPhase("playing");
    const { lines, total } = timeline(rounds[round]);
    const c = clock.current;
    c.start = performance.now();
    let shown: Timed | null = null;
    const tick = (now: number) => {
      const t = (now - c.start) * speeds[round];
      c.t = t;
      place(t);
      const line = lines.find((l) => t >= l.start && t < l.end) ?? null;
      if (line !== shown) {
        shown = line;
        setCaption(line);
      }
      if (t >= total) {
        finish();
        return;
      }
      c.raf = requestAnimationFrame(tick);
    };
    place(0);
    c.raf = requestAnimationFrame(tick);
  }, [round, finish, place]);

  const cut = useCallback(() => {
    if (phase !== "playing") return;
    const t = clock.current.t;
    const m = marksRef.current;
    if (m.in === null) marksRef.current = { in: t, out: null };
    else if (m.out === null) {
      marksRef.current = { in: m.in, out: t };
      setMarks(marksRef.current);
      finish();
      return;
    }
    setMarks(marksRef.current);
  }, [phase, finish]);

  // Space or Enter cuts while a round is playing.
  useEffect(() => {
    if (phase !== "playing") return;
    const onKey = (e: KeyboardEvent) => {
      // Buttons handle Space and Enter themselves (that's a click).
      const t = e.target;
      if (t instanceof HTMLButtonElement || t instanceof HTMLInputElement || t instanceof HTMLSelectElement || t instanceof HTMLTextAreaElement || t instanceof HTMLAnchorElement) return;
      if (e.key === " " || e.key === "Enter") {
        e.preventDefault();
        cut();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [phase, cut]);

  useEffect(() => () => cancelAnimationFrame(clock.current.raf), []);

  const total = results.reduce((a, r) => a + r.score, 0);
  const last = results[round];

  function next() {
    if (round + 1 < rounds.length) {
      setRound(round + 1);
      setPhase("intro");
      place(0);
      return;
    }
    const finalTotal = total;
    if (finalTotal > best) {
      setSessionBest(finalTotal);
      setNewBest(true);
      try {
        window.localStorage.setItem(BEST_KEY, String(finalTotal));
      } catch {
        // Private mode or blocked storage: the score just isn't kept.
      }
    }
    setPhase("done");
  }

  function restart() {
    cancelAnimationFrame(clock.current.raf);
    setRound(0);
    setResults([]);
    setNewBest(false);
    setCaption(null);
    setPhase("intro");
    place(0);
  }

  const revealed = phase === "result" || phase === "done";
  const x = (ms: number) => ms * PX_PER_MS;

  return (
    <div className="overflow-hidden rounded-2xl border border-[var(--line)] bg-white shadow-sm">
      {/* HUD */}
      <div className="mono grid grid-cols-3 border-b border-[var(--line-strong)] text-xs uppercase sm:text-xs">
        <div className="border-r border-[var(--line-strong)] p-3">
          <p className="opacity-80">Round</p>
          <p className="mt-0.5 text-base font-bold sm:text-lg">
            {round + 1}/{rounds.length} · {rounds[round].name}
          </p>
        </div>
        <div className="border-r border-[var(--line-strong)] p-3">
          <p className="opacity-80">Score</p>
          <p className="mt-0.5 text-base font-bold tabular-nums sm:text-lg">{total}</p>
        </div>
        <div className="p-3">
          <p className="opacity-80">Best</p>
          <p className="mt-0.5 text-base font-bold tabular-nums sm:text-lg">{best || "—"}</p>
        </div>
      </div>

      {/* Caption: what's being said under the playhead */}
      <div className="flex min-h-36 flex-col justify-center px-4 py-6 sm:min-h-44 sm:px-8">
        {phase === "playing" &&
          (caption ? (
            <p className="text-2xl leading-tight font-bold sm:text-4xl">
              <span className="mono mr-2 align-middle text-xs font-normal uppercase opacity-80">{caption.who}</span>
              {caption.text}
            </p>
          ) : (
            <p className="mono text-sm uppercase opacity-80">Rolling…</p>
          ))}
        {phase === "intro" && (
          <div>
            <p className="display text-3xl sm:text-5xl">
              {round === 0 ? "Cut the moment." : `Round ${round + 1}: faster.`}
            </p>
            <p className="mt-3 max-w-2xl text-base sm:text-lg">
              A conversation scrolls past the playhead. Press <strong>Cut</strong> (or Space) once to mark the in point,
              and again for the out point. Keep the setup. Land the payoff. Lose the dead air.
            </p>
          </div>
        )}
        {phase === "result" && last && (
          <div role="status">
            <p className="display text-6xl tabular-nums sm:text-8xl">{last.score}</p>
            <p className="mt-2 text-lg font-semibold sm:text-2xl">{last.verdict}</p>
          </div>
        )}
        {phase === "done" && (
          <div role="status">
            <p className="mono text-xs uppercase opacity-80">{newBest ? "New personal best" : "Final score"}</p>
            <p className="display mt-1 text-6xl tabular-nums sm:text-8xl">
              {total}
              <span className="text-2xl opacity-50 sm:text-4xl">/300</span>
            </p>
            <p className="serif mt-2 text-3xl sm:text-5xl">{rank(total)}.</p>
            <p className="mt-4 max-w-2xl text-base sm:text-lg">
              That instinct (keep the setup, land the payoff) is what CAVI is built around. It suggests the moments, shows
              you every timestamp, and leaves the cut to you.
            </p>
          </div>
        )}
      </div>

      {/* The timeline, tilted back in 3D, scrolling under a fixed playhead */}
      <div ref={stage} className="game-stage relative overflow-hidden border-y border-[var(--line-strong)]" aria-hidden="true">
        <div className="game-track relative h-28 sm:h-32">
          <div className="absolute inset-y-0 left-1/2">
            <div ref={track} className="absolute inset-y-0 left-0 will-change-transform">
              <div className="absolute top-3 left-0 h-[calc(100%-1.5rem)]" style={{ width: x(tl.total) }}>
                {tl.lines.map((l, i) => (
                  <div
                    key={i}
                    className={`absolute inset-y-0 overflow-hidden border border-[var(--line-strong)] px-2 py-1.5 ${
                      revealed && l.kind === "payoff" ? "bg-[var(--accent-strong)] text-white" : ""
                    } ${revealed && l.kind === "filler" ? "opacity-50" : ""} rounded-xl`}
                    style={{
                      left: x(l.start) + 2,
                      width: x(l.end - l.start) - 4,
                      backgroundImage:
                        revealed && l.kind === "setup"
                          ? "repeating-linear-gradient(135deg, color-mix(in srgb, var(--fg) 35%, transparent) 0 2px, transparent 2px 8px)"
                          : undefined,
                    }}
                  >
                    <p className="mono line-clamp-2 text-[11px] leading-tight sm:text-xs">{l.text}</p>
                    {revealed && <p className="mono mt-1 text-[11px] font-bold uppercase">{l.kind}</p>}
                  </div>
                ))}
                {marks.in !== null && (
                  <div className="absolute -inset-y-3 w-1 bg-[var(--accent-strong)]" style={{ left: x(marks.in) }}>
                    <span className="mono absolute -top-0.5 left-1.5 rounded bg-[var(--accent-strong)] px-1 text-[11px] font-bold text-white">IN</span>
                  </div>
                )}
                {marks.out !== null && (
                  <div className="absolute -inset-y-3 w-1 bg-[var(--accent-strong)]" style={{ left: x(marks.out) }}>
                    <span className="mono absolute right-1.5 -bottom-0.5 rounded bg-[var(--accent-strong)] px-1 text-[11px] font-bold text-white">OUT</span>
                  </div>
                )}
              </div>
            </div>
          </div>
          {/* Playhead (hidden once the round is zoomed out for review) */}
          {phase !== "result" && phase !== "done" && (
            <div className="absolute inset-y-0 left-1/2 w-0.5 -translate-x-1/2 bg-[var(--fg)]">
              <span className="absolute top-0 left-1/2 size-3 -translate-x-1/2 rotate-45 bg-[var(--fg)]" />
            </div>
          )}
        </div>
      </div>
      {revealed && (
        <p className="mono flex flex-wrap gap-x-4 gap-y-1 border-b border-[var(--line-strong)] px-4 py-2 text-xs uppercase sm:px-8">
          <span>
            <span className="mr-1 inline-block size-2.5 border border-[var(--line-strong)] bg-[repeating-linear-gradient(135deg,currentColor_0_1px,transparent_1px_4px)] align-middle" />
            Setup
          </span>
          <span>
            <span className="mr-1 inline-block size-2.5 bg-current align-middle" /> Payoff
          </span>
          <span className="opacity-80">Faded: dead air</span>
        </p>
      )}

      {/* Controls */}
      <div className="flex flex-wrap items-center gap-3 p-4 sm:p-6">
        {phase === "intro" && (
          <button type="button" onClick={start} className="btn min-w-40">
            {round === 0 ? "Play" : `Roll round ${round + 1}`}
          </button>
        )}
        {phase === "playing" && (
          <button
            ref={cutButton}
            type="button"
            onClick={cut}
            className="btn min-h-16 min-w-48 text-lg"
            aria-label={marks.in === null ? "Cut: mark the in point" : "Cut: mark the out point"}
          >
            ✂ Cut {marks.in === null ? "in" : "out"}
          </button>
        )}
        {phase === "result" && (
          <button type="button" onClick={next} className="btn min-w-40">
            {round + 1 < rounds.length ? "Next round" : "See final score"}
          </button>
        )}
        {phase === "done" && (
          <>
            <a href="#waitlist" className="btn">
              Join the waitlist
            </a>
            <button type="button" onClick={restart} className="btn btn-ghost">
              Play again
            </button>
          </>
        )}
        {(phase === "playing" || phase === "result") && (
          <button type="button" onClick={restart} className="btn btn-ghost ml-auto">
            Restart
          </button>
        )}
        {phase === "playing" && (
          <p className="mono w-full text-xs uppercase opacity-80">Space or Enter also cuts · {speed}× speed</p>
        )}
      </div>
    </div>
  );
}
