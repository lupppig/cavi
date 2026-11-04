"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";

/**
 * A drawn, animated illustration of CAVI's intended editing workflow.
 * Built from markup rather than a screenshot so it can never be mistaken
 * for a capture of a working product, and labelled as an illustration.
 *
 * Playback runs in one requestAnimationFrame loop that writes directly to
 * the DOM (no React re-renders), pauses off-screen, and is disabled when
 * the visitor prefers reduced motion.
 */

// Highlighted candidate range, as a percentage of the visible timeline.
const RANGE = { from: 34, to: 61 };

// Ruler covers 31:00 → 31:50, so 1% of the timeline is 0.5 seconds.
const RULER_START_S = 31 * 60;
const SECONDS_PER_PCT = 0.5;
const FPS = 30;

const SCAN_MS = 1800;
const PLAY_MS = 9000;
const HOLD_MS = 1400;

// The position shown before hydration and to reduced-motion visitors.
const STILL_POS = 44;

type Speaker = "a" | "b";

const words: { text: string; start: number; end: number; line: 0 | 1 }[] = [
  { text: "So", start: 34, end: 36.4, line: 0 },
  { text: "you’d", start: 36.4, end: 39.2, line: 0 },
  { text: "charge", start: 39.2, end: 42.2, line: 0 },
  { text: "less?", start: 42.2, end: 45, line: 0 },
  { text: "Less,", start: 45, end: 48, line: 1 },
  { text: "and", start: 48, end: 50, line: 1 },
  { text: "I’ll", start: 50, end: 52.4, line: 1 },
  { text: "tell", start: 52.4, end: 54.8, line: 1 },
  { text: "you", start: 54.8, end: 57.2, line: 1 },
  { text: "why.", start: 57.2, end: 61, line: 1 },
];
const lineSpeaker: Record<0 | 1, Speaker> = { 0: "b", 1: "a" };

// Deterministic waveform so server and client render identically.
const BARS = 96;
const waveform = Array.from({ length: BARS }, (_, i) => {
  const v = Math.abs(
    Math.sin(i * 0.37) * 0.6 + Math.sin(i * 1.13) * 0.3 + Math.sin(i * 2.9) * 0.1,
  );
  const speaking = i % 23 > 2; // short pauses between phrases
  return speaking ? 0.18 + v * 0.82 : 0.06;
});

const videoClips = [
  { left: 0, width: 21, alt: false },
  { left: 21, width: 13, alt: true },
  { left: 34, width: 27, alt: false },
  { left: 61, width: 17, alt: true },
  { left: 78, width: 22, alt: false },
];

const transcript: { left: number; width: number; speaker: Speaker; text: string }[] = [
  { left: 0, width: 18, speaker: "a", text: "…which is why we priced it" },
  { left: 18, width: 16, speaker: "b", text: "Wait, say that again" },
  { left: 34, width: 11, speaker: "b", text: "So you’d charge less?" },
  { left: 45, width: 16, speaker: "a", text: "Less, and I’ll tell you why" },
  { left: 61, width: 21, speaker: "b", text: "Okay, that changes things" },
  { left: 82, width: 18, speaker: "a", text: "Right. Next question" },
];

const ticks = ["31:00", "31:10", "31:20", "31:30", "31:40", "31:50"];

function wordIndexAt(pos: number) {
  for (let i = words.length - 1; i >= 0; i--) if (pos >= words[i].start) return i;
  return -1;
}

function timecodeAt(pos: number) {
  const total = RULER_START_S + pos * SECONDS_PER_PCT;
  const m = Math.floor(total / 60);
  const s = Math.floor(total % 60);
  const f = Math.floor((total % 1) * FPS);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `00:${pad(m)}:${pad(s)}:${pad(f)}`;
}

function wordState(i: number, active: number) {
  return i === active ? "active" : i < active ? "past" : "future";
}

const stillWord = wordIndexAt(STILL_POS);
const stillLine = words[stillWord].line;

function Lane({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[1.75rem_minmax(0,1fr)] items-stretch gap-2 sm:grid-cols-[2.25rem_minmax(0,1fr)]">
      <span className="self-center font-mono text-[10px] tracking-wider text-faint">{label}</span>
      <div className="relative">{children}</div>
    </div>
  );
}

export function EditorSketch() {
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    const q = <T extends Element>(sel: string) => Array.from(root.querySelectorAll<T>(sel));
    const timecode = root.querySelector<HTMLElement>("[data-timecode]");
    const monitor = root.querySelector<HTMLElement>("[data-monitor]");
    const range = root.querySelector<HTMLElement>("[data-range]");
    const monitorWords = q<HTMLElement>("[data-word]");
    const ccBlocks = q<HTMLElement>("[data-cc]");
    const lines = q<HTMLElement>("[data-line]");
    const segments = q<HTMLElement>("[data-segment]");
    const bars = q<HTMLElement>("[data-bar]");

    let activeWord = -2;
    let hotBars: number[] = [];
    let raf = 0;
    let running = false;
    let start = 0;
    let elapsedAtPause = 0;
    let scanned = false;

    const applyPosition = (pos: number) => {
      root.style.setProperty("--playhead", `${pos}%`);
      if (timecode) timecode.textContent = timecodeAt(pos);

      const w = wordIndexAt(pos);
      if (w !== activeWord) {
        activeWord = w;
        monitorWords.forEach((el, i) => (el.dataset.state = wordState(i, w)));
        ccBlocks.forEach((el, i) => (el.dataset.state = wordState(i, w)));
        const line = w >= 0 ? words[w].line : 0;
        lines.forEach((el) => (el.dataset.active = String(Number(el.dataset.line) === line)));
        if (monitor) monitor.dataset.speaker = lineSpeaker[line];
        segments.forEach((el) => {
          const l = Number(el.dataset.left);
          const r = l + Number(el.dataset.width);
          el.dataset.active = String(pos >= l && pos < r);
        });
      }

      const center = Math.round((pos / 100) * (BARS - 1));
      hotBars.forEach((i) => bars[i]?.removeAttribute("data-hot"));
      hotBars = [];
      for (let i = center - 2; i <= center + 2; i++) {
        if (bars[i]) {
          bars[i].setAttribute("data-hot", "");
          hotBars.push(i);
        }
      }
    };

    const frame = (now: number) => {
      const elapsed = now - start;
      if (!scanned) {
        const t = Math.min(elapsed / SCAN_MS, 1);
        const eased = 1 - Math.pow(1 - t, 3);
        root.style.setProperty("--scan", `${eased * 100}%`);
        root.dataset.phase = "scan";
        if (t >= 1) {
          scanned = true;
          root.dataset.phase = "play";
          if (range) range.dataset.visible = "true";
          start = now;
        }
      } else {
        const cycle = elapsed % (PLAY_MS + HOLD_MS);
        const t = Math.min(cycle / PLAY_MS, 1);
        applyPosition(RANGE.from + (RANGE.to - RANGE.from) * t);
      }
      raf = requestAnimationFrame(frame);
    };

    const play = () => {
      if (running) return;
      running = true;
      start = performance.now() - elapsedAtPause;
      raf = requestAnimationFrame(frame);
    };
    const pause = () => {
      if (!running) return;
      running = false;
      elapsedAtPause = performance.now() - start;
      cancelAnimationFrame(raf);
    };

    // Begin with the analysis sweep: hide the candidate until it is "found".
    if (range) range.dataset.visible = "false";
    applyPosition(RANGE.from);

    let onScreen = false;
    const sync = () => (onScreen && !document.hidden ? play() : pause());
    const io = new IntersectionObserver(([entry]) => {
      onScreen = entry.isIntersecting;
      sync();
    });
    io.observe(root);
    document.addEventListener("visibilitychange", sync);

    return () => {
      io.disconnect();
      document.removeEventListener("visibilitychange", sync);
      cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <figure className="w-full">
      <div
        ref={rootRef}
        role="img"
        aria-label="Illustration of the CAVI editor: a highlighted moment chosen from a long interview plays back across video, audio, transcript and caption tracks, with a vertical preview showing word-by-word captions."
        className="group/editor border border-line bg-ink-raised"
        data-phase="still"
        style={{ "--playhead": `${STILL_POS}%`, "--scan": "0%" } as CSSProperties}
      >
        <div aria-hidden="true">
          {/* Top bar */}
          <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2 font-mono text-[10px] tracking-wider text-muted sm:px-4 sm:text-[11px]">
            <span className="truncate">
              <span className="group-data-[phase=scan]/editor:hidden">EP. 112 · INTERVIEW · 1:52:08</span>
              <span className="hidden text-paper group-data-[phase=scan]/editor:inline">
                ANALYSING EP. 112…
              </span>
            </span>
            <span className="flex shrink-0 items-center gap-1.5 text-paper tabular-nums">
              <span className="rec-dot size-1.5 rounded-full bg-tally" />
              <span data-timecode>{timecodeAt(STILL_POS)}</span>
            </span>
          </div>

          <div className="grid grid-cols-1 gap-4 p-3 sm:p-4 md:grid-cols-[9rem_minmax(0,1fr)] md:gap-5 lg:grid-cols-[11rem_minmax(0,1fr)]">
            {/* Vertical program monitor */}
            <div className="mx-auto w-32 sm:w-36 md:w-full">
              <div
                data-monitor
                data-speaker={lineSpeaker[stillLine]}
                className="group/monitor relative aspect-[9/16] overflow-hidden border border-line bg-ink-sunken"
              >
                <span className="absolute top-2 left-2 font-mono text-[8px] tracking-widest text-faint">
                  <span className="group-data-[speaker=a]/monitor:hidden">SPEAKER B</span>
                  <span className="hidden group-data-[speaker=a]/monitor:inline">SPEAKER A</span>
                </span>
                {/* Abstract subject; tone shifts with the active speaker */}
                <div className="absolute inset-x-0 top-[34%] mx-auto h-[26%] w-[66%] rounded-t-[45%] bg-speaker-b/45 transition-colors duration-500 group-data-[speaker=a]/monitor:bg-speaker-a/45" />
                <div className="absolute inset-x-0 top-[13%] mx-auto aspect-square w-[34%] rounded-full bg-speaker-b/60 transition-colors duration-500 group-data-[speaker=a]/monitor:bg-speaker-a/60" />

                {/* Word-by-word captions */}
                <div className="absolute inset-x-[6%] bottom-[19%] grid text-center text-[11px] leading-tight font-extrabold tracking-tight uppercase font-stretch-[75%] sm:text-[13px]">
                  {([0, 1] as const).map((line) => (
                    <p
                      key={line}
                      data-line={line}
                      data-active={String(line === stillLine)}
                      className="col-start-1 row-start-1 transition-opacity duration-200 data-[active=false]:opacity-0"
                    >
                      {words.map((w, i) =>
                        w.line === line ? (
                          <span
                            key={i}
                            data-word
                            data-state={wordState(i, stillWord)}
                            className="inline-block px-[0.12em] transition-[color,transform] duration-150 data-[state=active]:scale-110 data-[state=active]:text-tally data-[state=future]:text-paper/25"
                          >
                            {w.text}
                          </span>
                        ) : null,
                      )}
                    </p>
                  ))}
                </div>
                <div className="absolute inset-x-0 bottom-2 text-center font-mono text-[8px] tracking-widest text-faint">
                  9:16 · 1080×1920
                </div>
              </div>
            </div>

            {/* Timeline */}
            <div className="min-w-0 space-y-2">
              <Lane label="">
                <div className="flex justify-between font-mono text-[9px] text-faint sm:text-[10px]">
                  {ticks.map((t, i) => (
                    <span key={t} className={i % 2 === 1 ? "hidden sm:inline" : undefined}>
                      {t}
                    </span>
                  ))}
                </div>
              </Lane>

              <div className="relative space-y-2">
                {/* Overlay: analysis sweep, candidate range and playhead */}
                <div className="pointer-events-none absolute inset-y-0 right-0 left-[calc(1.75rem+0.5rem)] z-10 sm:left-[calc(2.25rem+0.5rem)]">
                  <div className="relative h-full">
                    <div
                      className="absolute inset-y-[-4px] hidden w-px bg-paper/80 shadow-[0_0_12px_2px_rgb(242_239_233/0.25)] group-data-[phase=scan]/editor:block"
                      style={{ left: "var(--scan)" }}
                    />
                    <div
                      data-range
                      data-visible="true"
                      className="absolute inset-y-0 border-x border-tally bg-tally/10 transition-opacity duration-500 data-[visible=false]:opacity-0"
                      style={{ left: `${RANGE.from}%`, width: `${RANGE.to - RANGE.from}%` }}
                    />
                    <div
                      className="absolute inset-y-[-4px] w-px bg-tally group-data-[phase=scan]/editor:opacity-0"
                      style={{ left: "var(--playhead)" }}
                    >
                      <span className="absolute -top-1 left-1/2 size-2 -translate-x-1/2 rotate-45 bg-tally" />
                    </div>
                  </div>
                </div>

                <Lane label="V1">
                  <div className="relative h-7 sm:h-9">
                    {videoClips.map((c) => (
                      <div
                        key={c.left}
                        className={`absolute inset-y-0 border-r border-ink-raised ${
                          c.alt ? "bg-track-video-alt" : "bg-track-video"
                        }`}
                        style={{ left: `${c.left}%`, width: `${c.width}%` }}
                      />
                    ))}
                  </div>
                </Lane>

                <Lane label="A1">
                  <div className="flex h-8 items-center gap-px sm:h-10">
                    {waveform.map((h, i) => (
                      <span
                        key={i}
                        data-bar
                        className="min-w-0 flex-1 origin-center bg-muted/55 transition-[background-color,transform] duration-150 data-hot:scale-y-125 data-hot:bg-paper"
                        style={{ height: `${Math.round(h * 100)}%` }}
                      />
                    ))}
                  </div>
                </Lane>

                <Lane label="TX">
                  <div className="relative h-7 sm:h-9">
                    {transcript.map((s) => (
                      <div
                        key={s.left}
                        data-segment
                        data-left={s.left}
                        data-width={s.width}
                        data-active={String(STILL_POS >= s.left && STILL_POS < s.left + s.width)}
                        className="group/seg absolute inset-y-0 overflow-hidden border-r border-ink-raised bg-ink-sunken px-1.5 transition-colors duration-300 data-[active=true]:bg-ink"
                        style={{ left: `${s.left}%`, width: `${s.width}%` }}
                      >
                        <span
                          className={`absolute inset-x-0 top-0 h-0.5 ${
                            s.speaker === "a" ? "bg-speaker-a" : "bg-speaker-b"
                          }`}
                        />
                        <span className="hidden truncate pt-1.5 text-[10px] leading-tight text-muted transition-colors group-data-[active=true]/seg:text-paper lg:block">
                          {s.text}
                        </span>
                      </div>
                    ))}
                  </div>
                </Lane>

                <Lane label="CC">
                  <div className="relative h-4 sm:h-5">
                    {words.map((w, i) => (
                      <div
                        key={i}
                        data-cc
                        data-state={wordState(i, stillWord)}
                        className="absolute inset-y-0 bg-paper/35 transition-colors duration-150 data-[state=active]:bg-tally data-[state=past]:bg-paper/80"
                        style={{ left: `${w.start + 0.15}%`, width: `${w.end - w.start - 0.3}%` }}
                      />
                    ))}
                  </div>
                </Lane>
              </div>

              <Lane label="">
                <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 pt-1 font-mono text-[10px] text-muted sm:text-[11px]">
                  <span className="text-tally">CANDIDATE 02</span>
                  <span>31:17 → 31:30</span>
                  <span className="hidden sm:inline">Setup kept · sentence whole</span>
                </div>
              </Lane>
            </div>
          </div>
        </div>
      </div>
      <figcaption className="mt-3 font-mono text-[10px] tracking-wider text-faint uppercase sm:text-[11px]">
        Product direction · illustration, not a screenshot
      </figcaption>
    </figure>
  );
}
