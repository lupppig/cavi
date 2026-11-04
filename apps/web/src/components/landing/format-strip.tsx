const top = ["Podcasts", "Music videos", "Interviews", "Livestreams", "Product ads", "Tutorials"];
const bottom = ["Testimonials", "Release teasers", "Documentaries", "Events", "Vlogs", "Reactions"];

function Row({ items, hidden, star }: { items: string[]; hidden?: boolean; star: string }) {
  return (
    <ul aria-hidden={hidden || undefined} className="flex shrink-0 items-center">
      {items.map((f) => (
        <li key={f} className="flex items-center">
          <span className="px-5 text-3xl font-extrabold tracking-tight whitespace-nowrap sm:px-8 sm:text-5xl">{f}</span>
          <span aria-hidden className={`text-2xl sm:text-4xl ${star}`}>
            ✦
          </span>
        </li>
      ))}
    </ul>
  );
}

/** Two crossing, continuously moving tapes of the formats CAVI is for. */
export function FormatStrip() {
  return (
    <section aria-label="Formats CAVI is built for" className="relative overflow-hidden py-14 sm:py-20">
      <p className="sr-only">
        CAVI is built for {[...top, ...bottom].join(", ").toLowerCase()}.
      </p>
      <div aria-hidden="true" className="tape relative z-10 -mx-4 -rotate-2 border-y-[3px] border-ink bg-cobalt py-3 text-white sm:py-4">
        <div className="marquee-left flex w-max">
          <Row items={top} star="text-sun" />
          <Row items={top} hidden star="text-sun" />
        </div>
      </div>
      <div aria-hidden="true" className="tape relative -mx-4 mt-6 rotate-2 sm:mt-8 border-y-[3px] border-ink bg-sun py-3 text-ink sm:py-4">
        <div className="marquee-right flex w-max">
          <Row items={bottom} star="text-tomato" />
          <Row items={bottom} hidden star="text-tomato" />
        </div>
      </div>
    </section>
  );
}
