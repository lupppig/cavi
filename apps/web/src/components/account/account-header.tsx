"use client";

/**
 * A message strip. Errors are solid black with a "!" marker so they stand
 * out without colour; confirmations are a light panel with a tick.
 */
export function Banner({ tone = "error", children }: { tone?: "error" | "ok"; children: React.ReactNode }) {
  const error = tone === "error";
  return (
    <p
      role={error ? "alert" : "status"}
      className={`flex items-start gap-3 border-2 border-ink px-4 py-3 font-bold ${error ? "bg-ink text-paper" : "bg-paper-deep"}`}
    >
      <span
        aria-hidden="true"
        className={`grid size-6 shrink-0 place-items-center font-mono text-sm font-black ${error ? "bg-paper text-ink" : "bg-ink text-paper"}`}
      >
        {error ? "!" : "✓"}
      </span>
      <span className="min-w-0 pt-px">{children}</span>
    </p>
  );
}
