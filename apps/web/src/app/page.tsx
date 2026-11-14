import { Fragment, type CSSProperties } from "react";
import type { Viewport } from "next";
import { BriefToSpec } from "@/components/home/brief-to-spec";
import { CutGame } from "@/components/home/cut-game";
import { ReframeStage } from "@/components/home/reframe-stage";
import { Reel } from "@/components/home/reel";
import { Mark, SiteHeader } from "@/components/home/site-header";
import { StagePrism, type Stage } from "@/components/home/stage-prism";
import { Timecode } from "@/components/home/timecode";
import { WaitlistForm } from "@/components/home/waitlist-form";
import { signInHref } from "@/lib/api";
import "./home.css";


export const viewport: Viewport = {
  themeColor: "#f7f7f8",
  colorScheme: "light",
};

const stages: Stage[] = [
  { title: "Direction", body: "Say what you want in plain words. CAVI turns it into a brief you can read, question and change." },
  { title: "Selection", body: "Hours of footage searched for the moments worth cutting, with the setup kept and every timestamp shown." },
  { title: "Edit", body: "Trim to the frame, reframe for any shape, caption, layer and undo. Every control changes the real project." },
  { title: "Review", body: "Share cuts with collaborators and clients, collect notes and approvals, keep every version." },
  { title: "Render", body: "What you preview is what renders, and every output is checked before it reaches you." },
  { title: "Distribute", body: "Platform-ready exports and publishing, marked as published only once the platform confirms it." },
];

const formats = [
  "Podcasts", "Gaming", "Interviews", "Tutorials", "Music", "Commentary", "Documentary",
  "Product demos", "Fashion & beauty", "Food", "Events", "Ads", "Testimonials", "Real estate",
];

const audiences = [
  { name: "Clippers", body: "Find the moment, keep the context, cut it for every vertical feed, in batches." },
  { name: "Creators & podcasters", body: "One recording, a week of posts that still sound like you." },
  { name: "Musicians & artists", body: "Teasers, visualisers and release campaigns, every version kept." },
  { name: "Editors & studios", body: "Client workspaces, approvals and revisions instead of a dozen email threads." },
  { name: "Agencies & brands", body: "Ad variations made on-brand, reviewed by the right people, tracked from brief to post." },
];

const principles = [
  { title: "Your cut, not the model’s.", body: "AI suggests. You decide. Nothing renders or publishes without your say." },
  { title: "Real editing.", body: "Every control changes the actual project, and the preview matches the export." },
  { title: "Permission first.", body: "Clip only what you’re allowed to use. Your media stays private by default." },
];

/** Splits text into letters that flip up in sequence. Words never break. */
function Flip({ text, from = 0, className = "" }: { text: string; from?: number; className?: string }) {
  let n = from;
  return (
    <span className={`flip-scene ${className}`} aria-hidden="true">
      {text.split(" ").map((word, w) => (
        // The space sits between words: inside an inline-block it would collapse.
        <Fragment key={w}>
          <span className="inline-block whitespace-nowrap">
            {[...word].map((ch, i) => (
              <span key={i} className="flip-letter" style={{ "--i": n++ } as CSSProperties}>
                {ch}
              </span>
            ))}
          </span>{" "}
        </Fragment>
      ))}
    </span>
  );
}

function Kicker({ n, children }: { n: string; children: string }) {
  return (
    <p className="mono flex items-center gap-3 text-xs uppercase">
      <span className="rounded-md bg-[var(--accent-soft)] px-1.5 py-0.5 font-semibold text-[var(--accent-text)]">{n}</span>
      <span className="text-[var(--muted)]">{children}</span>
    </p>
  );
}

function Band({ items, invert, reverse }: { items: string[]; invert?: boolean; reverse?: boolean }) {
  const row = [...items, ...items];
  return (
    <div
      className={`skew-band overflow-hidden border-y py-3 sm:py-4 ${
        invert ? "border-transparent bg-[var(--accent-strong)] text-white" : "border-[var(--line)] bg-white text-[var(--fg)]"
      }`}
    >
      <ul className={`flex w-max gap-8 ${reverse ? "marquee-reverse" : "marquee"}`}>
        {row.map((f, i) => (
          <li key={i} className={`display flex items-center gap-8 text-3xl sm:text-5xl ${invert ? "" : "outline-text"}`} aria-hidden={i >= items.length}>
            {f}
            <span aria-hidden="true" className="inline-block size-3 rotate-45 bg-current sm:size-4" />
          </li>
        ))}
      </ul>
    </div>
  );
}

export default function Home() {
  return (
    <div className="home">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-lg focus:bg-white focus:px-4 focus:py-3 focus:text-black focus:shadow"
      >
        Skip to content
      </a>
      <SiteHeader />

      <main id="main" className="overflow-x-clip">
        {/* ---------------------------------------------------------- Hero */}
        <section id="top" className="ink grain relative overflow-hidden pt-24 pb-10 sm:pt-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <div className="mono flex flex-wrap items-center justify-between gap-2 text-xs uppercase">
              <p className="flex items-center gap-2">
                <span aria-hidden="true" className="rec size-2.5 rounded-full bg-[var(--accent)]" />
                Rec <Timecode />
              </p>
              <p className="text-[var(--muted)]">Creative production · in development</p>
            </div>

            <h1 className="display relative z-10 mt-8 text-[clamp(2.6rem,9.5vw,9.5rem)] sm:mt-12">
              <span className="sr-only">Make more of what moves people.</span>
              <Flip text="Make more of what" />
              <span aria-hidden="true" className="serif inline-block pr-[0.08em] text-[1.12em] leading-[0.8] lowercase">
                <Flip text="moves" from={14} />
              </span>{" "}
              <Flip text="people." from={19} />
            </h1>

            <div className="mt-6 flex flex-col gap-6 xl:mt-12">
              <div className="relative z-10 max-w-xl">
                <p className="text-lg leading-relaxed sm:text-xl">
                  We&rsquo;re building CAVI: one creative studio that takes raw footage and a wild idea to finished edits
                  for every platform, and every creative call stays{" "}
                  <span className="serif text-[1.3em]">yours</span>.
                </p>
                <div className="mt-8">
                  <WaitlistForm />
                </div>
              </div>
              {/* Under the headline on narrower screens; on wide ones it spins
                  behind the headline, which inverts over it. */}
              <div className="order-first -mt-6 mb-0 sm:-mt-24 sm:mb-6 xl:absolute xl:top-32 xl:-right-16 xl:order-none xl:my-0 xl:w-[58%]">
                <Reel />
              </div>
            </div>

            <div className="mono mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--line-strong)] pt-4 text-xs uppercase">
              <span className="text-[var(--muted)]">Scroll to roll ↓</span>
              <a href="#game" className="flex min-h-11 items-center gap-2 font-bold underline-offset-4 hover:underline">
                ▶ Play “Cut the moment”
              </a>
            </div>
          </div>
        </section>

        {/* -------------------------------------------------------- Formats */}
        <section aria-label="Made for every format" className="ink relative z-10 -my-6 space-y-[-0.75rem] py-6 sm:-my-10 sm:py-10">
          <div className="-rotate-2">
            <Band items={formats} invert />
          </div>
          <div className="rotate-2">
            <Band items={[...formats].reverse()} reverse />
          </div>
        </section>

        {/* ----------------------------------------------------------- Game */}
        <section id="game" className="paper py-20 sm:py-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <Kicker n="01">Play</Kicker>
            <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:items-end">
              <h2 className="display reveal text-[clamp(2.75rem,8vw,7.5rem)]">
                Find the <span className="serif lowercase">cut</span>.
              </h2>
              <p className="max-w-xl text-lg leading-relaxed">
                A great clip keeps the setup and lands the payoff, with no dead air. Three rounds, each faster than the last.
                Your best score stays on this device.
              </p>
            </div>
            <div className="mt-10">
              <CutGame />
            </div>
          </div>
        </section>

        {/* ------------------------------------------------------- Workflow */}
        <section id="workflow" className="ink grain">
          <div className="mx-auto max-w-[90rem] px-4 pt-20 sm:px-8 sm:pt-28">
            <Kicker n="02">How it works</Kicker>
            <h2 className="display reveal mt-6 max-w-6xl text-[clamp(2.5rem,7vw,6.5rem)]">
              From the first frame to the <span className="serif lowercase">final</span> upload.
            </h2>
            <p className="mt-6 max-w-2xl text-lg">Six stages, one project. Scroll to turn the reel. This is what we&rsquo;re building.</p>
          </div>
          <StagePrism stages={stages} />
        </section>

        {/* -------------------------------------------------------- Reframe */}
        <section id="reframe" className="paper py-20 sm:py-28">
          <div className="mx-auto grid max-w-[90rem] grid-cols-1 items-center gap-10 px-4 sm:px-8 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-16">
            <div>
              <Kicker n="03">Reframing</Kicker>
              <h2 className="display reveal mt-6 text-[clamp(2.75rem,7vw,6.5rem)]">
                One cut. <span className="serif lowercase">every</span> frame.
              </h2>
              <p className="mt-6 max-w-xl text-lg leading-relaxed">
                Cut once, then pick the shape: 9:16, 1:1, 4:5 or 16:9, framed where you want it. Next we&rsquo;re building
                framing that follows whoever is talking, so the reaction never falls out of shot.
              </p>
            </div>
            <ReframeStage />
          </div>
        </section>

        {/* ------------------------------------------------------ Direction */}
        <section id="direction" className="ink grain py-20 sm:py-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <Kicker n="04">Creative direction</Kicker>
            <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)] lg:items-end lg:gap-16">
              <h2 className="display reveal text-[clamp(2.5rem,6.5vw,6rem)]">
                Direction you can <span className="serif lowercase">read</span>, change and approve.
              </h2>
              <p className="max-w-xl text-lg leading-relaxed">
                CAVI&rsquo;s Creative Director won&rsquo;t render anything on its own. It writes a plan: which moments, in
                what order, how long, with which captions. You change it, and only then does it reach the timeline.
              </p>
            </div>
            <div className="mt-12">
              <BriefToSpec />
            </div>
          </div>
        </section>

        {/* ----------------------------------------------------- Audiences */}
        <section id="who" className="paper py-20 sm:py-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <Kicker n="05">Who it’s for</Kicker>
            <h2 className="display reveal mt-6 max-w-5xl text-[clamp(2.5rem,7vw,6.5rem)]">
              For people who make things <span className="serif lowercase">for a living</span>.
            </h2>
            <ul className="mt-12 border-b border-[var(--line-strong)]">
              {audiences.map((a, i) => (
                <li
                  key={a.name}
                  className="row-reveal reveal grid grid-cols-[2.5rem_minmax(0,1fr)] gap-x-4 gap-y-2 border-t border-[var(--line-strong)] px-2 py-6 hover:bg-[var(--accent-soft)] sm:grid-cols-[4rem_minmax(0,1.1fr)_minmax(0,1fr)] sm:items-center sm:px-4 sm:py-8"
                >
                  <span className="mono text-sm">{String(i + 1).padStart(2, "0")}</span>
                  <h3 className="display text-3xl sm:text-5xl">{a.name}</h3>
                  <p className="col-start-2 text-base leading-relaxed sm:col-start-3 sm:text-lg">{a.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* ---------------------------------------------------- Principles */}
        <section className="ink grain py-20 sm:py-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <Kicker n="06">What we promise</Kicker>
            <ol className="mt-12 grid grid-cols-1 gap-px overflow-hidden rounded-2xl border border-[var(--line)] bg-[var(--line)] md:grid-cols-3">
              {principles.map((p, i) => (
                <li key={p.title} className="reveal-3d bg-[var(--bg)] p-6 sm:p-8">
                  <span className="display text-7xl text-[var(--accent)] sm:text-8xl">{i + 1}</span>
                  <h3 className="display mt-6 text-3xl sm:text-4xl">{p.title}</h3>
                  <p className="mt-3 text-lg leading-relaxed">{p.body}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        {/* ------------------------------------------------------ Waitlist */}
        <section id="waitlist" className="paper py-20 sm:py-28">
          <div className="mx-auto max-w-[90rem] px-4 sm:px-8">
            <Kicker n="07">Early access</Kicker>
            <h2 className="display reveal mt-6 text-[clamp(3rem,11vw,10.5rem)]">
              Be first in the <span className="serif lowercase">edit</span>.
            </h2>
            <p className="mt-6 max-w-2xl text-lg sm:text-xl">
              CAVI isn&rsquo;t open yet. Join the waitlist and we&rsquo;ll tell you the moment early access begins.
            </p>
            <div className="mt-10 max-w-xl">
              <WaitlistForm />
            </div>
          </div>
        </section>
      </main>

      <footer className="ink px-4 pt-16 pb-10 sm:px-8">
        <div className="mx-auto max-w-[90rem]">
          <p className="display outline-text text-[clamp(5rem,26vw,24rem)] leading-[0.8]" aria-hidden="true">
            CAVI
          </p>
          <div className="mono mt-8 flex flex-col gap-3 border-t border-[var(--line-strong)] pt-6 text-xs uppercase sm:flex-row sm:items-center sm:justify-between">
            <span className="flex items-center gap-2">
              <Mark className="size-5" /> Made for people who make things
            </span>
            <a href={signInHref()} className="flex min-h-11 items-center underline-offset-4 hover:underline">
              Already have an account? Sign in
            </a>
            <span className="text-[var(--muted)]">© 2026 CAVI · In development</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
