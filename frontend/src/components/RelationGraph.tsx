"use client";

import { Box, Text, Input } from "@chakra-ui/react";
import { useMemo } from "react";
import type { GraphPrompt, GraphNode } from "@/gen-protos/api/v1/quiz_pb";
import { GraphPrompt_Shape, GraphNode_Kind } from "@/gen-protos/api/v1/quiz_pb";

interface RelationGraphProps {
  prompt: GraphPrompt;
  // value is what the user has typed into the blank.
  value: string;
  onValueChange: (next: string) => void;
  // disabled disables the input (used while submitting).
  disabled?: boolean;
}

// RelationGraph renders one of the supported graph-quiz shapes (CLUSTER
// for v1) with a single blank node the user fills. Layout is deterministic
// per shape — radial for CLUSTER, where the concept sits in the centre
// and members fan out evenly. v1 uses plain Chakra primitives (no
// reactflow) since the shapes are small and fixed.
export function RelationGraph({ prompt, value, onValueChange, disabled }: RelationGraphProps) {
  const members = useMemo(
    () => prompt.nodes.filter((n) => n.kind === GraphNode_Kind.ORIGIN),
    [prompt.nodes],
  );
  const concept = useMemo(
    () => prompt.nodes.find((n) => n.kind === GraphNode_Kind.CONCEPT),
    [prompt.nodes],
  );

  if (prompt.shape !== GraphPrompt_Shape.CLUSTER || !concept) {
    // v1 only supports CLUSTER; other shapes will be added in follow-up
    // PRs. Until then we render a minimal placeholder so the page doesn't
    // break if the backend ever emits another shape.
    return (
      <Box p={3} borderWidth="1px" borderRadius="md" bg="gray.50" _dark={{ bg: "gray.800" }}>
        <Text fontSize="xs" color="fg.muted">
          (graph prompt shape not yet supported)
        </Text>
      </Box>
    );
  }

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
      >
        <Box
          mx="auto"
          mb={3}
          px={3}
          py={1.5}
          borderRadius="full"
          bg="purple.100"
          _dark={{ bg: "purple.900" }}
          display="inline-flex"
          alignItems="baseline"
          gap={2}
          textAlign="center"
        >
          <Text fontSize="sm" color="purple.800" _dark={{ color: "purple.200" }} fontWeight="semibold">
            {concept.label}
          </Text>
          {concept.hint && (
            <Text fontSize="xs" color="purple.700" _dark={{ color: "purple.300" }} fontFamily="mono">
              ({concept.hint})
            </Text>
          )}
        </Box>
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
