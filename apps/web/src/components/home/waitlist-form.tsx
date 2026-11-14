"use client";

import { useId, useState, type FormEvent } from "react";
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
      <div role="status" className="[perspective:900px]">
        <div className="ticket flipped relative flex max-w-xl items-stretch border-2 border-current bg-[var(--fg)] text-[var(--bg)]">
          <div className="flex-1 p-5 sm:p-6">
            <p className="mono text-xs uppercase opacity-85">Access ticket · early access</p>
            <p className="display mt-2 text-3xl sm:text-4xl">You&rsquo;re on the list.</p>
            <p className="mt-2 font-medium">We&rsquo;ll email you when early access opens.</p>
          </div>
          <div className="mono flex w-14 items-center justify-center border-l-2 border-dashed border-current text-xs font-bold tracking-[0.3em] uppercase [writing-mode:vertical-rl]">
            Admit one
          </div>
        </div>
      </div>
    );
  }

  const unavailable = !api;
  const submitting = state.kind === "submitting";
  const errorId = `${id}-error`;
  const emailInvalid = state.kind === "error" && state.field === "email";

  const field =
    "min-h-13 w-full min-w-0 border-2 border-current bg-transparent px-4 text-base font-medium text-current placeholder:text-current placeholder:opacity-75 focus:outline-none focus-visible:outline-none focus:ring-2 focus:ring-current focus:ring-offset-2 focus:ring-offset-[var(--bg)] disabled:opacity-50";

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
          className={`${field} aria-invalid:border-[3px] aria-invalid:border-dashed ${layout === "row" ? "" : "sm:col-span-2"}`}
        />

        <label htmlFor={`${id}-discipline`} className="sr-only">
          What do you make? (optional)
        </label>
        <select
          id={`${id}-discipline`}
          name="discipline"
          defaultValue=""
          disabled={unavailable || submitting}
          className={`${field} bg-[var(--bg)]`}
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
          className="btn min-h-13 whitespace-nowrap"
        >
          {submitting ? "Joining…" : "Join the waitlist →"}
        </button>
      </div>

      <p id={errorId} aria-live="polite" className="mt-3 min-h-5 text-sm font-bold">
        {state.kind === "error" && (
          <span className="inline-block bg-[var(--fg)] px-2 py-0.5 text-[var(--bg)]">{state.message}</span>
        )}
        {unavailable && (
          <span className="inline-block bg-[var(--fg)] px-2 py-0.5 text-[var(--bg)]">
            Signups are unavailable right now. Please check back soon.
          </span>
        )}
      </p>
      <p className="text-sm font-medium opacity-85">
        We&rsquo;ll only use your email to contact you about CAVI early access.
      </p>
    </form>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
