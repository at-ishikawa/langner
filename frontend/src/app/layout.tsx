import { Fraunces, Hanken_Grotesk, JetBrains_Mono } from "next/font/google";
import { AppShell } from "@/components/AppShell";
import { Providers } from "./providers";

// Reading Room typography: Fraunces (characterful serif) for headwords/titles,
// Hanken Grotesk for UI, JetBrains Mono for phonetics + SR numbers. Exposed as
// CSS variables the Chakra theme (theme/system.ts) resolves its font tokens to.
const display = Fraunces({
  subsets: ["latin"],
  variable: "--font-display",
  weight: ["400", "500", "600", "700"],
  style: ["normal", "italic"],
  display: "swap",
});
const sans = Hanken_Grotesk({
  subsets: ["latin"],
  variable: "--font-sans",
  weight: ["400", "500", "600", "700"],
  display: "swap",
});
const mono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  weight: ["400", "500", "600"],
  display: "swap",
});

export const metadata = {
  title: "Langner",
  description: "Learn English vocabulary from the books and stories you read",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${display.variable} ${sans.variable} ${mono.variable}`}
    >
      <body>
        <Providers>
          <AppShell>{children}</AppShell>
        </Providers>
      </body>
    </html>
  );
}
