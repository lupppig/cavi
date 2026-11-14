"use client";

import { useId, useState, type FormEvent } from "react";
import type { Discipline } from "@cavi/api-client";
import { api } from "@/lib/api";
import { Listbox } from "./listbox";

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

export function WaitlistForm() {
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
        <div className="ticket flipped relative flex max-w-xl items-stretch rounded-2xl border border-transparent bg-[var(--accent-strong)] text-white shadow-lg">
          <div className="flex-1 p-5 sm:p-6">
            <p className="mono text-xs uppercase opacity-85">Access ticket · early access</p>
            <p className="display mt-2 text-3xl sm:text-4xl">You&rsquo;re on the list.</p>
            <p className="mt-2 font-medium">We&rsquo;ll email you when early access opens.</p>
          </div>
          <div className="mono flex w-14 items-center justify-center border-l border-dashed border-[var(--line-strong)] text-xs font-bold tracking-[0.3em] uppercase [writing-mode:vertical-rl]">
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
    "min-h-13 w-full min-w-0 rounded-xl border border-[var(--line-strong)] bg-white px-4 text-base font-medium text-[var(--fg)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none focus:ring-3 focus:ring-[var(--accent)]/20 disabled:opacity-50";

  return (
    <form onSubmit={onSubmit}>
      <div
        className="grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_auto]"
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
          className={`${field} aria-invalid:border-[3px] aria-invalid:border-dashed sm:col-span-2`}
        />

        <Listbox
          name="discipline"
          label="What do you make? (optional)"
          placeholder="What do you make? (optional)"
          noneLabel="Prefer not to say"
          options={disciplines}
          disabled={unavailable || submitting}
        />

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
          <span className="inline-block rounded-md bg-[var(--danger-soft)] px-2 py-0.5 text-[var(--danger)]">{state.message}</span>
        )}
        {unavailable && (
          <span className="inline-block rounded-md bg-[var(--danger-soft)] px-2 py-0.5 text-[var(--danger)]">
            Signups are unavailable right now. Please check back soon.
          </span>
        )}
      </p>
      <p className="text-sm text-[var(--muted)]">
        We&rsquo;ll only use your email to contact you about CAVI early access.
      </p>
    </form>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
