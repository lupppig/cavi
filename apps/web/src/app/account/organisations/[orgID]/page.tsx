import { Suspense } from "react";
import { OrganisationScreen } from "./organisation-screen";

// The organisation ID is only known at request time, so the screen that
// reads it (via useParams) renders inside Suspense; the fallback is the
// prerendered shell.
export default function OrganisationPage() {
  return (
    <Suspense
      fallback={
        <main aria-busy="true" className="min-h-dvh px-4 py-6 sm:px-8">
          <span className="sr-only">Loading…</span>
          <div className="mx-auto h-16 max-w-4xl animate-pulse bg-surface-2" />
          <div className="mx-auto mt-14 h-64 max-w-4xl animate-pulse bg-surface-2" />
        </main>
      }
    >
      <OrganisationScreen />
    </Suspense>
  );
}
