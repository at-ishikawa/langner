"use client";

import { Box, Button, Text } from "@chakra-ui/react";

export interface OriginPartDisplay {
  origin: string;
  meaning: string;
  language?: string;
  type?: string;
}

export interface ResultItem {
  index: number;
  key: string;
  entry: string;
  meaning: string;
  correct: boolean;
  contexts?: string[];
  noteId?: bigint;
  learnedAt?: string;
  isOverridden?: boolean;
  isSkipped?: boolean;
  originalCorrect: boolean;
  originBreakdown?: OriginPartDisplay[];
  userAnswer?: string;
  images?: string[];
  reason?: string;
  pronunciation?: string;
  partOfSpeech?: string;
}

function getTypeBadgeColors(type: string): { bg: string; darkBg: string; color: string; darkColor: string } {
  switch (type.toLowerCase()) {
    case "root":
      return { bg: "blue.100", darkBg: "blue.900", color: "blue.600", darkColor: "blue.300" };
    case "prefix":
      return { bg: "yellow.100", darkBg: "yellow.900", color: "yellow.800", darkColor: "yellow.200" };
    case "suffix":
      return { bg: "green.100", darkBg: "green.900", color: "green.800", darkColor: "green.200" };
    default:
      return { bg: "gray.100", darkBg: "gray.700", color: "gray.600", darkColor: "gray.300" };
  }
}

interface QuizResultCardProps {
  item: ResultItem;
  isEtymology: boolean;
  onOverride: (item: ResultItem) => void;
  onUndo: (item: ResultItem) => void;
  onSkip: (item: ResultItem) => void;
  onResume: (item: ResultItem) => void;
}

export function QuizResultCard({
  item,
  isEtymology,
  onOverride,
  onUndo,
  onSkip,
  onResume,
}: QuizResultCardProps) {

  const borderColor = item.isSkipped
    ? "gray.200"
    : item.correct
      ? "green.200"
      : "red.200";

  const topBarColor = item.isSkipped
    ? "gray.300"
    : item.correct
      ? "green.600"
      : "red.600";

  return (
    <Box
      borderWidth="1px"
      borderColor={borderColor}
      borderRadius="md"
      overflow="hidden"
      opacity={item.isSkipped ? 0.6 : 1}
    >
      {/* Color bar at top */}
      <Box h="4px" bg={topBarColor} />

      <Box p={2}>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Box flex="1" minW={0}>
            <Text fontWeight="bold">
              {item.entry}
              {(item.pronunciation || item.partOfSpeech) && (
                <Text as="span" fontSize="xs" color="fg.muted" fontWeight="normal">
                  {" "}
                  {[
                    item.pronunciation && `/${item.pronunciation}/`,
                    item.partOfSpeech,
                  ].filter(Boolean).join(" · ")}
                </Text>
              )}
            </Text>
          </Box>
          {item.isSkipped && (
            <Box bg="gray.100" _dark={{ bg: "gray.700" }} px={2} py={0.5} borderRadius="sm">
              <Text fontSize="xs" color="fg.muted" fontStyle="italic">Excluded</Text>
            </Box>
          )}
        </Box>
        <Text fontSize="sm">{item.meaning}</Text>

        {/* User's answer (for non-etymology types — etymology renders below) */}
        {!isEtymology && item.userAnswer && (
          <Text fontSize="xs" mt={1}>
            <Text as="span" color="fg.muted">Your answer: </Text>
            <Text
              as="span"
              textDecoration={item.correct ? "none" : "line-through"}
              color={item.correct ? undefined : "red.600"}
              _dark={{ color: item.correct ? undefined : "red.400" }}
            >
              {item.userAnswer}
            </Text>
          </Text>
        )}

        {/* Reason / explanation */}
        {item.reason && (
          <Text fontSize="xs" color="fg.muted" mt={1}>
            {item.reason}
          </Text>
        )}

        {item.contexts?.map((ctx, i) => (
          <Text key={i} fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
            {ctx}
          </Text>
        ))}

        {/* Images */}
        {item.images && item.images.length > 0 && (
          <Box display="flex" gap={2} mt={2} flexWrap="wrap">
            {item.images.map((src, i) => (
              <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
            ))}
          </Box>
        )}

        {/* Etymology origin breakdown with badges */}
        {isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
          <Box mt={2}>
            {/* Show user answer for incorrect results */}
            {!item.correct && item.userAnswer && (
              <Text fontSize="xs" color="fg.muted" mb={1}>
                Your answer: {item.userAnswer}
              </Text>
            )}
            <Text fontSize="xs" color={item.correct ? "green.600" : "fg.muted"} mb={1}>
              {item.correct ? "Breakdown:" : "Correct:"}
            </Text>
            <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
              {item.originBreakdown.map((p, i) => {
                const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
                return (
                  <Box key={i} display="flex" alignItems="center" gap={1}>
                    {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                    <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                    <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                    {p.language && (
                      <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                        <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                      </Box>
                    )}
                    {typeBadge && p.type && (
                      <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                        <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
                      </Box>
                    )}
                  </Box>
                );
              })}
            </Box>
          </Box>
        )}

        {/* Non-etymology origin breakdown (with type/language badges, matching feedback screen) */}
        {!isEtymology && item.originBreakdown && item.originBreakdown.length > 0 && (
          <Box mt={2}>
            <Text fontSize="xs" color="fg.muted" mb={1}>Etymology</Text>
            <Box display="flex" gap={1} alignItems="center" flexWrap="wrap">
              {item.originBreakdown.map((p, i) => {
                const typeBadge = p.type ? getTypeBadgeColors(p.type) : null;
                return (
                  <Box key={i} display="flex" alignItems="center" gap={1}>
                    {i > 0 && <Text fontSize="xs" color="fg.muted">+</Text>}
                    <Text fontSize="xs" color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium">{p.origin}</Text>
                    <Text fontSize="xs" color="fg.muted">({p.meaning})</Text>
                    {p.language && (
                      <Box px={1.5} py={0} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                        <Text fontSize="2xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                      </Box>
                    )}
                    {typeBadge && p.type && (
                      <Box px={1.5} py={0} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                        <Text fontSize="2xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
                      </Box>
                    )}
                  </Box>
                );
              })}
            </Box>
          </Box>
        )}

        {/* Override button */}
        {!item.isOverridden && !item.isSkipped && item.noteId && item.learnedAt && (
          <Button
            w="full"
            mt={2}
            size="sm"
            variant="outline"
            colorPalette={item.correct ? "red" : "blue"}
            onClick={() => onOverride(item)}
          >
            {item.correct ? "Mark as Incorrect" : "Mark as Correct"}
          </Button>
        )}

        {item.isOverridden && (
          <Text mt={2} fontSize="xs" color="fg.muted" fontStyle="italic">
            {item.correct ? "Marked as correct" : "Marked as incorrect"} (overridden){" "}
            <Text
              as="span"
              color="blue.600"
              cursor="pointer"
              textDecoration="underline"
              onClick={() => onUndo(item)}
            >
              Undo
            </Text>
          </Text>
        )}

        {/* Skip button, Resume button, or nothing */}
        {item.isSkipped ? (
          item.noteId ? (
            <Button
              w="full"
              mt={2}
              size="sm"
              variant="outline"
              colorPalette="blue"
              onClick={() => onResume(item)}
            >
              Resume
            </Button>
          ) : null
        ) : !item.isOverridden && item.noteId ? (
          <Button
            w="full"
            mt={2}
            size="sm"
            variant="outline"
            colorPalette="gray"
            onClick={() => onSkip(item)}
          >
            Exclude from Quizzes
          </Button>
        ) : null}
      </Box>
    </Box>
  );
}
