"use client";

import { useTheme } from "next-themes";
import { Box } from "@chakra-ui/react";

const options = [
  { value: "light", label: "☀️", title: "Light" },
  { value: "dark", label: "🌙", title: "Dark" },
  { value: "system", label: "💻", title: "System" },
];

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  return (
    <Box display="flex" gap={1}>
      {options.map((opt) => (
        <Box
          key={opt.value}
          as="button"
          px={2}
          py={1}
          borderRadius="md"
          fontSize="sm"
          cursor="pointer"
          bg={theme === opt.value ? "bg.muted" : "transparent"}
          borderWidth="1px"
          borderColor={
            theme === opt.value ? "border.emphasized" : "transparent"
          }
          onClick={() => setTheme(opt.value)}
          title={opt.title}
        >
          {opt.label}
        </Box>
      ))}
    </Box>
  );
}
