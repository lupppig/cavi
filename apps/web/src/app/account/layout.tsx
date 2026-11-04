import type { Metadata } from "next";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: "Your account · CAVI",
  robots: { index: false },
};

export default function AccountLayout({ children }: LayoutProps<"/account">) {
  return <Providers>{children}</Providers>;
}
