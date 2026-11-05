import { Suspense } from "react";
import { ProjectScreen } from "./project-screen";

// The IDs are only known at request time, so the screen that reads them
// (via useParams) renders inside Suspense; the fallback is the prerendered
// shell.
export default function ProjectPage() {
  return (
    <Suspense
      fallback={
        <main aria-busy="true" className="min-h-dvh px-4 py-6 sm:px-8">
          <span className="sr-only">Loading…</span>
          <div className="mx-auto h-16 max-w-4xl animate-pulse rounded-full bg-paper-deep" />
          <div className="mx-auto mt-14 h-64 max-w-4xl animate-pulse rounded-3xl bg-paper-deep" />
        </main>
      }
    >
      <ProjectScreen />
    </Suspense>
  );
}
