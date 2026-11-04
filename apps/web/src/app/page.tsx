import type { CSSProperties, ReactNode } from "react";
import {
  BrandArt,
  ClipperArt,
  MusicianArt,
  PodcasterArt,
  StudioArt,
} from "@/components/landing/audience-art";
import { Character, Heart, INK, Sparkle } from "@/components/landing/cartoon";
import { DirectionSpec } from "@/components/landing/direction-spec";
import { EditorSketch } from "@/components/landing/editor-sketch";
import { FormatStrip } from "@/components/landing/format-strip";
import { HeroStickers } from "@/components/landing/hero-stickers";
import { PlayWhenVisible } from "@/components/landing/play-when-visible";
import { ReframeSketch } from "@/components/landing/reframe-sketch";
import { SiteHeader } from "@/components/landing/site-header";
import { signInHref } from "@/lib/api";
import { WaitlistForm } from "@/components/landing/waitlist-form";
import {
  DirectionArt,
  DistributeArt,
  EditArt,
  RenderArt,
  ReviewArt,
  SelectionArt,
} from "@/components/landing/workflow-art";

const stages = [
  {
    title: "Direction",
    Art: DirectionArt,
    bg: "bg-sun",
    body: "Say what you want in plain words. CAVI turns it into a creative brief you can read, question and change.",
  },
  {
    title: "Selection",
    Art: SelectionArt,
    bg: "bg-pink",
    body: "Hours of footage searched for the moments worth cutting, with the setup kept and every timestamp shown.",
  },
  {
    title: "Edit",
    Art: EditArt,
    bg: "bg-mint",
    body: "A real multi-track timeline. Snip, trim to the frame, reframe for any aspect ratio, caption, layer and undo.",
  },
  {
    title: "Review",
    Art: ReviewArt,
    bg: "bg-sky",
    body: "Share cuts with collaborators and clients, collect notes and approvals, and keep every version.",
  },
  {
    title: "Render",
    Art: RenderArt,
    bg: "bg-lime",
    body: "What you see in preview is what renders, and every output is checked before it reaches you.",
  },
  {
    title: "Distribute",
    Art: DistributeArt,
    bg: "bg-tomato",
    body: "Platform-ready exports, scheduling and publishing, marked as published only once the platform confirms it.",
  },
];

const audiences = [
  {
    name: "Clippers & short-form editors",
    body: "Find the moment, keep the context, and cut it for every vertical feed, in batches, with client delivery built in.",
    Art: ClipperArt,
    bg: "bg-sky",
    span: "lg:col-span-3",
  },
  {
    name: "Creators, streamers & podcasters",
    body: "Turn one recording into a week of posts that still sound like you and look like your brand.",
    Art: PodcasterArt,
    bg: "bg-sun",
    span: "lg:col-span-3",
  },
  {
    name: "Musicians & artists",
    body: "Teasers, visualisers and release campaigns, with every version kept and every cut on-identity.",
    Art: MusicianArt,
    bg: "bg-pink",
    span: "lg:col-span-2",
  },
  {
    name: "Freelance editors & studios",
    body: "Client workspaces, brand kits, approvals and revisions in one place instead of a dozen email threads.",
    Art: StudioArt,
    bg: "bg-mint",
    span: "lg:col-span-2",
  },
  {
    name: "Agencies & brands",
    body: "Ad variations and campaign assets made on-brand, reviewed by the right people, and tracked from brief to post.",
    Art: BrandArt,
    bg: "bg-lime",
    span: "lg:col-span-2",
  },
];

const principles = [
  {
    title: "Your cut, not the model’s.",
    body: "AI suggests. You decide. Nothing renders or publishes without your say.",
    bg: "bg-sun",
    rot: "-rotate-2",
  },
  {
    title: "Real editing.",
    body: "Every control changes the actual project, and the preview matches the export.",
    bg: "bg-pink",
    rot: "rotate-1",
  },
  {
    title: "Permission first.",
    body: "Live clipping only from streams you’re allowed to use. Your media stays private by default.",
    bg: "bg-sky",
    rot: "-rotate-1",
  },
];

const headline: { w: string; hl?: boolean }[] = [
  { w: "Make" },
  { w: "more" },
  { w: "of" },
  { w: "what" },
  { w: "moves", hl: true },
  { w: "people." },
];
const closing = ["Be", "first", "in", "the", "edit."];

function Tag({ children, bg = "bg-white" }: { children: ReactNode; bg?: string }) {
  return (
    <p className={`sticker inline-flex items-center gap-2 rounded-full ${bg} px-3.5 py-1.5 text-sm font-extrabold`}>
      {children}
    </p>
  );
}

const v = (vars: Record<string, string | number>) => vars as CSSProperties;

export default function Home() {
  return (
    <>
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-full focus:bg-ink focus:px-4 focus:py-3 focus:text-white"
      >
        Skip to content
      </a>
      <SiteHeader />

      <main id="main" className="overflow-x-clip">
        {/* ---------------------------------------------------------- Hero */}
        <section id="top" className="relative mx-auto max-w-7xl px-4 pt-14 pb-10 sm:px-8 sm:pt-20">
          <HeroStickers />
          <Tag>
            <span aria-hidden className="rec-dot size-2.5 rounded-full border-2 border-ink bg-tomato" />
            Creative production · in the making
          </Tag>

          <h1 className="relative mt-7 max-w-5xl text-[clamp(3.1rem,11.5vw,8.75rem)] leading-[0.9] font-extrabold tracking-[-0.045em]">
            <span className="sr-only">Make more of what moves people.</span>
            <span aria-hidden="true">
              {headline.map(({ w, hl }, i) => (
                <span key={w}>
                  {hl ? (
                    <span
                      className="caption-sticker sticker my-1 rounded-[0.25em] bg-tomato px-[0.18em] pb-[0.06em] text-white"
                      style={v({ "--i": i })}
                    >
                      {w}
                    </span>
                  ) : (
                    <span className="word-pop inline-block" style={v({ "--i": i })}>
                      {w === "people." ? (
                        <span className="relative inline-block">
                          people.
                          <svg viewBox="0 0 300 30" preserveAspectRatio="none" className="absolute -bottom-[0.12em] left-0 h-[0.2em] w-[92%]" aria-hidden="true">
                            <path d="M4 18 Q40 4 76 16 T150 16 T224 14 T296 12" fill="none" stroke="var(--cobalt)" strokeWidth="10" strokeLinecap="round" />
                          </svg>
                        </span>
                      ) : (
                        w
                      )}
                    </span>
                  )}{" "}
                </span>
              ))}
            </span>
          </h1>

          <div className="mt-10 grid grid-cols-1 gap-12 lg:mt-14 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-14">
            <div className="max-w-xl">
              <p className="text-xl leading-relaxed font-medium sm:text-2xl">
                We&rsquo;re building CAVI: one creative studio that takes you from raw footage and a
                wild idea to finished, on-brand edits for every platform, and every creative call
                stays <span className="rounded-md bg-sun px-1 font-extrabold">yours</span>.
              </p>
              <div className="mt-8">
                <WaitlistForm />
              </div>
            </div>
            <EditorSketch />
          </div>
        </section>

        <FormatStrip />

        {/* ------------------------------------------------------ Workflow */}
        <section id="workflow" className="mx-auto max-w-7xl px-4 py-16 sm:px-8 md:py-24">
          <Tag bg="bg-sun">01 · How it works</Tag>
          <h2 className="reveal mt-6 max-w-4xl text-5xl leading-[0.95] font-extrabold tracking-[-0.04em] sm:text-7xl">
            From the first frame to the{" "}
            <span className="inline-block -rotate-1 rounded-2xl bg-cobalt px-3 text-white">final upload.</span>
          </h2>
          <p className="mt-6 max-w-2xl text-lg font-medium sm:text-xl">
            Six stages in one project, so nothing gets lost between the idea and the post. This is
            what we&rsquo;re building.
          </p>

          <ol className="mt-14 grid grid-cols-1 gap-7 sm:grid-cols-2 lg:grid-cols-3">
            {stages.map((s, i) => (
              <li
                key={s.title}
                className={`reveal tilt-hover sticker-lg flex flex-col overflow-hidden rounded-[28px] ${s.bg}`}
                style={v({ "--i": i % 3 })}
              >
                <PlayWhenVisible className="aspect-[4/3] p-3 sm:p-4">
                  <s.Art />
                </PlayWhenVisible>
                <div className="m-2 mt-0 flex-1 rounded-[20px] border-[3px] border-ink bg-white p-5">
                  <div className="flex items-center gap-3">
                    <span className="grid size-9 place-items-center rounded-full border-[3px] border-ink bg-ink text-sm font-extrabold text-white">
                      {i + 1}
                    </span>
                    <h3 className="text-2xl font-extrabold tracking-tight">{s.title}</h3>
                  </div>
                  <p className="mt-3 leading-relaxed font-medium">{s.body}</p>
                </div>
              </li>
            ))}
          </ol>
        </section>

        {/* ----------------------------------------------------- Reframing */}
        <section id="reframe" className="px-3 sm:px-6">
          <div className="sticker-lg mx-auto max-w-7xl overflow-hidden rounded-[40px] bg-cobalt px-5 py-14 text-white sm:px-10 md:py-20">
            <div className="grid grid-cols-1 items-center gap-10 lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)] lg:gap-14">
              <div>
                <Tag bg="bg-sun">
                  <span className="text-ink">02 · Reframing</span>
                </Tag>
                <h2 className="reveal mt-6 text-5xl leading-[0.95] font-extrabold tracking-[-0.04em] sm:text-7xl">
                  One cut.
                  <br />
                  <span className="text-sun">Every frame.</span>
                </h2>
                <p className="mt-6 max-w-xl text-lg font-medium sm:text-xl">
                  We&rsquo;re building reframing that follows whoever is talking, so one edit can go
                  out at 16:9, 1:1, 4:5 and 9:16 without losing the person, the reaction or the
                  moment.
                </p>
              </div>
              <ReframeSketch />
            </div>
          </div>
        </section>

        {/* ----------------------------------------------- Creative direction */}
        <section id="direction" className="mx-auto max-w-7xl px-4 py-16 sm:px-8 md:py-24">
          <Tag bg="bg-pink">03 · Creative direction</Tag>
          <div className="mt-6 grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,6fr)_minmax(0,5fr)] lg:items-end lg:gap-16">
            <h2 className="reveal text-5xl leading-[0.95] font-extrabold tracking-[-0.04em] sm:text-7xl">
              Direction you can read, change and{" "}
              <span className="inline-block rotate-1 rounded-2xl bg-mint px-3">approve.</span>
            </h2>
            <p className="max-w-xl text-lg font-medium sm:text-xl">
              CAVI&rsquo;s Creative Director won&rsquo;t render anything on its own. It writes a plan:
              which moments, in what order, how long, with which captions. You tweak it, and only
              then does it hit the timeline.
            </p>
          </div>
          <div className="mt-14">
            <DirectionSpec />
          </div>
        </section>

        {/* -------------------------------------------------------- Audiences */}
        <section id="who" className="mx-auto max-w-7xl px-4 pb-16 sm:px-8 md:pb-24">
          <Tag bg="bg-sky">04 · Who it&rsquo;s for</Tag>
          <h2 className="reveal mt-6 max-w-4xl text-5xl leading-[0.95] font-extrabold tracking-[-0.04em] sm:text-7xl">
            For people who make things{" "}
            <span className="inline-block -rotate-2 rounded-2xl bg-sun px-3">for a living.</span>
          </h2>
          <ul className="mt-14 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-6">
            {audiences.map((a, i) => (
              <li
                key={a.name}
                className={`reveal tilt-hover sticker-lg flex flex-col overflow-hidden rounded-[28px] ${a.bg} ${a.span}`}
                style={v({ "--i": i % 3 })}
              >
                <div className="h-44 px-6 pt-6 sm:h-48">
                  <a.Art />
                </div>
                <div className="m-2 mt-0 flex-1 rounded-[20px] border-[3px] border-ink bg-white p-5">
                  <h3 className="text-2xl font-extrabold tracking-tight">{a.name}</h3>
                  <p className="mt-2 leading-relaxed font-medium">{a.body}</p>
                </div>
              </li>
            ))}
          </ul>
        </section>

        {/* ------------------------------------------------------ Principles */}
        <section className="px-3 sm:px-6">
          <div className="sticker-lg mx-auto max-w-7xl rounded-[40px] bg-ink px-5 py-14 text-white sm:px-10 md:py-20">
            <Tag bg="bg-mint">
              <span className="text-ink">05 · What we promise</span>
            </Tag>
            <ul className="mt-10 grid grid-cols-1 gap-8 md:grid-cols-3">
              {principles.map((p, i) => (
                <li
                  key={p.title}
                  className={`reveal sticker rounded-3xl ${p.bg} ${p.rot} p-6 text-ink`}
                  style={v({ "--i": i })}
                >
                  <svg viewBox="0 0 40 40" className="art wobble size-10" aria-hidden="true" style={v({ "--d": `${-i * 0.7}s` })}>
                    {i === 0 && <Heart x={20} y={22} size={1.5} fill="var(--tomato)" />}
                    {i === 1 && <Sparkle x={20} y={20} size={15} fill="white" />}
                    {i === 2 && (
                      <g>
                        <rect x="9" y="17" width="22" height="17" rx="4" fill="var(--sun)" stroke={INK} strokeWidth="2.5" />
                        <path d="M14 17 V12 Q14 6 20 6 Q26 6 26 12 V17" fill="none" stroke={INK} strokeWidth="2.5" />
                      </g>
                    )}
                  </svg>
                  <h3 className="mt-4 text-3xl leading-tight font-extrabold tracking-tight">{p.title}</h3>
                  <p className="mt-3 text-lg leading-relaxed font-medium">{p.body}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>

        {/* ---------------------------------------------------- Closing CTA */}
        <section id="waitlist" className="px-3 py-16 sm:px-6 md:py-24">
          <div className="sticker-lg relative mx-auto max-w-7xl overflow-hidden rounded-[40px] bg-tomato px-5 py-14 sm:px-10 md:py-20">
            <svg viewBox="-60 -50 120 120" className="art bob pointer-events-none absolute -right-4 -bottom-6 hidden w-44 sm:block md:w-56" aria-hidden="true" style={v({ "--rot": "-6deg" })}>
              <Character x={0} y={0} scale={1.4} skin="var(--skin-2)" hair="cap" hairColor="var(--cobalt)" shirt="var(--sun)" mouth="grin" blinkDelay="0.4s" />
            </svg>
            <svg viewBox="0 0 40 40" className="art spin-slow pointer-events-none absolute top-8 right-8 size-14" aria-hidden="true">
              <Sparkle x={20} y={20} size={17} fill="var(--sun)" />
            </svg>

            <Tag>06 · Early access</Tag>
            <h2 className="mt-6 text-[clamp(3rem,11vw,8.5rem)] leading-[0.88] font-extrabold tracking-[-0.045em] text-white">
              <span className="sr-only">Be first in the edit.</span>
              <span aria-hidden="true">
                {closing.map((w, i) => (
                  <span key={w}>
                    <span className="reveal-word [paint-order:stroke_fill] [-webkit-text-stroke:6px_var(--ink)]" style={v({ "--i": i })}>
                      {w}
                    </span>{" "}
                  </span>
                ))}
              </span>
            </h2>
            <p className="mt-6 max-w-2xl text-lg font-bold text-white sm:text-xl">
              CAVI isn&rsquo;t open yet. Join the waitlist and we&rsquo;ll let you know the moment
              early access begins.
            </p>
            <div className="sticker relative mt-10 max-w-4xl rounded-[28px] bg-white p-4 sm:p-6">
              <WaitlistForm layout="row" />
            </div>
          </div>
        </section>
      </main>

      <footer className="px-4 pb-10 sm:px-8">
        <div className="mx-auto flex max-w-7xl flex-col gap-4 border-t-[3px] border-ink pt-8 sm:flex-row sm:items-end sm:justify-between">
          <p className="text-[clamp(4rem,16vw,10rem)] leading-none font-extrabold tracking-[-0.06em] text-paper [paint-order:stroke_fill] [-webkit-text-stroke:3px_var(--ink)]">
            CAVI
          </p>
          <div className="flex flex-col gap-1 pb-3 text-sm font-bold sm:items-end">
            <span>Made for people who make things.</span>
            <a href={signInHref()} className="flex min-h-11 items-center underline decoration-2 underline-offset-4 hover:text-tomato sm:justify-end">
              Already have an account? Sign in
            </a>
            <span className="text-ink-soft">© 2026 CAVI · In development</span>
          </div>
        </div>
      </footer>
    </>
  );
}
