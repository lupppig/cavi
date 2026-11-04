import { DirectionSpec } from "@/components/landing/direction-spec";
import { EditorSketch } from "@/components/landing/editor-sketch";
import { FormatStrip } from "@/components/landing/format-strip";
import { PlayWhenVisible } from "@/components/landing/play-when-visible";
import {
  DirectionArt,
  DistributeArt,
  EditArt,
  RenderArt,
  ReviewArt,
  SelectionArt,
} from "@/components/landing/workflow-art";
import { ReframeSketch } from "@/components/landing/reframe-sketch";
import { SiteHeader } from "@/components/landing/site-header";
import { WaitlistForm } from "@/components/landing/waitlist-form";

const stages = [
  {
    title: "Direction",
    Art: DirectionArt,
    body: "Describe the piece you want in plain language. CAVI turns it into a creative brief you can read, question and change.",
  },
  {
    title: "Selection",
    Art: SelectionArt,
    body: "Long recordings searched for moments worth cutting, with the setup kept, sentences whole and every timestamp shown.",
  },
  {
    title: "Edit",
    Art: EditArt,
    body: "A real multi-track timeline. Trim to the frame, reframe for any aspect ratio, caption, layer and undo.",
  },
  {
    title: "Review",
    Art: ReviewArt,
    body: "Share cuts with collaborators and clients, collect approvals, and keep every revision.",
  },
  {
    title: "Render",
    Art: RenderArt,
    body: "What you see in preview is what renders, and every output is checked before it is delivered.",
  },
  {
    title: "Distribute",
    Art: DistributeArt,
    body: "Platform-ready exports, scheduling and publishing, marked as published only once the platform confirms it.",
  },
];

const audiences = [
  {
    name: "Clippers & short-form editors",
    body: "Find the moment, keep the context, and cut it for every vertical feed, in batches, with client delivery built in.",
  },
  {
    name: "Creators, streamers & podcasters",
    body: "Turn one recording into a week of posts that still sound like you and look like your brand.",
  },
  {
    name: "Musicians & artists",
    body: "Teasers, visualisers and release campaigns, with every version kept and every cut on-identity.",
  },
  {
    name: "Freelance editors & studios",
    body: "Client workspaces, brand kits, approvals and revisions in one place instead of a dozen email threads.",
  },
  {
    name: "Agencies & brands",
    body: "Ad variations and campaign assets produced on-brand, reviewed by the right people, and tracked from brief to post.",
  },
];

const principles = [
  {
    title: "Your cut, not the model’s.",
    body: "AI proposes. You decide. Nothing renders or publishes without your say.",
  },
  {
    title: "Real editing.",
    body: "Every control changes the actual project, and the preview matches the export.",
  },
  {
    title: "Permission first.",
    body: "Live clipping only from streams you’re authorised to use. Your media stays private by default.",
  },
];

const headline = ["Make", "more", "of", "what", "moves", "people."];
const closing = ["Be", "first", "in", "the", "edit."];

function SectionLabel({ index, children }: { index: string; children: React.ReactNode }) {
  return (
    <p className="flex items-center gap-3 font-mono text-[11px] tracking-[0.2em] text-muted uppercase">
      <span className="text-tally">{index}</span>
      <span aria-hidden className="h-px w-8 bg-line-strong" />
      {children}
    </p>
  );
}

export default function Home() {
  return (
    <>
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:bg-paper focus:px-4 focus:py-3 focus:text-ink"
      >
        Skip to content
      </a>
      <SiteHeader />

      <main id="main" className="overflow-x-clip">
        {/* Hero */}
        <section id="top" className="mx-auto max-w-7xl px-4 pt-12 pb-20 sm:px-8 sm:pt-20 md:pb-28">
          <p className="flex items-center gap-2 font-mono text-[11px] tracking-[0.12em] text-muted uppercase sm:tracking-[0.2em]">
            <span aria-hidden className="rec-dot size-1.5 shrink-0 rounded-full bg-tally" />
            Creative production · in development
          </p>

          <h1 className="mt-6 max-w-6xl text-[clamp(3rem,11vw,8.25rem)] leading-[0.86] font-extrabold tracking-[-0.03em] uppercase font-stretch-[70%]">
            <span className="sr-only">Make more of what moves people.</span>
            <span aria-hidden="true">
              {headline.map((w, i) => (
                <span key={w}>
                  <span
                    className="word-rise inline-block"
                    style={{ "--i": i } as React.CSSProperties}
                  >
                    <span className={w === "moves" ? "caption-hl" : undefined}>{w}</span>
                  </span>{" "}
                </span>
              ))}
            </span>
          </h1>

          <div className="mt-10 grid grid-cols-1 gap-10 lg:mt-14 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
            <div className="max-w-xl">
              <p className="text-lg leading-relaxed text-muted sm:text-xl">
                We&rsquo;re building CAVI: one production environment that takes
                you from raw footage and a creative idea to finished, on-brand
                edits for every platform, with every creative decision still
                yours.
              </p>
              <div className="mt-8">
                <WaitlistForm />
              </div>
            </div>
            <EditorSketch />
          </div>
        </section>

        <FormatStrip />

        {/* Workflow */}
        <section id="workflow" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="01">Workflow</SectionLabel>
            <h2 className="reveal mt-6 max-w-4xl text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
              From the first frame to the final upload.
            </h2>
            <p className="mt-6 max-w-2xl text-lg text-muted">
              Six stages in one project, so nothing gets lost between the idea
              and the post. This is what we&rsquo;re building; the artwork
              shows how each stage is meant to work.
            </p>

            <ol className="mt-14 grid grid-cols-1 gap-px border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
              {stages.map((s, i) => (
                <li
                  key={s.title}
                  className="reveal group relative flex flex-col overflow-hidden bg-ink transition-colors duration-300 hover:bg-ink-raised"
                  style={{ "--i": i % 3 } as React.CSSProperties}
                >
                  <span
                    aria-hidden
                    className="absolute inset-x-0 top-0 z-10 h-0.5 origin-left scale-x-0 bg-tally transition-transform duration-700 ease-out group-hover:scale-x-100"
                  />
                  <PlayWhenVisible className="aspect-[4/3] border-b border-line bg-ink-sunken p-2 sm:p-3">
                    <s.Art />
                  </PlayWhenVisible>
                  <div className="p-6 sm:p-8">
                    <p className="font-mono text-xs text-faint transition-colors group-hover:text-tally">
                      {String(i + 1).padStart(2, "0")}:00:00:00
                    </p>
                    <h3 className="mt-3 text-2xl font-bold tracking-tight">{s.title}</h3>
                    <p className="mt-3 leading-relaxed text-muted">{s.body}</p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </section>

        {/* Reframing */}
        <section id="reframe" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="02">Reframing</SectionLabel>
            <div className="mt-6 grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
              <div>
                <h2 className="reveal text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
                  One cut. Every frame.
                </h2>
                <p className="mt-6 max-w-xl text-lg leading-relaxed text-muted">
                  We&rsquo;re building reframing that follows whoever is talking,
                  so one edit can go out at 16:9, 1:1, 4:5 and 9:16 without
                  losing the person, the reaction or the moment.
                </p>
              </div>
              <div className="reveal">
                <ReframeSketch />
              </div>
            </div>
          </div>
        </section>

        {/* Creative direction */}
        <section id="direction" className="border-t border-line bg-ink-sunken">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="03">Creative direction</SectionLabel>
            <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
              <h2 className="reveal text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
                Direction you can read, change and approve.
              </h2>
              <p className="max-w-2xl self-end text-lg leading-relaxed text-muted">
                CAVI&rsquo;s Creative Director won&rsquo;t render anything on its
                own. It writes a structured plan: which moments, in what order,
                at what length, with which captions. You review it, edit it, and
                only then does it reach the timeline.
              </p>
            </div>
            <div className="reveal mt-12">
              <DirectionSpec />
            </div>
          </div>
        </section>

        {/* Audiences */}
        <section id="who" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="04">Who it&rsquo;s for</SectionLabel>
            <h2 className="reveal mt-6 max-w-4xl text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
              For people who make things for a living.
            </h2>
            <ul className="mt-14 border-t border-line">
              {audiences.map((a) => (
                <li
                  key={a.name}
                  className="reveal group grid grid-cols-1 gap-3 border-b border-line py-8 md:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] md:gap-16"
                >
                  <h3 className="flex items-center gap-0 text-2xl font-bold tracking-tight transition-[gap] duration-500 group-hover:gap-4 sm:text-3xl">
                    <span
                      aria-hidden
                      className="h-0.5 w-0 bg-tally transition-[width] duration-500 group-hover:w-8"
                    />
                    {a.name}
                  </h3>
                  <p className="max-w-2xl text-lg leading-relaxed text-muted transition-colors duration-300 group-hover:text-paper">
                    {a.body}
                  </p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Principles */}
        <section className="border-t border-line bg-ink-raised">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-24">
            <SectionLabel index="05">Principles</SectionLabel>
            <ul className="mt-10 grid grid-cols-1 gap-10 md:grid-cols-3 md:gap-12">
              {principles.map((p, i) => (
                <li key={p.title} className="reveal" style={{ "--i": i } as React.CSSProperties}>
                  <h3 className="text-2xl font-bold tracking-tight">{p.title}</h3>
                  <p className="mt-3 leading-relaxed text-muted">{p.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Closing CTA */}
        <section id="waitlist" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-32">
            <SectionLabel index="06">Early access</SectionLabel>
            <h2 className="mt-6 text-[clamp(2.75rem,10vw,8rem)] leading-[0.88] font-extrabold tracking-[-0.03em] uppercase font-stretch-[70%]">
              <span className="sr-only">Be first in the edit.</span>
              <span aria-hidden="true">
                {closing.map((w, i) => (
                  <span key={w}>
                    <span className="reveal-word" style={{ "--i": i } as React.CSSProperties}>
                      {w}
                    </span>{" "}
                  </span>
                ))}
              </span>
            </h2>
            <p className="mt-6 max-w-2xl text-lg text-muted">
              CAVI isn&rsquo;t open yet. Join the waitlist and we&rsquo;ll let you
              know when early access begins.
            </p>
            <div className="mt-10 max-w-4xl">
              <WaitlistForm tone="large" />
            </div>
          </div>
        </section>
      </main>

      <footer className="border-t border-line">
        <div className="mx-auto flex max-w-7xl flex-col gap-2 px-4 py-8 font-mono text-[11px] tracking-wider text-faint uppercase sm:flex-row sm:justify-between sm:px-8">
          <span>© 2026 CAVI</span>
          <span>In development</span>
        </div>
      </footer>
    </>
  );
}
