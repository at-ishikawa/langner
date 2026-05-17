"use client";

import { Box, Text, Input } from "@chakra-ui/react";
import { useMemo } from "react";
import type { GraphPrompt, GraphNode } from "@/gen-protos/api/v1/quiz_pb";
import { GraphPrompt_Shape, GraphNode_Kind } from "@/gen-protos/api/v1/quiz_pb";

interface RelationGraphProps {
  prompt: GraphPrompt;
  value: string;
  onValueChange: (next: string) => void;
  disabled?: boolean;
}

// RelationGraph renders the graph-quiz shape carried by `prompt` with a
// single blank node the user fills. Layout is deterministic per shape: a
// radial cluster for CLUSTER (one concept at the centre, members below),
// two side-by-side concept columns for ANTONYM_PAIR. v1 uses plain
// Chakra primitives (no reactflow) since the shapes are small and fixed.
export function RelationGraph({ prompt, value, onValueChange, disabled }: RelationGraphProps) {
  switch (prompt.shape) {
    case GraphPrompt_Shape.CLUSTER:
      return <ClusterGraph prompt={prompt} value={value} onValueChange={onValueChange} disabled={disabled} />;
    case GraphPrompt_Shape.ANTONYM_PAIR:
      return <AntonymPairGraph prompt={prompt} value={value} onValueChange={onValueChange} disabled={disabled} />;
    default:
      return (
        <Box p={3} borderWidth="1px" borderRadius="md" bg="gray.50" _dark={{ bg: "gray.800" }}>
          <Text fontSize="xs" color="fg.muted">
            (graph prompt shape not yet supported)
          </Text>
        </Box>
      );
  }
}

function ClusterGraph({ prompt, value, onValueChange, disabled }: RelationGraphProps) {
  const members = useMemo(
    () => prompt.nodes.filter((n) => n.kind === GraphNode_Kind.ORIGIN),
    [prompt.nodes],
  );
  const concept = useMemo(
    () => prompt.nodes.find((n) => n.kind === GraphNode_Kind.CONCEPT),
    [prompt.nodes],
  );
  if (!concept) return null;

  return (
    <Box>
      <Text fontSize="xs" color="fg.muted" mb={2}>
        Fill in the blank to complete this concept:
      </Text>
      <Box
        p={4}
        borderWidth="1px"
        borderRadius="lg"
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderColor="gray.200"
        textAlign="center"
      >
        <ConceptChip node={concept} />
        <Box
          display="grid"
          gridTemplateColumns={`repeat(${Math.min(members.length, 3)}, minmax(0, 1fr))`}
          gap={2}
          mt={3}
        >
          {members.map((n) => (
            <MemberNode
              key={n.id}
              node={n}
              isBlank={n.id === prompt.blankNodeId}
              value={value}
              onValueChange={onValueChange}
              disabled={disabled}
            />
          ))}
        </Box>
      </Box>
    </Box>
  );
}

function AntonymPairGraph({ prompt, value, onValueChange, disabled }: RelationGraphProps) {
  // Edges of type `member_of` identify which concept each member belongs to.
  // The single `antonym` edge tells us which two concepts to render. Concept
  // node ids are stable ("concept_a" and "concept_b") so we look them up
  // directly. Member node ids are prefixed "a<n>" or "b<n>" to identify
  // which side they belong to.
  const conceptA = useMemo(() => prompt.nodes.find((n) => n.id === "concept_a"), [prompt.nodes]);
  const conceptB = useMemo(() => prompt.nodes.find((n) => n.id === "concept_b"), [prompt.nodes]);
  const membersA = useMemo(
    () => prompt.nodes.filter((n) =>
      n.kind === GraphNode_Kind.ORIGIN && n.id.startsWith("a"),
    ),
    [prompt.nodes],
  );
  const membersB = useMemo(
    () => prompt.nodes.filter((n) =>
      n.kind === GraphNode_Kind.ORIGIN && n.id.startsWith("b"),
    ),
    [prompt.nodes],
  );
  if (!conceptA || !conceptB) return null;

  return (
    <Box>
      <Text fontSize="xs" color="fg.muted" mb={2}>
        Fill in the blank in this antonym pair:
      </Text>
      <Box
        p={4}
        borderWidth="1px"
        borderRadius="lg"
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderColor="gray.200"
      >
        <Box display="grid" gridTemplateColumns="1fr auto 1fr" gap={3} alignItems="start">
          <Box textAlign="center">
            <ConceptChip node={conceptA} />
            <Box mt={2} display="grid" gridTemplateColumns="1fr" gap={1}>
              {membersA.map((n) => (
                <MemberNode
                  key={n.id}
                  node={n}
                  isBlank={n.id === prompt.blankNodeId}
                  value={value}
                  onValueChange={onValueChange}
                  disabled={disabled}
                />
              ))}
            </Box>
          </Box>
          <Box display="flex" alignItems="center" pt={2}>
            <Box px={2} py={0.5} borderRadius="full" bg="red.100" _dark={{ bg: "red.900" }}>
              <Text fontSize="xs" color="red.800" _dark={{ color: "red.200" }} fontWeight="medium">
                ⇄ antonym
              </Text>
            </Box>
          </Box>
          <Box textAlign="center">
            <ConceptChip node={conceptB} />
            <Box mt={2} display="grid" gridTemplateColumns="1fr" gap={1}>
              {membersB.map((n) => (
                <MemberNode
                  key={n.id}
                  node={n}
                  isBlank={n.id === prompt.blankNodeId}
                  value={value}
                  onValueChange={onValueChange}
                  disabled={disabled}
                />
              ))}
            </Box>
          </Box>
        </Box>
      </Box>
    </Box>
  );
}

function ConceptChip({ node }: { node: GraphNode }) {
  return (
    <Box
      display="inline-flex"
      alignItems="baseline"
      gap={2}
      px={3}
      py={1.5}
      borderRadius="full"
      bg="purple.100"
      _dark={{ bg: "purple.900" }}
    >
      <Text fontSize="sm" color="purple.800" _dark={{ color: "purple.200" }} fontWeight="semibold">
        {node.label}
      </Text>
      {node.hint && (
        <Text fontSize="xs" color="purple.700" _dark={{ color: "purple.300" }} fontFamily="mono">
          ({node.hint})
        </Text>
      )}
    </Box>
  );
}

interface MemberNodeProps {
  node: GraphNode;
  isBlank: boolean;
  value: string;
  onValueChange: (next: string) => void;
  disabled?: boolean;
}

function MemberNode({ node, isBlank, value, onValueChange, disabled }: MemberNodeProps) {
  return (
    <Box
      p={2}
      borderWidth={isBlank ? "2px" : "1px"}
      borderRadius="md"
      borderColor={isBlank ? "yellow.500" : "gray.300"}
      bg={isBlank ? "yellow.50" : "blue.50"}
      _dark={{
        bg: isBlank ? "yellow.900" : "blue.900",
        borderColor: isBlank ? "yellow.500" : "gray.600",
      }}
      textAlign="center"
    >
      {isBlank ? (
        <Box>
          <Input
            value={value}
            onChange={(e) => onValueChange(e.target.value)}
            disabled={disabled}
            size="sm"
            placeholder="???"
            textAlign="center"
            fontFamily="mono"
            bg="white"
            _dark={{ bg: "gray.800" }}
            autoFocus
          />
          <Text fontSize="2xs" color="fg.muted" mt={1}>
            {node.hint || node.language}
          </Text>
        </Box>
      ) : (
        <>
          <Text fontFamily="mono" fontSize="sm" fontWeight="semibold">
            {node.label}
          </Text>
          <Text fontSize="2xs" color="fg.muted">
            {node.language}
          </Text>
        </>
      )}
    </Box>
  );
}
