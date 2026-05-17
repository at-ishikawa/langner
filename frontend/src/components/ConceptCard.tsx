"use client";

import { Box, Heading, Text, VStack } from "@chakra-ui/react";
import { RelationChip } from "@/components/RelationChip";
import type { SemanticConcept } from "@/lib/client";

interface ConceptCardProps {
  concept: SemanticConcept;
  // currentSession highlights members declared in that session, dimming
  // the rest. Optional — when omitted, all members render at full
  // intensity. Used by the etymology notebook page to scope a card to
  // the session block it's rendered under.
  currentSession?: string;
  // meaningsByKey supplies the human-readable label for a relation
  // target. Falls back to the concept key when the lookup misses.
  meaningsByKey?: Record<string, string>;
  // onSelectConcept fires when a relation chip is clicked. Optional —
  // when omitted, chips are non-interactive. The notebook page wires
  // this to scroll/anchor the linked concept into view.
  onSelectConcept?: (conceptKey: string) => void;
}

// ConceptCard renders one semantic concept: its meaning + optional note
// in the header, its members across languages (with session provenance
// when given), and its outgoing relations as colored chips. The shape
// adapts to whether the concept has relations (compact card otherwise).
export function ConceptCard({
  concept,
  currentSession,
  meaningsByKey,
  onSelectConcept,
}: ConceptCardProps) {
  const members = concept.members ?? [];
  const relations = concept.outgoingRelations ?? [];

  return (
    <Box
      p={3}
      borderWidth="1px"
      borderRadius="lg"
      bg="white"
      _dark={{ bg: "gray.800", borderColor: "gray.600" }}
      borderColor="gray.200"
    >
      <Box display="flex" alignItems="baseline" gap={2} flexWrap="wrap" mb={1}>
        <Heading size="sm">{concept.meaning || concept.conceptKey}</Heading>
        <Text fontSize="xs" color="fg.muted" fontFamily="mono">
          {concept.conceptKey}
        </Text>
      </Box>
      {concept.note && (
        <Text fontSize="xs" color="fg.muted" mb={2}>
          {concept.note}
        </Text>
      )}
      <Box display="flex" gap={2} flexWrap="wrap" mb={relations.length > 0 ? 2 : 0}>
        {members.map((m, i) => {
          const isThisSession = !currentSession || m.sessionTitle === currentSession;
          return (
            <Box
              key={`${m.origin?.origin}-${m.origin?.language}-${m.sessionTitle}-${i}`}
              px={2}
              py={0.5}
              borderRadius="md"
              borderWidth="1px"
              opacity={isThisSession ? 1 : 0.6}
              bg={isThisSession ? "blue.50" : "transparent"}
              _dark={{ bg: isThisSession ? "blue.900" : "transparent", borderColor: "gray.600" }}
              borderColor="gray.200"
              display="inline-flex"
              alignItems="baseline"
              gap={1}
            >
              <Text fontSize="sm" fontWeight={isThisSession ? "semibold" : "normal"}>
                {m.origin?.origin}
              </Text>
              <Text fontSize="xs" color="fg.muted">
                · {m.origin?.language}
              </Text>
              {!isThisSession && m.sessionTitle && (
                <Text fontSize="2xs" color="fg.muted" fontStyle="italic">
                  ({m.sessionTitle})
                </Text>
              )}
            </Box>
          );
        })}
      </Box>
      {relations.length > 0 && (
        <VStack align="start" gap={1}>
          {relations.map((r, i) => {
            const target = meaningsByKey?.[r.toConceptKey] ?? r.toConceptKey;
            return (
              <RelationChip
                key={`${r.type}-${r.toConceptKey}-${i}`}
                type={r.type}
                label={target}
                onClick={onSelectConcept ? () => onSelectConcept(r.toConceptKey) : undefined}
              />
            );
          })}
        </VStack>
      )}
    </Box>
  );
}
