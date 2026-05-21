"use client";

import { Box, Text, VStack } from "@chakra-ui/react";

// FormItem mirrors the EtymologyOriginForm proto message in a way that's
// independent of generated code so this component can be used both with
// proto values (camelCased after codegen) and with hand-built fixtures.
export interface FormItem {
  form: string;
  role: string;
  note?: string;
}

interface FormsTableProps {
  forms?: FormItem[];
  // highlightForm, when set, gives the row whose `form` matches a
  // highlighted background. Used to indicate "this is the form that
  // produced the English word currently in view" — set it to the
  // definition's from_form when showing forms on a derived word.
  highlightForm?: string;
}

// FormsTable renders the principal parts / inflectional variants of a
// source-language origin. Empty / undefined input renders nothing so it's
// safe to drop into any origin row unconditionally. The layout is a small
// 3-column grid on widths >= 480px and a stacked card list below that, so
// it reads well in both quiz feedback cards and side-by-side origin pages.
export function FormsTable({ forms, highlightForm }: FormsTableProps) {
  if (!forms || forms.length === 0) return null;

  const highlight = highlightForm?.trim().toLowerCase();

  return (
    <Box mt={1}>
      <Text fontSize="xs" color="fg.muted" fontWeight="medium" mb={1}>
        Forms
      </Text>
      <VStack align="stretch" gap={1}>
        {forms.map((f, i) => {
          const isMatch = highlight && f.form.trim().toLowerCase() === highlight;
          return (
            <Box
              key={`${f.form}-${f.role}-${i}`}
              display="grid"
              gridTemplateColumns={{ base: "1fr", sm: "minmax(0, 1fr) minmax(0, 1fr) minmax(0, 2fr)" }}
              gap={2}
              px={2}
              py={1}
              borderRadius="md"
              bg={isMatch ? "yellow.100" : "gray.50"}
              _dark={{ bg: isMatch ? "yellow.900" : "gray.800" }}
              borderLeftWidth={isMatch ? "3px" : "0"}
              borderLeftColor="yellow.500"
            >
              <Text fontFamily="mono" fontSize="sm" fontWeight={isMatch ? "bold" : "medium"}>
                {f.form}
              </Text>
              <Text fontSize="xs" color="fg.muted" alignSelf="center">
                {f.role}
              </Text>
              <Text fontSize="xs" color="fg.muted" alignSelf="center">
                {f.note ?? ""}
              </Text>
            </Box>
          );
        })}
      </VStack>
    </Box>
  );
}
