"use client";

import { ChakraProvider } from "@chakra-ui/react";
import { ThemeProvider } from "next-themes";
import { SessionProvider } from "@/components/SessionProvider";
import { system } from "@/theme/system";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ChakraProvider value={system}>
      <ThemeProvider attribute="class" disableTransitionOnChange>
        <SessionProvider>{children}</SessionProvider>
      </ThemeProvider>
    </ChakraProvider>
  );
}
