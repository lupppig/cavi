/**
 * Animated illustration of reframing: one 16:9 source shot, cropped to
 * 1:1, 4:5 and 9:16 while keeping the active speaker in frame. Pure CSS;
 * see the `reframe` keyframes in globals.css.
 */

const formats = [
  { ratio: "16:9", note: "Source · both speakers" },
  { ratio: "1:1", note: "Square · speaker B" },
  { ratio: "4:5", note: "Portrait · speaker B" },
  { ratio: "9:16", note: "Vertical · follows speaker A" },
];

function Person({ x, tone }: { x: number; tone: "a" | "b" }) {
  const color = tone === "a" ? "bg-speaker-a" : "bg-speaker-b";
  // Sizes are percentages of the 16:9 stage, so the scene scales as one.
  return (
    <>
      <div
        className={`absolute top-[20%] aspect-square w-[8%] -translate-x-1/2 rounded-full ${color} opacity-70`}
        style={{ left: `${x}%` }}
      />
      <div
        className={`absolute top-[42%] h-[42%] w-[19%] -translate-x-1/2 rounded-t-[45%] ${color} opacity-45`}
        style={{ left: `${x}%` }}
      />
    </>
  );
}

export function ReframeSketch() {
  return (
    <figure className="w-full">
      <div
        role="img"
        aria-label="Illustration: a wide two-person interview shot reframed to square, portrait and vertical, keeping whoever is speaking in frame."
        className="relative aspect-video w-full overflow-hidden border border-line bg-ink-sunken"
      >
        <div aria-hidden="true" className="absolute inset-0">
          {/* Scene */}
          <div className="absolute top-[10%] left-[6%] h-[46%] w-[20%] border border-line-strong" />
          <div className="absolute top-[18%] right-[7%] h-px w-[18%] bg-line-strong" />
          <div className="absolute top-[30%] right-[7%] h-px w-[14%] bg-line-strong" />
          <Person x={32} tone="a" />
          <Person x={68} tone="b" />
          <div className="absolute right-[18%] bottom-[16%] left-[18%] h-[5%] bg-line-strong" />

          {/* Moving crop frame; everything outside it is dimmed */}
          <div className="reframe absolute inset-y-0 border border-paper/90 shadow-[0_0_0_200vmax_rgb(13_12_11/0.72)]">
            <span className="rec-dot absolute top-2 left-2 size-1.5 rounded-full bg-tally" />
            {/* Corner marks */}
            <span className="absolute top-0 right-0 size-3 border-t-2 border-r-2 border-paper" />
            <span className="absolute bottom-0 left-0 size-3 border-b-2 border-l-2 border-paper" />
          </div>
        </div>
      </div>

      <div aria-hidden="true" className="relative mt-4 h-6 font-mono text-[11px] tracking-wider uppercase">
        {formats.map((f, k) => (
          <p
            key={f.ratio}
            className="reframe-label absolute inset-0 flex items-baseline gap-3"
            style={{ animationDelay: `calc(var(--reframe-step) * -${(4 - k) % 4})` }}
          >
            <span className="text-tally">{f.ratio}</span>
            <span className="text-muted">{f.note}</span>
          </p>
        ))}
      </div>
      <figcaption className="mt-2 font-mono text-[10px] tracking-wider text-faint uppercase sm:text-[11px]">
        Product direction · illustration, not a screenshot
      </figcaption>
    </figure>
  );
}
