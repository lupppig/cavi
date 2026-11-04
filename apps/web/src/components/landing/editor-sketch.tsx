import type { CSSProperties } from "react";

/**
 * A drawn illustration of CAVI's intended editing workflow. It is built
 * from markup rather than a screenshot so it can never be mistaken for a
 * capture of a working product, and it is labelled as an illustration.
 */

// Highlighted candidate range, as a percentage of the visible timeline.
const RANGE = { from: 34, to: 61 };

// Deterministic waveform so server and client render identically.
const waveform = Array.from({ length: 96 }, (_, i) => {
  const v =
    Math.abs(Math.sin(i * 0.37) * 0.6 + Math.sin(i * 1.13) * 0.3 + Math.sin(i * 2.9) * 0.1);
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

const transcript = [
  { left: 0, width: 18, speaker: "a", text: "…which is why we priced it" },
  { left: 18, width: 16, speaker: "b", text: "Wait — say that again" },
  { left: 34, width: 11, speaker: "b", text: "So you'd charge less?" },
  { left: 45, width: 16, speaker: "a", text: "Less, and I'll tell you why…" },
  { left: 61, width: 21, speaker: "b", text: "Okay, that changes things" },
  { left: 82, width: 18, speaker: "a", text: "Right. Next question" },
];

const captions = [
  { left: 34, width: 5 },
  { left: 39.5, width: 6 },
  { left: 46, width: 4.5 },
  { left: 51, width: 5.5 },
  { left: 57, width: 4 },
];

const ticks = ["31:00", "31:10", "31:20", "31:30", "31:40", "31:50"];

function Lane({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[1.75rem_minmax(0,1fr)] items-stretch gap-2 sm:grid-cols-[2.25rem_minmax(0,1fr)]">
      <span className="self-center font-mono text-[10px] tracking-wider text-faint">{label}</span>
      <div className="relative">{children}</div>
    </div>
  );
}

export function EditorSketch() {
  const rangeStyle: CSSProperties = {
    left: `${RANGE.from}%`,
    width: `${RANGE.to - RANGE.from}%`,
  };

  return (
    <figure className="w-full">
      <div
        role="img"
        aria-label="Illustration of the CAVI editor: a highlighted moment chosen from a long interview, shown on video, audio, transcript and caption tracks, with a vertical preview and caption."
        className="border border-line bg-ink-raised"
      >
        <div aria-hidden="true">
          {/* Top bar */}
          <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2 font-mono text-[10px] tracking-wider text-muted sm:px-4 sm:text-[11px]">
            <span className="truncate">EP. 112 · INTERVIEW · 1:52:08</span>
            <span className="flex shrink-0 items-center gap-1.5 text-paper">
              <span className="size-1.5 rounded-full bg-tally" />
              00:31:24:06
            </span>
          </div>

          <div className="grid grid-cols-1 gap-4 p-3 sm:p-4 md:grid-cols-[9rem_minmax(0,1fr)] md:gap-5 lg:grid-cols-[11rem_minmax(0,1fr)]">
            {/* Vertical program monitor */}
            <div className="mx-auto w-28 sm:w-32 md:w-full">
              <div className="relative aspect-[9/16] overflow-hidden border border-line bg-ink-sunken">
                {/* Abstract frame: two speakers, rule of thirds */}
                <div className="absolute inset-x-0 top-[34%] mx-auto h-[26%] w-[66%] rounded-t-[45%] bg-track-video-alt" />
                <div className="absolute inset-x-0 top-[13%] mx-auto aspect-square w-[34%] rounded-full bg-track-video" />
                <div className="absolute inset-x-[10%] bottom-[22%] text-center text-[11px] leading-tight font-extrabold tracking-tight uppercase font-stretch-[75%] sm:text-[13px]">
                  So you&rsquo;d charge <span className="text-tally">less?</span>
                </div>
                <div className="absolute inset-x-0 bottom-2 text-center font-mono text-[8px] tracking-widest text-faint">
                  9:16 · 1080×1920
                </div>
              </div>
            </div>

            {/* Timeline */}
            <div className="min-w-0 space-y-2">
              {/* Ruler */}
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
                {/* Highlighted candidate range across all lanes */}
                <div
                  className="pointer-events-none absolute inset-y-0 z-10 ml-[calc(1.75rem+0.5rem)] sm:ml-[calc(2.25rem+0.5rem)]"
                  style={{ left: 0, right: 0 }}
                >
                  <div className="relative h-full">
                    <div
                      className="absolute inset-y-0 border-x border-tally bg-tally/10"
                      style={rangeStyle}
                    />
                    <div
                      className="playhead absolute inset-y-[-4px] w-px bg-tally"
                      style={{ "--from": `${RANGE.from}%`, "--to": `${RANGE.to}%` } as CSSProperties}
                    />
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
                        className="min-w-0 flex-1 bg-muted/60"
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
                        className="absolute inset-y-0 overflow-hidden border-r border-ink-raised bg-ink-sunken px-1.5"
                        style={{ left: `${s.left}%`, width: `${s.width}%` }}
                      >
                        <span
                          className={`absolute inset-x-0 top-0 h-0.5 ${
                            s.speaker === "a" ? "bg-speaker-a" : "bg-speaker-b"
                          }`}
                        />
                        <span className="hidden truncate pt-1.5 text-[10px] leading-tight text-muted lg:block">
                          {s.text}
                        </span>
                      </div>
                    ))}
                  </div>
                </Lane>

                <Lane label="CC">
                  <div className="relative h-4 sm:h-5">
                    {captions.map((c) => (
                      <div
                        key={c.left}
                        className="absolute inset-y-0 bg-paper/80"
                        style={{ left: `${c.left}%`, width: `${c.width}%` }}
                      />
                    ))}
                  </div>
                </Lane>
              </div>

              {/* Candidate note */}
              <Lane label="">
                <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 pt-1 font-mono text-[10px] text-muted sm:text-[11px]">
                  <span className="text-tally">CANDIDATE 02</span>
                  <span>31:12 → 31:39</span>
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
