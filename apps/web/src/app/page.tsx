import { Suspense } from "react";
import { ApiStatus, ApiStatusPending } from "./api-status";

export default function Home() {
  return (
    <main className="flex min-h-dvh flex-col justify-between px-6 py-8 sm:px-12 sm:py-12">
      <header className="flex items-baseline justify-between font-mono text-xs uppercase tracking-[0.2em]">
        <span>CAVI</span>
        <span className="text-muted">In development</span>
      </header>

      <section className="max-w-4xl">
        <h1 className="text-5xl font-semibold leading-[0.95] tracking-tight sm:text-7xl md:text-8xl">
          From the first frame to the final upload.
        </h1>
        <p className="mt-8 max-w-xl text-lg text-muted">
          CAVI is a creative production platform for clippers, creators,
          artists, studios and brands. It is being built now. Nothing here is
          ready to use yet.
        </p>
      </section>

      <footer className="font-mono text-xs uppercase tracking-[0.15em]">
        <Suspense fallback={<ApiStatusPending />}>
          <ApiStatus />
        </Suspense>
      </footer>
    </main>
  );
}
