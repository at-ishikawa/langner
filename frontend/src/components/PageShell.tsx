"use client";

import { Box, Heading, type BoxProps } from "@chakra-ui/react";
import type { ReactNode } from "react";

interface PageShellProps {
  children: ReactNode;
  // Optional page title rendered as a consistent top heading.
  title?: ReactNode;
  // Max content width (a Chakra sizes token). Defaults to a readable column.
  maxW?: BoxProps["maxW"];
  // Reserve bottom padding for a page with a sticky/fixed bottom action bar so
  // the bar never covers the last of the content.
  stickyFooter?: boolean;
  // Padding overrides for the rare page that manages its own inner padding
  // (e.g. a tabbed hub) or needs extra bottom room for a pinned bottom sheet.
  px?: BoxProps["px"];
  py?: BoxProps["py"];
  pb?: BoxProps["pb"];
}

// PageShell is the single page container: one consistent centered width, one
// consistent padding, one heading placement — replacing the per-page
// `<Box maxW=… mx="auto" p=…>` boilerplate that drifted across the app.
export function PageShell({
  children,
  title,
  maxW = "3xl",
  stickyFooter = false,
  px = 4,
  py = 6,
  pb,
}: PageShellProps) {
  return (
    <Box maxW={maxW} mx="auto" px={px} py={py} pb={pb ?? (stickyFooter ? 24 : py)}>
      {title != null && (
        <Heading size="lg" mb={4}>
          {title}
        </Heading>
      )}
      {children}
    </Box>
  );
}
