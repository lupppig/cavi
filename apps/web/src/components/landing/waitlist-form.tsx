"use client";

import { useId, useState, type FormEvent } from "react";
import { createApiClient, type Discipline } from "@cavi/api-client";

const apiBaseUrl = process.env.NEXT_PUBLIC_CAVI_API_URL;
const api = apiBaseUrl ? createApiClient(apiBaseUrl) : null;

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

export function WaitlistForm({ tone = "default" }: { tone?: "default" | "large" }) {
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
        message: fieldError?.message
          ? capitalise(fieldError.message) + "."
          : error.error.message,
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
      <div role="status" className="border-l-2 border-tally py-2 pl-4">
        <p className="text-lg font-semibold">You&rsquo;re on the list.</p>
        <p className="mt-1 text-muted">
          We&rsquo;ll email you when early access opens.
        </p>
      </div>
    );
  }

  const unavailable = !api;
  const submitting = state.kind === "submitting";
  const errorId = `${id}-error`;
  const emailInvalid = state.kind === "error" && state.field === "email";

  return (
    <form onSubmit={onSubmit}>
      <div
        className={`grid grid-cols-1 gap-2 ${
          tone === "large"
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
          className={`min-h-12 w-full min-w-0 border border-line-strong bg-ink-sunken px-4 text-base text-paper placeholder:text-faint focus:border-paper focus:outline-none disabled:opacity-50 aria-invalid:border-tally ${
            tone === "large" ? "" : "sm:col-span-2"
          }`}
        />

        <label htmlFor={`${id}-discipline`} className="sr-only">
          What do you make? (optional)
        </label>
        <select
          id={`${id}-discipline`}
          name="discipline"
          defaultValue=""
          disabled={unavailable || submitting}
          className="min-h-12 w-full min-w-0 appearance-none border border-line-strong bg-ink-sunken bg-[length:10px] bg-[right_1rem_center] bg-no-repeat px-4 pr-10 text-base text-paper focus:border-paper focus:outline-none disabled:opacity-50"
          style={{
            backgroundImage:
              "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%239a948b' stroke-width='1.5' fill='none'/%3E%3C/svg%3E\")",
          }}
        >
          <option value="">What do you make? (optional)</option>
          {disciplines.map((d) => (
            <option key={d.value} value={d.value}>
              {d.label}
            </option>
          ))}
        </select>

        {/* Honeypot: hidden from people and assistive tech; bots tend to fill it. */}
        <div aria-hidden="true" className="absolute -left-[9999px] h-px w-px overflow-hidden">
          <label htmlFor={`${id}-company-website`}>Company website</label>
          <input
            id={`${id}-company-website`}
            name="company_website"
            type="text"
            tabIndex={-1}
            autoComplete="off"
          />
        </div>

        <button
          type="submit"
          disabled={unavailable || submitting}
          className="min-h-12 bg-paper px-6 text-base font-semibold text-ink transition-colors hover:bg-tally disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-paper"
        >
          {submitting ? "Joining…" : "Join the waitlist"}
        </button>
      </div>

      <p id={errorId} aria-live="polite" className="mt-3 min-h-5 text-sm">
        {state.kind === "error" && <span className="text-tally">{state.message}</span>}
        {unavailable && (
          <span className="text-tally">
            Signups are unavailable right now. Please check back soon.
          </span>
        )}
      </p>
      <p className="text-sm text-faint">
        We&rsquo;ll only use your email to contact you about CAVI early access.
      </p>
    </form>
  );
}

function capitalise(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
