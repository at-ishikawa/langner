import { Providers } from "./providers";

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
    <html lang="en">
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
