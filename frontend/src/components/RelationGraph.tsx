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
  /** compact forces a single-column layout regardless of viewport width.
   * Use when the graph is rendered inside a narrow container (e.g. the
   * QuizResultCard feedback row, which lives inside a maxW="sm" page).
   * Without this, Chakra's sm: rules fire based on viewport — not the
   * surrounding container — so the multi-column desktop layout activates
   * even when the parent card is only ~480px wide, overflowing the card
   * border. Single-column is the safer default in nested contexts. */
  compact?: boolean;
}

// RelationGraph renders the graph-quiz shape carried by `prompt` with a
// single blank node the user fills. Layout is deterministic per shape: a
// radial cluster for CLUSTER (one concept at the centre, members below),
// two side-by-side concept columns for ANTONYM_PAIR. v1 uses plain
// Chakra primitives (no reactflow) since the shapes are small and fixed.
export function RelationGraph({ prompt, value, onValueChange, disabled, compact }: RelationGraphProps) {
  switch (prompt.shape) {
    case GraphPrompt_Shape.CLUSTER:
      return <ClusterGraph prompt={prompt} value={value} onValueChange={onValueChange} disabled={disabled} compact={compact} />;
    case GraphPrompt_Shape.ANTONYM_PAIR:
      return <AntonymPairGraph prompt={prompt} value={value} onValueChange={onValueChange} disabled={disabled} compact={compact} />;
    case GraphPrompt_Shape.FORM_BRANCH:
      return <FormBranchGraph prompt={prompt} value={value} onValueChange={onValueChange} disabled={disabled} compact={compact} />;
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

function FormBranchGraph({ prompt, value, onValueChange, disabled, compact }: RelationGraphProps) {
  // Layout: ORIGIN node (the blank, with meaning as hint) at the top,
  // FORM nodes in a row below, ENGLISH_WORD nodes under each form. The
  // backend always blanks the origin; the user types the headword given
  // the form tree and English derivations as context.
  const origin = useMemo(
    () => prompt.nodes.find((n) => n.kind === GraphNode_Kind.ORIGIN),
    [prompt.nodes],
  );
  const forms = useMemo(
    () => prompt.nodes.filter((n) => n.kind === GraphNode_Kind.FORM),
    [prompt.nodes],
  );
  // english by form: prompt edges of type "derives" go from FORM → ENGLISH_WORD.
  const englishByFormId = useMemo(() => {
    const map = new Map<string, GraphNode[]>();
    for (const f of forms) map.set(f.id, []);
    for (const e of prompt.edges) {
      if (e.type !== "derives") continue;
      const target = prompt.nodes.find((n) => n.id === e.to);
      if (!target || target.kind !== GraphNode_Kind.ENGLISH_WORD) continue;
      const list = map.get(e.from);
      if (list) list.push(target);
    }
    return map;
  }, [prompt.edges, prompt.nodes, forms]);
  if (!origin) return null;

  // Forms grid: single column on phones (<480px), then up to three
  // columns on sm+ to keep the original tree shape on tablets/desktop.
  // Three 110px-wide form tiles on a 375px phone wrap or overflow when
  // labels are full Latin headwords like "missum" / "mittere" plus the
  // derived English words underneath. Compact mode (nested inside a
  // narrow card) stays single-column regardless of viewport.
  const formCols = `repeat(${Math.min(forms.length, 3)}, minmax(0, 1fr))`;
  const formColsResponsive = compact ? { base: "1fr" } : { base: "1fr", sm: formCols };
  return (
    <Box>
      <Text fontSize="xs" color="fg.muted" mb={2}>
        Fill in the source-language headword that produced these forms and English words:
      </Text>
      <Box
        p={{ base: 3, sm: 4 }}
        borderWidth="1px"
        borderRadius="lg"
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderColor="gray.200"
        textAlign="center"
      >
        <MemberNode
          node={origin}
          isBlank={origin.id === prompt.blankNodeId}
          value={value}
          onValueChange={onValueChange}
          disabled={disabled}
        />
        <Box
          display="grid"
          gridTemplateColumns={formColsResponsive}
          gap={2}
          mt={3}
        >
          {forms.map((f) => (
            <Box key={f.id} textAlign="center">
              <Box
                px={2}
                py={1}
                borderRadius="md"
                bg="green.50"
                _dark={{ bg: "green.900", borderColor: "gray.600" }}
                borderWidth="1px"
                borderColor="gray.300"
              >
                <Text fontFamily="mono" fontSize="sm" fontWeight="medium">
                  {f.label}
                </Text>
                {f.hint && (
                  <Text fontSize="2xs" color="fg.muted">
                    {f.hint}
                  </Text>
                )}
              </Box>
              <Box mt={1} display="flex" flexDirection="column" gap={1}>
                {(englishByFormId.get(f.id) ?? []).map((eng) => (
                  <Text key={eng.id} fontSize="xs" color="fg.muted">
                    → {eng.label}
                  </Text>
                ))}
              </Box>
            </Box>
          ))}
        </Box>
      </Box>
    </Box>
  );
}

function ClusterGraph({ prompt, value, onValueChange, disabled, compact }: RelationGraphProps) {
  const members = useMemo(
    () => prompt.nodes.filter((n) => n.kind === GraphNode_Kind.ORIGIN),
    [prompt.nodes],
  );
  const concept = useMemo(
    () => prompt.nodes.find((n) => n.kind === GraphNode_Kind.CONCEPT),
    [prompt.nodes],
  );
  if (!concept) return null;

  // Member grid is a single column on phones (<480px), then up to three
  // columns on sm+ to preserve the radial-cluster visual on tablets and
  // desktop. A 3-column grid on a 375px phone leaves ~110px per tile —
  // not enough for Latin/Greek labels like "philanthropy". Compact mode
  // (nested inside a narrow feedback card) stays single-column on every
  // viewport so the graph cannot overflow the parent card border.
  const desktopCols = `repeat(${Math.min(members.length, 3)}, minmax(0, 1fr))`;
  const memberColsResponsive = compact ? { base: "1fr" } : { base: "1fr", sm: desktopCols };
  return (
    <Box>
      <Text fontSize="xs" color="fg.muted" mb={2}>
        Fill in the blank to complete this concept:
      </Text>
      <Box
        p={{ base: 3, sm: 4 }}
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
          gridTemplateColumns={memberColsResponsive}
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

function AntonymPairGraph({ prompt, value, onValueChange, disabled, compact }: RelationGraphProps) {
  // Edges of type `member_of` identify which concept each member belongs to.
  // The single `antonym` edge tells us which two concepts to render. Concept
  // node ids are stable ("concept_a" and "concept_b") so we look them up
  // directly. Member node ids are prefixed "a<n>" or "b<n>" to identify
  // which side they belong to.
  //
  // Layout: a 3-column grid (concept A | arrow | concept B) on screens
  // >= 480px (sm breakpoint), and a single-column vertical stack below
  // that — the antonym arrow rotates ⇄ → ⇅ to match the new direction.
  // Avoids cramped 130px columns and overflowing labels on phones.
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
        p={{ base: 3, sm: 4 }}
        borderWidth="1px"
        borderRadius="lg"
        bg="white"
        _dark={{ bg: "gray.800", borderColor: "gray.600" }}
        borderColor="gray.200"
      >
        <Box
          display="grid"
          gridTemplateColumns={compact ? { base: "1fr" } : { base: "1fr", sm: "1fr auto 1fr" }}
          gap={3}
          alignItems={compact ? "stretch" : { base: "stretch", sm: "start" }}
        >
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
          <Box
            display="flex"
            alignItems="center"
            justifyContent="center"
            pt={{ base: 0, sm: 2 }}
          >
            <Box px={2} py={0.5} borderRadius="full" bg="red.100" _dark={{ bg: "red.900" }}>
              <Text fontSize="xs" color="red.800" _dark={{ color: "red.200" }} fontWeight="medium">
                <Text as="span" display={compact ? "none" : { base: "none", sm: "inline" }}>⇄ antonym</Text>
                <Text as="span" display={compact ? "inline" : { base: "inline", sm: "none" }}>⇅ antonym</Text>
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
      p={{ base: 3, sm: 2 }}
      minH={{ base: "44px", sm: "auto" }}
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
            size="md"
            placeholder="???"
            textAlign="center"
            fontFamily="mono"
            // 16px+ font-size prevents iOS Safari from auto-zooming the
            // viewport on focus, which would break the carefully sized
            // grid layout. Chakra's "md" size already lands at 16px;
            // pinning the inline style here makes the rule explicit and
            // survives future theme changes that might shrink size="md".
            fontSize="16px"
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
