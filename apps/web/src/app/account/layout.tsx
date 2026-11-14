import type { Metadata } from "next";
import { Suspense } from "react";
import { AppShell } from "@/components/account/app-shell";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: "Your account · CAVI",
  robots: { index: false },
};

export default function AccountLayout({ children }: LayoutProps<"/account">) {
  return (
    <Providers>
      <Suspense>
        <AppShell>{children}</AppShell>
      </Suspense>
    </Providers>
  );
}
