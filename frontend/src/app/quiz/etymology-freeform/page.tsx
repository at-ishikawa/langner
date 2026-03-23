"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Box,
  Button,
  Heading,
  Input,
  Spinner,
  Text,
  VStack,
} from "@chakra-ui/react";
import { quizClient } from "@/lib/client";
import { useQuizStore } from "@/store/quizStore";
import { FeedbackActions } from "@/components/FeedbackActions";

interface OriginRow {
  origin: string;
  meaning: string;
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

export default function EtymologyFreeformQuizPage() {
  const router = useRouter();
  const quizType = useQuizStore((s) => s.quizType);
  const etymologyResults = useQuizStore((s) => s.etymologyResults);
  const storeSubmitResult = useQuizStore((s) => s.submitEtymologyResult);
  const etymologyFreeformExpressions = useQuizStore((s) => s.etymologyFreeformExpressions);
  const etymologyFreeformNextReviewDates = useQuizStore((s) => s.etymologyFreeformNextReviewDates);
  const reset = useQuizStore((s) => s.reset);

  const [word, setWord] = useState("");
  const [rows, setRows] = useState<OriginRow[]>([{ origin: "", meaning: "" }]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    reason: string;
    originGrades: Array<{
      userOrigin: string;
      userMeaning: string;
      originCorrect: boolean;
      meaningCorrect: boolean;
      correctOrigin?: { origin: string; meaning: string };
    }>;
    originParts?: Array<{ origin: string; type: string; language: string; meaning: string }>;
    relatedDefinitions: Array<{ expression: string; meaning: string; notebookName: string }>;
    nextReviewDate?: string;
    learnedAt?: string;
    noteId?: bigint;
    notebookName?: string;
    images?: string[];
  } | null>(null);
  const startTimeRef = useRef(Date.now());
  const wordInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (quizType !== "etymology-freeform") {
      router.push("/");
    }
    wordInputRef.current?.focus();
  }, [quizType, router]);

  const wordStatus: null | boolean | string = useMemo(() => {
    const trimmed = word.trim();
    if (!trimmed || etymologyFreeformExpressions.length === 0) return null;
    const lower = trimmed.toLowerCase();
    const found = etymologyFreeformExpressions.some((e) => e.toLowerCase() === lower);
    if (!found) return false;
    const nextReview = etymologyFreeformNextReviewDates[lower];
    if (nextReview) return nextReview;
    return true;
  }, [word, etymologyFreeformExpressions, etymologyFreeformNextReviewDates]);

  const addRow = () => {
    setRows([...rows, { origin: "", meaning: "" }]);
  };

  const updateRow = (index: number, field: keyof OriginRow, value: string) => {
    setRows(rows.map((r, i) => (i === index ? { ...r, [field]: value } : r)));
  };

  const removeRow = (index: number) => {
    if (rows.length <= 1) return;
    setRows(rows.filter((_, i) => i !== index));
  };

  const canSubmit = word.trim() && rows.some((r) => r.origin.trim() || r.meaning.trim());

  const handleSubmit = async () => {
    if (!canSubmit) return;
    const responseTimeMs = Date.now() - startTimeRef.current;
    setLoading(true);
    setFeedback(null);
    setError(null);

    try {
      const res = await quizClient.submitEtymologyFreeformAnswer({
        expression: word.trim(),
        answers: rows
          .filter((r) => r.origin.trim() || r.meaning.trim())
          .map((r) => ({ origin: r.origin.trim(), meaning: r.meaning.trim() })),
        responseTimeMs: BigInt(responseTimeMs),
      });

      const fb = {
        correct: res.correct,
        reason: res.reason,
        originGrades: (res.originGrades ?? []).map((g) => ({
          userOrigin: g.userOrigin,
          userMeaning: g.userMeaning,
          originCorrect: g.originCorrect,
          meaningCorrect: g.meaningCorrect,
          correctOrigin: g.correctOrigin
            ? { origin: g.correctOrigin.origin, meaning: g.correctOrigin.meaning }
            : undefined,
        })),
        // Freeform response does not include originParts; build from correctOrigin in grades
        originParts: (res.originGrades ?? [])
          .filter((g) => g.correctOrigin)
          .map((g) => ({
            origin: g.correctOrigin!.origin,
            type: "",
            language: "",
            meaning: g.correctOrigin!.meaning,
          })),
        relatedDefinitions: (res.relatedDefinitions ?? []).map((d) => ({
          expression: d.expression,
          meaning: d.meaning,
          notebookName: d.notebookName,
        })),
        nextReviewDate: res.nextReviewDate || undefined,
        learnedAt: res.learnedAt || undefined,
        noteId: res.noteId ? BigInt(res.noteId) : undefined,
        notebookName: res.notebookName || undefined,
        images: res.images.length > 0 ? res.images : undefined,
      };

      setFeedback(fb);
      storeSubmitResult({
        noteId: fb.noteId,
        expression: word.trim(),
        meaning: "",
        answer: rows.map((r) => `${r.origin}=${r.meaning}`).join(", "),
        correct: res.correct,
        reason: res.reason,
        originGrades: fb.originGrades,
        relatedDefinitions: fb.relatedDefinitions,
        originParts: fb.originParts,
        nextReviewDate: fb.nextReviewDate,
        learnedAt: fb.learnedAt,
        images: res.images.length > 0 ? res.images : undefined,
      });
    } catch {
      setError("Failed to submit answer");
    } finally {
      setLoading(false);
    }
  };

  const handleNext = () => {
    setWord("");
    setRows([{ origin: "", meaning: "" }]);
    setFeedback(null);
    setError(null);
    startTimeRef.current = Date.now();
    wordInputRef.current?.focus();
  };

  return (
    <Box p={4} maxW="sm" mx="auto" minH="100dvh">
      <Heading size="lg" mb={2}>
        Freeform Etymology
      </Heading>

      <Text mb={4} color="gray.600" _dark={{ color: "gray.400" }} fontSize="sm">
        Type any word to see its etymology. The word must exist in your etymology notebooks.
      </Text>

      {loading ? (
        <Box textAlign="center" py={8}>
          <Spinner size="lg" mb={4} />
          <Text>Checking your answer...</Text>
        </Box>
      ) : error ? (
        <VStack align="stretch" gap={4}>
          <Text color="red.500">{error}</Text>
          <Button
            w="full"
            colorPalette="blue"
            variant="outline"
            onClick={() => {
              setError(null);
              wordInputRef.current?.focus();
            }}
          >
            Retry
          </Button>
        </VStack>
      ) : feedback ? (
        <VStack align="stretch" gap={4}>
          {/* Correct/Incorrect banner */}
          <Box
            p={4}
            borderRadius="md"
            bg={feedback.correct ? "green.100" : "red.100"}
            _dark={{ bg: feedback.correct ? "green.900" : "red.900" }}
            textAlign="center"
          >
            <Text fontWeight="bold" fontSize="lg">
              {feedback.correct ? "\u2713 Correct!" : "\u2717 Incorrect"}
            </Text>
          </Box>

          {/* Word */}
          <Box>
            <Text fontWeight="bold">Word</Text>
            <Text fontSize="xl">{word}</Text>
          </Box>

          {/* Your answer section */}
          {feedback.originGrades.length > 0 && (
            <Box>
              <Text fontWeight="medium" fontSize="sm" mb={1}>Your answer</Text>
              <Box
                p={3}
                borderWidth="1.5px"
                borderRadius="lg"
                borderColor={feedback.correct ? "green.600" : "red.600"}
                bg="white"
                _dark={{ bg: "gray.800" }}
              >
                <VStack align="stretch" gap={2}>
                  {feedback.originGrades.map((g, i) => {
                    const bothCorrect = g.originCorrect && g.meaningCorrect;
                    return (
                      <Box key={i} display="flex" justifyContent="space-between" alignItems="center">
                        <Text
                          fontSize="sm"
                          textDecoration={!bothCorrect ? "line-through" : "none"}
                          color={!bothCorrect ? "red.600" : undefined}
                        >
                          {g.userOrigin} = {g.userMeaning}
                        </Text>
                        <Text
                          fontSize="sm"
                          fontWeight="medium"
                          color={bothCorrect ? "green.600" : "red.600"}
                        >
                          {bothCorrect ? "\u2713" : "\u2717"}
                        </Text>
                      </Box>
                    );
                  })}
                </VStack>
              </Box>
            </Box>
          )}

          {/* Full/Correct breakdown with badges */}
          {feedback.originParts && feedback.originParts.length > 0 && (
            <Box>
              <Text fontWeight="medium" fontSize="sm" mb={1}>
                {feedback.correct ? "Full breakdown" : "Correct breakdown"}
              </Text>
              <Box p={3} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                <VStack align="stretch" gap={2}>
                  {feedback.originParts.map((p, i) => {
                    const typeBadge = getTypeBadgeColors(p.type);
                    return (
                      <Box key={i} display="flex" gap={2} alignItems="center" flexWrap="wrap">
                        <Text color="blue.600" _dark={{ color: "blue.300" }} fontWeight="medium" fontSize="sm">{p.origin}</Text>
                        <Text fontSize="sm" color="fg.muted">= {p.meaning}</Text>
                        <Box px={2} py={0.5} borderRadius="full" bg="gray.100" _dark={{ bg: "gray.700" }}>
                          <Text fontSize="xs" color="gray.600" _dark={{ color: "gray.300" }}>{p.language}</Text>
                        </Box>
                        {p.type && (
                          <Box px={2} py={0.5} borderRadius="full" bg={typeBadge.bg} _dark={{ bg: typeBadge.darkBg }}>
                            <Text fontSize="xs" color={typeBadge.color} _dark={{ color: typeBadge.darkColor }}>{p.type}</Text>
                          </Box>
                        )}
                      </Box>
                    );
                  })}
                </VStack>
              </Box>
            </Box>
          )}

          {feedback.reason && (
            <Box>
              <Text fontWeight="bold">Reason</Text>
              <Text>{feedback.reason}</Text>
            </Box>
          )}

          {/* Related words */}
          {feedback.relatedDefinitions.length > 0 && (
            <Box>
              <Text fontWeight="medium" fontSize="sm" mb={1}>Related words</Text>
              <Box p={3} borderWidth="1px" borderRadius="lg" bg="white" _dark={{ bg: "gray.800" }}>
                <VStack align="stretch" gap={1}>
                  {feedback.relatedDefinitions.map((d, i) => (
                    <Text key={i} fontSize="sm">
                      <Text as="span" fontWeight="medium" color="blue.600" _dark={{ color: "blue.300" }}>{d.expression}</Text>
                      {" - "}{d.meaning}
                    </Text>
                  ))}
                </VStack>
              </Box>
            </Box>
          )}

          {feedback.notebookName && (
            <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }}>
              Found in: {feedback.notebookName}
            </Text>
          )}

          {feedback.images && feedback.images.length > 0 && (
            <Box display="flex" gap={2} flexWrap="wrap">
              {feedback.images.map((src, i) => (
                <img key={i} src={src} alt="" style={{ maxHeight: "150px", borderRadius: "4px" }} />
              ))}
            </Box>
          )}

          <FeedbackActions
            isCorrect={feedback.correct}
            noteId={feedback.noteId}
            nextReviewDate={feedback.nextReviewDate}
            isOverridden={false}
            isSkipped={false}
            nextLabel="Next Word"
            onNext={handleNext}
          />

          {etymologyResults.length > 0 && (
            <Button
              colorPalette="green"
              variant="outline"
              onClick={() => router.push("/quiz/complete")}
            >
              See Results
            </Button>
          )}

          <Button
            variant="ghost"
            onClick={() => {
              reset();
              router.push("/");
            }}
          >
            Back to Start
          </Button>
        </VStack>
      ) : (
        <VStack align="stretch" gap={4}>
          <Box>
            <Text fontWeight="medium" mb={1} fontSize="sm">Word</Text>
            <Input
              ref={wordInputRef}
              value={word}
              onChange={(e) => setWord(e.target.value)}
              placeholder="e.g., biology"
              size="lg"
            />
            {wordStatus === true && (
              <Text fontSize="sm" color="green.500" mt={1}>
                Found in notebooks
              </Text>
            )}
            {wordStatus === false && (
              <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} mt={1}>
                Word not found in notebooks
              </Text>
            )}
            {typeof wordStatus === "string" && (
              <Text fontSize="sm" color="orange.500" _dark={{ color: "orange.300" }} mt={1}>
                Not due until {wordStatus}
              </Text>
            )}
          </Box>

          <Box>
            <Text fontWeight="medium" fontSize="sm">What are the origins of this word?</Text>
          </Box>

          {rows.map((row, i) => (
            <Box key={i}>
              <Text fontSize="xs" color="fg.muted" mb={1}>Origin {i + 1}</Text>
              <Box display="flex" gap={2} alignItems="center">
                <Input
                  placeholder="origin..."
                  value={row.origin}
                  onChange={(e) => updateRow(i, "origin", e.target.value)}
                  flex={1}
                />
                <Text color="fg.muted">=</Text>
                <Input
                  placeholder="meaning..."
                  value={row.meaning}
                  onChange={(e) => updateRow(i, "meaning", e.target.value)}
                  flex={1}
                />
                {rows.length > 1 && (
                  <Text
                    color="red.500"
                    cursor="pointer"
                    fontSize="sm"
                    onClick={() => removeRow(i)}
                  >
                    x
                  </Text>
                )}
              </Box>
            </Box>
          ))}

          <Text
            color="blue.600"
            _dark={{ color: "blue.300" }}
            fontSize="sm"
            cursor="pointer"
            fontWeight="medium"
            onClick={addRow}
          >
            + Add origin
          </Text>

          <Button
            colorPalette="blue"
            onClick={handleSubmit}
            disabled={!canSubmit || wordStatus === false || typeof wordStatus === "string"}
            size="lg"
          >
            Submit
          </Button>

          {etymologyResults.length > 0 && (
            <Button
              colorPalette="green"
              variant="outline"
              onClick={() => router.push("/quiz/complete")}
            >
              See Results
            </Button>
          )}

          <Text fontSize="sm" color="gray.500" _dark={{ color: "gray.400" }} textAlign="center">
            {etymologyFreeformExpressions.length} words available in your notebooks
          </Text>
        </VStack>
      )}
    </Box>
  );
}
