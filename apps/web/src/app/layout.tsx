import type { Metadata, Viewport } from "next";
import { Archivo, Geist_Mono, Instrument_Serif } from "next/font/google";
import "./globals.css";

// One type system for the whole site: Archivo (with its width axis for
// the wide display cut), Instrument Serif italics and Geist Mono.
const archivo = Archivo({ variable: "--font-archivo", subsets: ["latin"], axes: ["wdth"] });
const instrument = Instrument_Serif({ variable: "--font-instrument", subsets: ["latin"], weight: "400", style: "italic" });

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "CAVI: from the first frame to the final upload",
  description:
    "CAVI is a creative production platform in development for clippers, creators, artists, studios, agencies and brands. Join the waitlist for early access.",
};

export const viewport: Viewport = {
  themeColor: "#050505",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${archivo.variable} ${instrument.variable} ${geistMono.variable} antialiased`}>
      <body className="min-h-dvh font-sans">{children}</body>
    </html>
  );
}
