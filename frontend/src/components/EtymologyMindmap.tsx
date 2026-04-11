"use client";

import { useMemo, useCallback, useState } from "react";
import {
  ReactFlow,
  type Node,
  type Edge,
  type NodeMouseHandler,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Box, Text } from "@chakra-ui/react";
import type {
  EtymologyOriginPart,
  EtymologyDefinition,
} from "@/lib/client";

const originTypeColors: Record<string, string> = {
  root: "#3182CE",
  prefix: "#D69E2E",
  suffix: "#38A169",
};

const defaultOriginColor = "#718096";

function getOriginColor(type: string | undefined): string {
  return originTypeColors[type?.toLowerCase() ?? ""] ?? defaultOriginColor;
}

function buildRadialGraph(
  focusedOrigin: string,
  origins: EtymologyOriginPart[],
  definitions: EtymologyDefinition[],
): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = [];
  const edges: Edge[] = [];
  const originMap = new Map(origins.map((o) => [o.origin, o]));
  const focusedData = originMap.get(focusedOrigin);

  nodes.push({
    id: `origin-${focusedOrigin}`,
    position: { x: 0, y: 0 },
    data: {
      label: `${focusedOrigin}\n(${focusedData?.meaning ?? ""})`,
    },
    style: {
      background: getOriginColor(focusedData?.type),
      color: "#fff",
      border: `3px solid ${getOriginColor(focusedData?.type)}`,
      borderRadius: "12px",
      padding: "12px 16px",
      fontSize: "15px",
      fontWeight: 700,
      whiteSpace: "pre-line" as const,
      textAlign: "center" as const,
      minWidth: "100px",
      zIndex: 10,
    },
  });

  const relatedDefs = definitions.filter((d) =>
    d.originParts.some((p) => p.origin === focusedOrigin),
  );

  const seenWords = new Set<string>();
  const uniqueDefs: EtymologyDefinition[] = [];
  for (const d of relatedDefs) {
    if (!seenWords.has(d.expression)) {
      seenWords.add(d.expression);
      uniqueDefs.push(d);
    }
  }

  const wordRadius = Math.max(250, uniqueDefs.length * 50);
  const wordAngleStep = (2 * Math.PI) / Math.max(uniqueDefs.length, 1);
  const secondaryOrigins = new Map<string, { origin: EtymologyOriginPart; wordIds: string[] }>();

  uniqueDefs.forEach((def, i) => {
    const angle = i * wordAngleStep - Math.PI / 2;
    const x = wordRadius * Math.cos(angle);
    const y = wordRadius * Math.sin(angle);
    const wordId = `word-${def.expression}`;

    nodes.push({
      id: wordId,
      position: { x, y },
      data: { label: def.expression, meaning: def.meaning },
      style: {
        background: "#EDF2F7",
        color: "#2D3748",
        border: "2px solid #A0AEC0",
        borderRadius: "8px",
        padding: "8px 12px",
        fontSize: "13px",
        fontWeight: 600,
        textAlign: "center" as const,
        minWidth: "70px",
      },
    });

    edges.push({
      id: `edge-${focusedOrigin}-${def.expression}`,
      source: `origin-${focusedOrigin}`,
      target: wordId,
      style: {
        stroke: getOriginColor(focusedData?.type),
        strokeWidth: 2,
      },
    });

    for (const part of def.originParts) {
      if (part.origin === focusedOrigin) continue;
      if (!secondaryOrigins.has(part.origin)) {
        const data = originMap.get(part.origin);
        if (data) {
          secondaryOrigins.set(part.origin, { origin: data, wordIds: [] });
        }
      }
      secondaryOrigins.get(part.origin)?.wordIds.push(wordId);
    }
  });

  const secondaryList = Array.from(secondaryOrigins.entries());
  const outerRadius = wordRadius + Math.max(200, secondaryList.length * 30);
  const outerAngleStep = (2 * Math.PI) / Math.max(secondaryList.length, 1);

  secondaryList.forEach(([originName, { origin, wordIds }], i) => {
    const angle = i * outerAngleStep - Math.PI / 2;
    const x = outerRadius * Math.cos(angle);
    const y = outerRadius * Math.sin(angle);
    const color = getOriginColor(origin.type);

    nodes.push({
      id: `origin-${originName}`,
      position: { x, y },
      data: {
        label: `${originName}\n(${origin.meaning})`,
      },
      style: {
        background: color,
        color: "#fff",
        border: `2px solid ${color}`,
        borderRadius: "8px",
        padding: "8px 12px",
        fontSize: "12px",
        fontWeight: 600,
        whiteSpace: "pre-line" as const,
        textAlign: "center" as const,
        minWidth: "70px",
        opacity: 0.85,
      },
    });

    for (const wordId of wordIds) {
      edges.push({
        id: `edge-${originName}-${wordId}`,
        source: `origin-${originName}`,
        target: wordId,
        style: {
          stroke: color,
          strokeWidth: 1.5,
          opacity: 0.5,
          strokeDasharray: "5,5",
        },
      });
    }
  });

  return { nodes, edges };
}

/**
 * Inner component that holds React Flow state.
 * Keyed by focusedOrigin in the parent so it remounts when the origin changes.
 */
function MindmapInner({
  nodes: initialNodes,
  edges: initialEdges,
  focusedOrigin,
  onSelectOrigin,
  onSelectWord,
}: {
  nodes: Node[];
  edges: Edge[];
  focusedOrigin: string;
  onSelectOrigin: (origin: string) => void;
  onSelectWord: (expression: string, meaning: string) => void;
}) {
  const [nodes, , onNodesChange] = useNodesState(initialNodes);
  const [edges, , onEdgesChange] = useEdgesState(initialEdges);

  const onNodeClick: NodeMouseHandler = useCallback(
    (_event, node) => {
      if (node.id.startsWith("origin-")) {
        const originName = node.id.replace("origin-", "");
        if (originName !== focusedOrigin) {
          onSelectOrigin(originName);
        }
      } else if (node.id.startsWith("word-")) {
        const expression = node.id.replace("word-", "");
        const meaning = (node.data as { meaning?: string }).meaning ?? "";
        onSelectWord(expression, meaning);
      }
    },
    [focusedOrigin, onSelectOrigin, onSelectWord],
  );

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onNodeClick={onNodeClick}
      fitView
      minZoom={0.1}
      maxZoom={2}
      proOptions={{ hideAttribution: true }}
    >
      <Background />
      <Controls />
    </ReactFlow>
  );
}

export default function EtymologyMindmap({
  focusedOrigin,
  origins,
  definitions,
  onSelectOrigin,
}: {
  focusedOrigin: string;
  origins: EtymologyOriginPart[];
  definitions: EtymologyDefinition[];
  onSelectOrigin: (origin: string) => void;
}) {
  const { nodes, edges } = useMemo(
    () => buildRadialGraph(focusedOrigin, origins, definitions),
    [focusedOrigin, origins, definitions],
  );

  const [selectedWord, setSelectedWord] = useState<{
    expression: string;
    meaning: string;
  } | null>(null);

  const onSelectWord = useCallback(
    (expression: string, meaning: string) => {
      setSelectedWord((prev) =>
        prev?.expression === expression ? null : { expression, meaning },
      );
    },
    [],
  );

  return (
    <div style={{ width: "100%", height: "100%", position: "relative" }}>
      {/* Key forces remount when focusedOrigin changes */}
      <MindmapInner
        key={focusedOrigin}
        nodes={nodes}
        edges={edges}
        focusedOrigin={focusedOrigin}
        onSelectOrigin={onSelectOrigin}
        onSelectWord={onSelectWord}
      />

      {/* Word meaning tooltip */}
      {selectedWord && (
        <Box
          position="absolute"
          bottom={4}
          left="50%"
          transform="translateX(-50%)"
          bg="white"
          _dark={{ bg: "gray.800", borderColor: "gray.600" }}
          borderWidth="1px"
          borderColor="gray.300"
          borderRadius="lg"
          px={4}
          py={3}
          boxShadow="md"
          zIndex={20}
          maxW="90%"
          textAlign="center"
          cursor="pointer"
          onClick={() => setSelectedWord(null)}
        >
          <Text fontWeight="bold" fontSize="sm">
            {selectedWord.expression}
          </Text>
          <Text fontSize="sm" color="gray.600" _dark={{ color: "gray.400" }}>
            {selectedWord.meaning}
          </Text>
        </Box>
      )}
    </div>
  );
}
