"use client";

import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { Character, Sparkle } from "./cartoon";

/**
 * Cartoon illustration of CAVI's intended editing workflow: a moment is
 * found in a long interview, then plays back with word-by-word captions
 * while the speakers talk in the vertical preview.
 *
 * Playback is one requestAnimationFrame loop writing straight to the DOM.
 * It pauses off-screen and is disabled for reduced-motion visitors, who
 * see a complete still frame.
 */

const RANGE = { from: 34, to: 61 };
const RULER_START_S = 31 * 60;
const SECONDS_PER_PCT = 0.5;
const FPS = 30;
const SCAN_MS = 1800;
const PLAY_MS = 9000;
const HOLD_MS = 1600;
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
  { text: "why!", start: 57.2, end: 61, line: 1 },
];
const lineSpeaker: Record<0 | 1, Speaker> = { 0: "b", 1: "a" };

const BARS = 64;
const waveform = Array.from({ length: BARS }, (_, i) => {
  const v = Math.abs(Math.sin(i * 0.37) * 0.6 + Math.sin(i * 1.13) * 0.3 + Math.sin(i * 2.9) * 0.1);
  return i % 15 > 1 ? 0.22 + v * 0.78 : 0.12;
});

const videoClips = [
  { left: 0, width: 21, color: "bg-sky" },
  { left: 21, width: 13, color: "bg-pink" },
  { left: 34, width: 27, color: "bg-sun" },
  { left: 61, width: 17, color: "bg-mint" },
  { left: 78, width: 22, color: "bg-sky" },
];

const transcript: { left: number; width: number; speaker: Speaker; text: string }[] = [
  { left: 0, width: 18, speaker: "a", text: "…which is why" },
  { left: 18, width: 16, speaker: "b", text: "Wait, again?" },
  { left: 34, width: 11, speaker: "b", text: "So you’d…" },
  { left: 45, width: 16, speaker: "a", text: "Less, and I’ll…" },
  { left: 61, width: 21, speaker: "b", text: "Okay, wow" },
  { left: 82, width: 18, speaker: "a", text: "Right. Next" },
];

const ticks = ["31:00", "31:10", "31:20", "31:30", "31:40", "31:50"];

function wordIndexAt(pos: number) {
  for (let i = words.length - 1; i >= 0; i--) if (pos >= words[i].start) return i;
  return -1;
}
function timecodeAt(pos: number) {
  const total = RULER_START_S + pos * SECONDS_PER_PCT;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `00:${pad(Math.floor(total / 60))}:${pad(Math.floor(total % 60))}:${pad(Math.floor((total % 1) * FPS))}`;
}
function wordState(i: number, active: number) {
  return i === active ? "active" : i < active ? "past" : "future";
}

const stillWord = wordIndexAt(STILL_POS);
const stillLine = words[stillWord].line;

function Lane({ label, color, children }: { label: string; color: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[2rem_minmax(0,1fr)] items-center gap-2 sm:grid-cols-[2.5rem_minmax(0,1fr)]">
      {label ? (
        <span className={`rounded-full border-2 border-ink ${color} py-0.5 text-center font-mono text-[9px] font-bold sm:text-[10px]`}>
          {label}
        </span>
      ) : (
        <span />
      )}
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

    const applyPosition = (pos: number, talking: boolean) => {
      root.style.setProperty("--playhead", `${pos}%`);
      root.dataset.talking = String(talking);
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
          el.dataset.active = String(pos >= l && pos < l + Number(el.dataset.width));
        });
      }

      const center = Math.round((pos / 100) * (BARS - 1));
      hotBars.forEach((i) => bars[i]?.removeAttribute("data-hot"));
      hotBars = [];
      for (let i = center - 1; i <= center + 1; i++) {
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
        root.style.setProperty("--scan", `${(1 - Math.pow(1 - t, 3)) * 100}%`);
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
        applyPosition(RANGE.from + (RANGE.to - RANGE.from) * t, t < 1);
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
      root.dataset.talking = "false";
    };

    if (range) range.dataset.visible = "false";
    applyPosition(RANGE.from, false);

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
        aria-label="Cartoon illustration of the CAVI editor: a moment is found in a long interview and plays back with word-by-word captions while two speakers talk in a vertical phone preview."
        className="group/editor sticker-lg overflow-hidden rounded-[28px] bg-white"
        data-phase="still"
        data-talking="false"
        style={{ "--playhead": `${STILL_POS}%`, "--scan": "0%" } as CSSProperties}
      >
        <div aria-hidden="true">
          {/* Title bar */}
          <div className="flex items-center justify-between gap-3 border-b-[3px] border-ink bg-sun px-3 py-2.5 sm:px-4">
            <div className="flex min-w-0 items-center gap-2">
              <span className="size-3 shrink-0 rounded-full border-2 border-ink bg-tomato" />
              <span className="size-3 shrink-0 rounded-full border-2 border-ink bg-white" />
              <span className="size-3 shrink-0 rounded-full border-2 border-ink bg-mint" />
              <span className="ml-1 truncate text-sm font-extrabold">
                <span className="group-data-[phase=scan]/editor:hidden">ep. 112 · the pricing chat</span>
                <span className="hidden group-data-[phase=scan]/editor:inline">finding the good bit…</span>
              </span>
            </div>
            <span className="flex shrink-0 items-center gap-1.5 rounded-full bg-ink px-2.5 py-1 font-mono text-[10px] text-white tabular-nums sm:text-[11px]">
              <span className="rec-dot size-2 rounded-full bg-tomato" />
              <span data-timecode>{timecodeAt(STILL_POS)}</span>
            </span>
          </div>

          <div className="grid grid-cols-1 gap-4 p-3 sm:p-4 md:grid-cols-[9.5rem_minmax(0,1fr)] md:gap-5 lg:grid-cols-[11rem_minmax(0,1fr)]">
            {/* Phone preview */}
            <div className="mx-auto w-36 sm:w-40 md:w-full">
              <div
                data-monitor
                data-speaker={lineSpeaker[stillLine]}
                className="group/monitor relative aspect-[9/16] overflow-hidden rounded-[22px] border-[3px] border-ink bg-pink transition-colors duration-300 data-[speaker=a]:bg-sky"
              >
                <div className="absolute top-2 left-1/2 h-1.5 w-10 -translate-x-1/2 rounded-full bg-ink" />
                <svg viewBox="0 0 100 120" className="art absolute inset-x-0 top-[12%] w-full">
                  <g className="transition-opacity duration-200 group-data-[speaker=a]/monitor:opacity-0">
                    <Character x={50} y={52} scale={1.05} skin="var(--skin-2)" hair="bun" hairColor="#5a3420" shirt="var(--sun)" mouth="talk" mouthClass="talk-when-live" blinkDelay="0.5s" />
                  </g>
                  <g className="opacity-0 transition-opacity duration-200 group-data-[speaker=a]/monitor:opacity-100">
                    <Character x={50} y={52} scale={1.05} skin="var(--skin-1)" hair="afro" hairColor="#1d1410" shirt="var(--tomato)" mouth="talk" mouthClass="talk-when-live" blinkDelay="1.4s" />
                  </g>
                </svg>

                {/* Word-by-word captions */}
                <div className="absolute inset-x-[5%] bottom-[16%] grid text-center text-[13px] leading-tight font-extrabold uppercase sm:text-[15px]">
                  {([0, 1] as const).map((line) => (
                    <p
                      key={line}
                      data-line={line}
                      data-active={String(line === stillLine)}
                      className="col-start-1 row-start-1 transition-[opacity,transform] duration-200 data-[active=false]:scale-90 data-[active=false]:opacity-0"
                    >
                      {words.map((w, i) =>
                        w.line === line ? (
                          <span
                            key={i}
                            data-word
                            data-state={wordState(i, stillWord)}
                            className="inline-block px-[0.1em] text-white transition-[color,transform] duration-150 [paint-order:stroke_fill] [-webkit-text-stroke:3px_var(--ink)] data-[state=active]:-rotate-3 data-[state=active]:scale-125 data-[state=active]:text-sun data-[state=future]:opacity-40"
                          >
                            {w.text}
                          </span>
                        ) : null,
                      )}
                    </p>
                  ))}
                </div>
              </div>
            </div>

            {/* Timeline */}
            <div className="min-w-0 space-y-2.5">
              <Lane label="" color="">
                <div className="flex justify-between font-mono text-[9px] font-bold text-ink-soft sm:text-[10px]">
                  {ticks.map((t, i) => (
                    <span key={t} className={i % 2 === 1 ? "hidden sm:inline" : undefined}>
                      {t}
                    </span>
                  ))}
                </div>
              </Lane>

              <div className="relative space-y-2.5">
                <div className="pointer-events-none absolute inset-y-0 right-0 left-[calc(2rem+0.5rem)] z-10 sm:left-[calc(2.5rem+0.5rem)]">
                  <div className="relative h-full">
                    {/* Scan */}
                    <div className="absolute inset-y-[-6px] hidden w-1 -translate-x-1/2 rounded-full bg-cobalt group-data-[phase=scan]/editor:block" style={{ left: "var(--scan)" }}>
                      <span className="absolute -top-3 left-1/2 size-5 -translate-x-1/2 rounded-full border-[3px] border-cobalt bg-white" />
                    </div>
                    {/* Found moment */}
                    <div
                      data-range
                      data-visible="true"
                      className="absolute inset-y-[-4px] rounded-xl border-[3px] border-dashed border-tomato bg-tomato/15 transition-[opacity,transform] duration-500 [transition-timing-function:cubic-bezier(0.34,1.56,0.64,1)] data-[visible=false]:scale-75 data-[visible=false]:opacity-0"
                      style={{ left: `${RANGE.from}%`, width: `${RANGE.to - RANGE.from}%` }}
                    >
                      <span className="absolute -top-7 left-1/2 -translate-x-1/2 -rotate-3 rounded-full border-2 border-ink bg-tomato px-2 py-0.5 text-[10px] font-extrabold whitespace-nowrap text-white sm:text-[11px]">
                        found it!
                      </span>
                    </div>
                    {/* Playhead */}
                    <div className="absolute inset-y-[-6px] w-[3px] -translate-x-1/2 rounded-full bg-ink group-data-[phase=scan]/editor:opacity-0" style={{ left: "var(--playhead)" }}>
                      <span className="absolute -top-2 left-1/2 size-4 -translate-x-1/2 rounded-full border-[3px] border-ink bg-tomato" />
                    </div>
                  </div>
                </div>

                <Lane label="V1" color="bg-sky">
                  <div className="relative h-8 sm:h-10">
                    {videoClips.map((c) => (
                      <div
                        key={c.left}
                        className={`absolute inset-y-0 rounded-lg border-2 border-ink ${c.color}`}
                        style={{ left: `calc(${c.left}% + 1px)`, width: `calc(${c.width}% - 2px)` }}
                      />
                    ))}
                  </div>
                </Lane>

                <Lane label="A1" color="bg-mint">
                  <div className="flex h-9 items-center gap-[2px] sm:h-11">
                    {waveform.map((h, i) => (
                      <span
                        key={i}
                        data-bar
                        className="min-w-0 flex-1 rounded-full bg-ink transition-[background-color,transform] duration-150 data-hot:scale-y-125 data-hot:bg-tomato"
                        style={{ height: `${Math.round(h * 100)}%` }}
                      />
                    ))}
                  </div>
                </Lane>

                <Lane label="TX" color="bg-sun">
                  <div className="relative h-7 sm:h-9">
                    {transcript.map((s) => (
                      <div
                        key={s.left}
                        data-segment
                        data-left={s.left}
                        data-width={s.width}
                        data-active={String(STILL_POS >= s.left && STILL_POS < s.left + s.width)}
                        className="absolute inset-y-0 flex items-center overflow-hidden rounded-md border-2 border-ink bg-white px-1.5 transition-colors duration-200 data-[active=true]:bg-sun"
                        style={{ left: `calc(${s.left}% + 1px)`, width: `calc(${s.width}% - 2px)` }}
                      >
                        <span className={`mr-1 size-2 shrink-0 rounded-full border border-ink ${s.speaker === "a" ? "bg-sky" : "bg-pink"}`} />
                        <span className="hidden truncate text-[10px] font-bold lg:block">{s.text}</span>
                      </div>
                    ))}
                  </div>
                </Lane>

                <Lane label="CC" color="bg-pink">
                  <div className="relative h-5 sm:h-6">
                    {words.map((w, i) => (
                      <div
                        key={i}
                        data-cc
                        data-state={wordState(i, stillWord)}
                        className="absolute inset-y-0 rounded-[4px] border-2 border-ink bg-white transition-[background-color,transform] duration-150 data-[state=active]:-translate-y-0.5 data-[state=active]:bg-tomato data-[state=past]:bg-ink"
                        style={{ left: `${w.start + 0.2}%`, width: `${w.end - w.start - 0.4}%` }}
                      />
                    ))}
                  </div>
                </Lane>
              </div>

              <Lane label="" color="">
                <div className="flex flex-wrap items-center gap-2 pt-1">
                  <span className="rounded-full border-2 border-ink bg-tomato px-2 py-0.5 text-[10px] font-extrabold text-white sm:text-[11px]">
                    moment #2
                  </span>
                  <span className="font-mono text-[10px] font-bold sm:text-[11px]">31:17 → 31:30</span>
                  <span className="hidden text-[11px] font-semibold text-ink-soft sm:inline">setup kept · no cut-off sentences</span>
                </div>
              </Lane>
            </div>
          </div>
        </div>
      </div>
      <figcaption className="mt-4 flex items-center gap-2 text-xs font-semibold text-ink-soft">
        <svg viewBox="0 0 40 40" className="art size-4" aria-hidden="true">
          <Sparkle x={20} y={20} size={15} fill="var(--sun)" />
        </svg>
        Illustration of what we&rsquo;re building, not a screenshot
      </figcaption>
    </figure>
  );
}
