/**
 * An illustrative brief and the structured creative specification CAVI's
 * Creative Director is being designed to produce from it.
 */

const spec: { key: string; value: string; note?: string }[] = [
  { key: "format", value: "9:16 · 1080×1920" },
  { key: "duration", value: "00:45" },
  { key: "hook", value: "31:12 → 31:19", note: "“So you’d charge less?”" },
  { key: "segments", value: "3 · 31:12–31:58", note: "her full answer kept" },
  { key: "captions", value: "Brand type · word by word" },
  { key: "audio", value: "Speech only · no music under dialogue" },
];

export function DirectionSpec() {
  return (
    <figure className="w-full">
      <div className="grid grid-cols-1 border border-line md:grid-cols-2">
        <div className="border-b border-line p-5 sm:p-6 md:border-r md:border-b-0">
          <p className="font-mono text-[11px] tracking-wider text-faint uppercase">Brief</p>
          <blockquote className="mt-4 text-xl leading-snug sm:text-2xl">
            &ldquo;Turn the second half of this interview into a 45-second vertical
            cut. Lead with the disagreement about pricing, keep her full answer,
            captions in our brand type, no music under speech.&rdquo;
          </blockquote>
        </div>

        <div className="bg-ink-raised p-5 sm:p-6">
          <div className="flex items-center justify-between gap-3">
            <p className="font-mono text-[11px] tracking-wider text-faint uppercase">
              Creative spec · v1
            </p>
            <p className="flex items-center gap-1.5 font-mono text-[11px] tracking-wider text-tally uppercase">
              <span aria-hidden className="rec-dot size-1.5 rounded-full bg-tally" />
              Awaiting your review
            </p>
          </div>
          <dl className="mt-4 divide-y divide-line font-mono text-[13px]">
            {spec.map((row, i) => (
              <div
                key={row.key}
                style={{ "--i": i } as React.CSSProperties}
                className="reveal-line grid grid-cols-[6rem_minmax(0,1fr)] gap-3 py-2.5 sm:grid-cols-[7rem_minmax(0,1fr)]"
              >
                <dt className="text-faint">{row.key}</dt>
                <dd className="min-w-0">
                  <span className="text-paper">{row.value}</span>
                  {row.note && <span className="block text-muted">{row.note}</span>}
                </dd>
              </div>
            ))}
          </dl>
        </div>
      </div>
      <figcaption className="mt-3 font-mono text-[10px] tracking-wider text-faint uppercase sm:text-[11px]">
        Illustrative example · not output from a live system
      </figcaption>
    </figure>
  );
}
