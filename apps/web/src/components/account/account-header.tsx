"use client";

/**
 * A message strip. Errors are red with a "!" marker, confirmations green
 * with a tick, so the meaning never rests on colour alone.
 */
export function Banner({ tone = "error", children }: { tone?: "error" | "ok"; children: React.ReactNode }) {
  const error = tone === "error";
  return (
    <p
      role={error ? "alert" : "status"}
      className={`flex items-start gap-3 rounded-xl border px-4 py-3 text-sm font-medium ${
        error ? "border-[#fecdca] bg-danger-soft text-danger" : "border-[#abefc6] bg-success-soft text-success"
      }`}
    >
      <span
        aria-hidden="true"
        className={`grid size-5 shrink-0 place-items-center rounded-full text-xs font-bold text-white ${error ? "bg-danger" : "bg-success"}`}
      >
        {error ? "!" : "✓"}
      </span>
      <span className="min-w-0">{children}</span>
    </p>
  );
}
