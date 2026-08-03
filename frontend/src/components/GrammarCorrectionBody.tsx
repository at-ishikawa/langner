"use client";

import { Box, Text } from "@chakra-ui/react";
import { wordDiff, type DiffToken } from "@/lib/wordDiff";

// GrammarCorrectionBody is the labelled-lines + diff body the live grammar
// quiz's feedback card shows (Mistake / You wrote / Suggested / Why you
// missed it / Grammar note) — extracted so the Relearn Quiz's grammar card
// can show identical feedback without forking a second copy of the diff
// rendering. GrammarFeedbackCard wraps this with its status header and
// override/skip footer (real learning-history actions); Relearn wraps it
// with FeedbackActions instead (session-only, since Relearn persists
// nothing — see RelearnCard's doc comment in relearn.go).

// Render diffed tokens; changed words get a tinted background so they stand out.
function DiffLine({
  tokens,
  tone,
  testId,
}: {
  tokens: DiffToken[];
  tone: "user" | "correct";
  testId?: string;
}) {
  const changedBg = tone === "user" ? "red.100" : "green.100";
  const changedDarkBg = tone === "user" ? "red.900" : "green.900";
  const changedColor = tone === "user" ? "red.700" : "green.700";
  const changedDarkColor = tone === "user" ? "red.200" : "green.200";
  return (
    <Text fontSize="sm" wordBreak="break-word" data-testid={testId}>
      {tokens.map((t, i) => (
        <Text as="span" key={i}>
          {i > 0 ? " " : ""}
          {t.changed ? (
            <Text
              as="span"
              px={1}
              borderRadius="sm"
              fontWeight="bold"
              bg={changedBg}
              color={changedColor}
              _dark={{ bg: changedDarkBg, color: changedDarkColor }}
            >
              {t.text}
            </Text>
          ) : (
            t.text
          )}
        </Text>
      ))}
    </Text>
  );
}

export interface GrammarCorrectionBodyProps {
  incorrect: string;
  answer: string;
  correctAnswer: string;
  correct: boolean;
  isSkipped?: boolean;
  // assessment is the grader's critique of THIS answer (empty when correct
  // or skipped) — SubmitRelearnAnswerResponse.reason / GradeResult.Reason.
  assessment?: string;
  // grammarNote is the authored explanation for the mistake, independent of
  // this particular answer — SubmitRelearnAnswerResponse.grammar_note /
  // GrammarBlank.Reason.
  grammarNote?: string;
  // correctAnswerTestId, when set, is attached to the "Suggested" row so a
  // caller (the Relearn Quiz) can locate the reference fix the same way
  // every other Relearn format exposes its answer (data-testid=
  // relearn-answer), letting the generic relearn e2e loop drive a grammar
  // card exactly like any other.
  correctAnswerTestId?: string;
}

export function GrammarCorrectionBody({
  incorrect,
  answer,
  correctAnswer,
  correct,
  isSkipped,
  assessment,
  grammarNote,
  correctAnswerTestId,
}: GrammarCorrectionBodyProps) {
  const diff = wordDiff(answer, correctAnswer);
  return (
    <>
      {/* Mistake (original), muted — also the anchor for the wrong span. */}
      <Box display="flex" gap={2} mb={1}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Mistake
        </Text>
        <Text fontSize="sm" color="fg.muted" textDecoration="line-through" wordBreak="break-word">
          {incorrect}
        </Text>
      </Box>

      {/* You wrote — changed words vs the correction highlighted. */}
      <Box display="flex" gap={2} mb={1}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          You wrote
        </Text>
        {!isSkipped && answer.trim() ? (
          <DiffLine tokens={diff.left} tone="user" />
        ) : (
          <Text fontSize="sm" color="fg.muted" fontStyle="italic">
            (skipped)
          </Text>
        )}
      </Box>

      {/* Suggested — the words you still needed highlighted. */}
      <Box display="flex" gap={2} mb={2}>
        <Text fontSize="xs" color="fg.muted" flexShrink={0} w="4.5rem">
          Suggested
        </Text>
        <DiffLine tokens={diff.right} tone="correct" testId={correctAnswerTestId} />
      </Box>

      {/* Why you missed it — the grader's critique of THIS answer (wrong only). */}
      {!correct && assessment && (
        <Box mb={grammarNote ? 1 : 3}>
          <Text fontSize="xs" color="red.600" _dark={{ color: "red.300" }} fontWeight="medium">
            Why you missed it
          </Text>
          <Text fontSize="sm">{assessment}</Text>
        </Box>
      )}

      {/* Grammar note authored for the mistake. */}
      {grammarNote && (
        <Box mb={3}>
          <Text fontSize="xs" color="fg.muted">
            {!correct && assessment ? "Grammar note" : "Why"}
          </Text>
          <Text fontSize="sm" color="fg.muted" fontStyle="italic">
            {grammarNote}
          </Text>
        </Box>
      )}
    </>
  );
}
