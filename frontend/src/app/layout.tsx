import { Providers } from "./providers";
import { ThemeToggle } from "@/components/ThemeToggle";

export const metadata = {
  title: "Langner",
  description: "Vocabulary learning quiz",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body>
        <Providers>
          <header
            style={{
              display: "flex",
              justifyContent: "flex-end",
              alignItems: "center",
              padding: "0.5rem 1rem",
              borderBottom: "1px solid var(--chakra-colors-border-subtle, #e2e8f0)",
            }}
          >
            <ThemeToggle />
          </header>
          {children}
        </Providers>
      </body>
    </html>
  );
}
