"use client";

import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ThemeProvider } from "next-themes";
import { SessionProvider } from "@/components/SessionProvider";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <ChakraProvider value={defaultSystem}>
      <ThemeProvider attribute="class" disableTransitionOnChange>
        <SessionProvider>{children}</SessionProvider>
      </ThemeProvider>
    </ChakraProvider>
  );
}
