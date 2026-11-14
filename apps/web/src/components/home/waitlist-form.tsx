"use client";

import { useId, useState, type CSSProperties, type FormEvent } from "react";
import type { Discipline } from "@cavi/api-client";
import { api } from "@/lib/api";

const disciplines: { value: Discipline; label: string }[] = [
  { value: "clipper", label: "Clipping & short-form" },
  { value: "creator", label: "Creator, streamer or podcaster" },
  { value: "musician", label: "Musician or artist" },
  { value: "studio", label: "Freelance editor or studio" },
  { value: "agency", label: "Agency" },
  { value: "brand", label: "Brand or in-house team" },
  { value: "other", label: "Something else" },
];

type State =
  | { kind: "idle" }
  | { kind: "submitting" }
  | { kind: "joined" }
  | { kind: "error"; message: string; field?: "email" | "discipline" };

const confettiColors = ["var(--tomato)", "var(--sun)", "var(--cobalt)", "var(--mint)", "var(--pink)", "var(--sky)"];
// Deterministic burst: 28 pieces spread around a circle.
const confetti = Array.from({ length: 28 }, (_, i) => {
  const angle = (i / 28) * Math.PI * 2 + (i % 3) * 0.2;
  const dist = 90 + (i % 5) * 28;
  return {
    x: `${Math.round(Math.cos(angle) * dist)}px`,
    y: `${Math.round(Math.sin(angle) * dist - 40)}px`,
    spin: `${(i % 2 ? 1 : -1) * (180 + i * 23)}deg`,
    delay: `${(i % 4) * 30}ms`,
    color: confettiColors[i % confettiColors.length],
    round: i % 3 === 0,
  };
});

export function WaitlistForm({ layout = "stacked" }: { layout?: "stacked" | "row" }) {
  const id = useId();
  const [state, setState] = useState<State>({ kind: "idle" });

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!api || state.kind === "submitting") return;

    const data = new FormData(event.currentTarget);
    const discipline = String(data.get("discipline") ?? "");
    setState({ kind: "submitting" });

    try {
      const { error, response } = await api.POST("/api/v1/waitlist", {
        body: {
          email: String(data.get("email") ?? ""),
          ...(discipline ? { discipline: discipline as Discipline } : {}),
          company_website: String(data.get("company_website") ?? ""),
        },
      });
      if (!error) {
        setState({ kind: "joined" });
        return;
      }
      if (response.status === 429) {
        setState({
          kind: "error",
          message: "Too many attempts from your network. Please try again later.",
        });
        return;
      }
      const fieldError = error.error.fields?.[0];
      setState({
        kind: "error",
        message: fieldError?.message ? capitalise(fieldError.message) + "." : error.error.message,
        field:
          fieldError?.field === "email" || fieldError?.field === "discipline"
            ? fieldError.field
            : undefined,
      });
    } catch {
      setState({
        kind: "error",
        message: "We couldn't reach CAVI. Check your connection and try again.",
      });
    }
  }

  if (state.kind === "joined") {
    return (
      <div role="status" className="relative">
        <div aria-hidden="true" className="pointer-events-none absolute top-1/2 left-1/2 z-10">
          {confetti.map((c, i) => (
            <span
              key={i}
              className={`confetti-piece absolute size-3 border-2 border-ink ${c.round ? "rounded-full" : "rounded-[2px]"}`}
              style={
                {
                  background: c.color,
                  "--x": c.x,
                  "--y": c.y,
                  "--spin": c.spin,
                  "--delay": c.delay,
                } as CSSProperties
              }
            />
          ))}
        </div>
        <div className="sticker -rotate-1 rounded-3xl bg-mint p-5 sm:p-6">
          <p className="text-2xl font-extrabold">You&rsquo;re on the list! 🎬</p>
          <p className="mt-1 font-medium">We&rsquo;ll email you when early access opens.</p>
        </div>
      </div>
    );
  }

  const unavailable = !api;
  const submitting = state.kind === "submitting";
  const errorId = `${id}-error`;
  const emailInvalid = state.kind === "error" && state.field === "email";

  const field =
    "min-h-13 w-full min-w-0 rounded-2xl border-[3px] border-ink bg-white px-4 text-base font-medium text-ink placeholder:text-ink-soft/70 focus:outline-none focus-visible:outline-none focus:ring-4 focus:ring-cobalt/40 disabled:opacity-60";

  return (
    <form onSubmit={onSubmit}>
      <div
        className={`grid grid-cols-1 gap-3 ${
          layout === "row"
            ? "md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto]"
            : "sm:grid-cols-[minmax(0,1fr)_auto]"
        }`}
      >
        <label htmlFor={`${id}-email`} className="sr-only">
          Email address
        </label>
        <input
          id={`${id}-email`}
          name="email"
          type="email"
          required
          maxLength={254}
          autoComplete="email"
          inputMode="email"
          placeholder="you@studio.com"
          disabled={unavailable || submitting}
          aria-invalid={emailInvalid || undefined}
          aria-describedby={emailInvalid ? errorId : undefined}
          className={`${field} aria-invalid:border-tomato ${layout === "row" ? "" : "sm:col-span-2"}`}
        />

        <label htmlFor={`${id}-discipline`} className="sr-only">
          What do you make? (optional)
        </label>
        <select
          id={`${id}-discipline`}
          name="discipline"
          defaultValue=""
          disabled={unavailable || submitting}
          className={`${field} appearance-none bg-[length:12px] bg-[right_1rem_center] bg-no-repeat pr-10`}
          style={{
            backgroundImage:
              "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 12 8'%3E%3Cpath d='M1 1.5l5 5 5-5' stroke='%23141414' stroke-width='2.5' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E\")",
          }}
        >
          <option value="">What do you make?</option>
          {disciplines.map((d) => (
            <option key={d.value} value={d.value}>
              {d.label}
            </option>
          ))}
        </select>

        {/* Honeypot: hidden from people and assistive tech; bots tend to fill it. */}
        <div aria-hidden="true" className="absolute -left-[9999px] h-px w-px overflow-hidden">
          <label htmlFor={`${id}-company-website`}>Company website</label>
          <input id={`${id}-company-website`} name="company_website" type="text" tabIndex={-1} autoComplete="off" />
        </div>

        <button
          type="submit"
          disabled={unavailable || submitting}
          className="btn-pop min-h-13 rounded-2xl bg-cobalt px-6 text-base font-extrabold whitespace-nowrap text-white disabled:cursor-not-allowed disabled:opacity-60"
        >
          {submitting ? "Joining…" : "Join the waitlist →"}
        </button>
      </div>

      <p id={errorId} aria-live="polite" className="mt-3 min-h-5 text-sm font-bold">
        {state.kind === "error" && (
          <span className="inline-block rounded-lg bg-tomato px-2 py-0.5 text-white">{state.message}</span>
        )}
        {unavailable && (
          <span className="inline-block rounded-lg bg-tomato px-2 py-0.5 text-white">
            Signups are unavailable right now. Please check back soon.
          </span>
        )}
      </p>
      <p className="text-sm font-medium text-ink-soft">
        We&rsquo;ll only use your email to contact you about CAVI early access.
      </p>
    </form>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
