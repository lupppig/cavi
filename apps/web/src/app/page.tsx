import { DirectionSpec } from "@/components/landing/direction-spec";
import { EditorSketch } from "@/components/landing/editor-sketch";
import { SiteHeader } from "@/components/landing/site-header";
import { WaitlistForm } from "@/components/landing/waitlist-form";

const stages = [
  {
    title: "Direction",
    body: "Describe the piece you want in plain language. CAVI turns it into a creative brief you can read, question and change.",
  },
  {
    title: "Selection",
    body: "Long recordings searched for moments worth cutting, with the setup kept, sentences whole and every timestamp shown.",
  },
  {
    title: "Edit",
    body: "A real multi-track timeline. Trim to the frame, reframe for any aspect ratio, caption, layer and undo.",
  },
  {
    title: "Review",
    body: "Share cuts with collaborators and clients, collect approvals, and keep every revision.",
  },
  {
    title: "Render",
    body: "What you see in preview is what renders, and every output is checked before it is delivered.",
  },
  {
    title: "Distribute",
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
            <span aria-hidden className="size-1.5 shrink-0 rounded-full bg-tally" />
            Creative production · in development
          </p>

          <h1 className="mt-6 max-w-6xl text-[clamp(3rem,11vw,8.25rem)] leading-[0.86] font-extrabold tracking-[-0.03em] uppercase font-stretch-[70%]">
            Make more of what moves people.
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

        {/* Workflow */}
        <section id="workflow" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="01">Workflow</SectionLabel>
            <h2 className="mt-6 max-w-4xl text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
              From the first frame to the final upload.
            </h2>
            <p className="mt-6 max-w-2xl text-lg text-muted">
              Six stages in one project, so nothing gets lost between the idea
              and the post. This is what we&rsquo;re building.
            </p>

            <ol className="mt-14 grid grid-cols-1 gap-px border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
              {stages.map((s, i) => (
                <li key={s.title} className="bg-ink p-6 sm:p-8">
                  <p className="font-mono text-xs text-faint">
                    {String(i + 1).padStart(2, "0")}:00:00:00
                  </p>
                  <h3 className="mt-3 text-2xl font-bold tracking-tight">{s.title}</h3>
                  <p className="mt-3 leading-relaxed text-muted">{s.body}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        {/* Creative direction */}
        <section id="direction" className="border-t border-line bg-ink-sunken">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="02">Creative direction</SectionLabel>
            <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
              <h2 className="text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
                Direction you can read, change and approve.
              </h2>
              <p className="max-w-2xl self-end text-lg leading-relaxed text-muted">
                CAVI&rsquo;s Creative Director won&rsquo;t render anything on its
                own. It writes a structured plan: which moments, in what order,
                at what length, with which captions. You review it, edit it, and
                only then does it reach the timeline.
              </p>
            </div>
            <div className="mt-12">
              <DirectionSpec />
            </div>
          </div>
        </section>

        {/* Audiences */}
        <section id="who" className="border-t border-line">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-28">
            <SectionLabel index="03">Who it&rsquo;s for</SectionLabel>
            <h2 className="mt-6 max-w-4xl text-4xl leading-[0.95] font-extrabold tracking-tight font-stretch-[80%] sm:text-6xl">
              For people who make things for a living.
            </h2>
            <ul className="mt-14 border-t border-line">
              {audiences.map((a) => (
                <li
                  key={a.name}
                  className="grid grid-cols-1 gap-3 border-b border-line py-8 md:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] md:gap-16"
                >
                  <h3 className="text-2xl font-bold tracking-tight sm:text-3xl">{a.name}</h3>
                  <p className="max-w-2xl text-lg leading-relaxed text-muted">{a.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* Principles */}
        <section className="border-t border-line bg-ink-raised">
          <div className="mx-auto max-w-7xl px-4 py-20 sm:px-8 md:py-24">
            <SectionLabel index="04">Principles</SectionLabel>
            <ul className="mt-10 grid grid-cols-1 gap-10 md:grid-cols-3 md:gap-12">
              {principles.map((p) => (
                <li key={p.title}>
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
            <SectionLabel index="05">Early access</SectionLabel>
            <h2 className="mt-6 text-[clamp(2.75rem,10vw,8rem)] leading-[0.88] font-extrabold tracking-[-0.03em] uppercase font-stretch-[70%]">
              Be first in the edit.
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
