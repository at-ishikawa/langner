import { Box, HStack, Text } from "@chakra-ui/react";

const GLYPH = {
  correct: "✓",
  wrong: "✗",
  none: "·",
} as const;

const COLOR = {
  correct: "green.fg",
  wrong: "red.fg",
  none: "fg.muted",
} as const;

const LABEL = {
  correct: "correct",
  wrong: "wrong",
  none: "no attempt",
} as const;

function describe(glyphs: readonly string[]): string {
  return glyphs.map((g) => LABEL[(g as keyof typeof LABEL)] ?? g).join(", ");
}

export function PatternGlyphs({ pattern }: { pattern: readonly string[] }) {
  return (
    <HStack
      gap={1}
      role="img"
      aria-label={`Recent attempts: ${describe(pattern)}`}
      fontFamily="mono"
      fontSize="md"
    >
      {pattern.map((g, i) => {
        const key = (g as keyof typeof GLYPH) in GLYPH ? (g as keyof typeof GLYPH) : "none";
        return (
          <Box
            key={i}
            color={COLOR[key]}
            minW="1ch"
            textAlign="center"
            data-testid={`pattern-glyph-${key}`}
          >
            <Text as="span">{GLYPH[key]}</Text>
          </Box>
        );
      })}
    </HStack>
  );
}
