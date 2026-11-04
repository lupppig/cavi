const formats = [
  "Podcasts",
  "Music videos",
  "Interviews",
  "Livestreams",
  "Product ads",
  "Tutorials",
  "Testimonials",
  "Release teasers",
  "Documentaries",
  "Events",
];

function Row({ hidden }: { hidden?: boolean }) {
  return (
    <ul aria-hidden={hidden || undefined} className="flex shrink-0 items-center">
      {formats.map((f, i) => (
        <li key={f} className="flex items-center">
          <span
            className={`px-6 text-4xl font-extrabold tracking-tight uppercase font-stretch-[70%] sm:px-10 sm:text-6xl ${
              i % 2 === 1 ? "text-outline" : "text-paper"
            }`}
          >
            {f}
          </span>
          <span aria-hidden className="rec-dot size-2 shrink-0 rounded-full bg-tally" />
        </li>
      ))}
    </ul>
  );
}

/** A slow, continuously moving strip of the formats CAVI is built for. */
export function FormatStrip() {
  return (
    <section aria-label="Formats CAVI is built for" className="overflow-hidden border-y border-line py-6 sm:py-8">
      <div className="marquee-track flex w-max">
        <Row />
        <Row hidden />
      </div>
    </section>
  );
}
