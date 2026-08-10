"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Box, Heading, Spinner, Text } from "@chakra-ui/react";
import { quizClient, QuizType, type SubmitRelearnAnswerResponse } from "@/lib/client";
import { AnswerInput } from "@/components/AnswerInput";
import { FeedbackActions } from "@/components/FeedbackActions";
import { RelearnGrammarPost } from "@/components/RelearnGrammarPost";
import { RelearnOriginPost } from "@/components/RelearnOriginPost";
import { useRelearnStore } from "@/store/relearnStore";
import RelearnContext from "@/components/RelearnContext";

// sourceLabel names which quiz produced the wrong answer that pooled this card —
// and, now that relearn mirrors that quiz, which format it is presented in.
function sourceLabel(source: QuizType): string {
  switch (source) {
    case QuizType.REVERSE:
      return "Reverse — recall the word";
    default:
      return "Recognition — recall the meaning";
  }
}

type Phase = "answering" | "feedback";

export default function RelearnSessionPage() {
  const router = useRouter();
  const queue = useRelearnStore((s) => s.queue);
  const totalAnswers = useRelearnStore((s) => s.totalAnswers);
  const resolveFront = useRelearnStore((s) => s.resolveFront);
  const completePost = useRelearnStore((s) => s.completePost);
  const completeOrigin = useRelearnStore((s) => s.completeOrigin);

  const front = queue[0];
  const [phase, setPhase] = useState<Phase>("answering");
  const [answer, setAnswer] = useState("");
  const [feedback, setFeedback] = useState<SubmitRelearnAnswerResponse | null>(null);
  // override holds the learner's overriding verdict for the current card, or
  // null when they accept the grader's. It only affects this session's working
  // queue — relearn persists nothing, so there is no learning history, and no
  // relearn-local state, to reconcile.
  const [override, setOverride] = useState<boolean | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const startRef = useRef<number>(Date.now());

  // Words left across every remaining screen: a card is one word, a grammar
  // post contributes one per due blank, an origin family one per missed word.
  const wordsLeft = queue.reduce((n, item) => {
    if (item.kind === "card") return n + 1;
    if (item.kind === "post") return n + item.post.blanks.length;
    return n + item.group.words.length;
  }, 0);

  // Leaving the queue empty ends the session. A direct visit with no answers
  // yet bounces back to the start screen instead of a hollow complete page.
  useEffect(() => {
    if (queue.length === 0) {
      router.push(totalAnswers > 0 ? "/quiz/relearn/complete" : "/quiz?tab=relearn");
    }
  }, [queue.length, totalAnswers, router]);

  // Reset the per-card timer whenever a new screen reaches the front.
  useEffect(() => {
    startRef.current = Date.now();
  }, [front]);

  if (!front) {
    return null;
  }

  // A grammar post is presented once with all its due blanks, drilled
  // progressively like the live grammar quiz (see RelearnGrammarPost). It is
  // answered in a single pass and removed — never requeued.
  if (front.kind === "post") {
    return (
      <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh" p={4}>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mb={3} aria-live="polite">
          {wordsLeft} {wordsLeft === 1 ? "word" : "words"} left
        </Text>
        <RelearnGrammarPost
          key={front.post.content}
          content={front.post.content}
          blanks={front.post.blanks}
          onComplete={(correctCount, blankCount) => completePost(correctCount, blankCount)}
        />
      </Box>
    );
  }

  // An etymology origin is presented once with the missed words that share it,
  // drilled progressively like the etymology family card (see RelearnOriginPost).
  // Words answered wrong re-queue as a smaller family screen (completeOrigin) so
  // they are re-drilled this session until answered correctly; when all are
  // correct the family is dropped. The key includes group.attempt so a re-queued
  // family remounts fresh even when its wrong words are the same set.
  if (front.kind === "origin") {
    return (
      <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh" p={4}>
        <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mb={3} aria-live="polite">
          {wordsLeft} {wordsLeft === 1 ? "word" : "words"} left
        </Text>
        <RelearnOriginPost
          key={`${front.group.originText} ${front.group.originMeaning} #${front.group.attempt}`}
          originText={front.group.originText}
          originMeaning={front.group.originMeaning}
          type={front.group.type}
          language={front.group.language}
          englishForms={front.group.englishForms}
          words={front.group.words}
          onComplete={(wrongWords, correctCount) => completeOrigin(wrongWords, correctCount)}
        />
      </Box>
    );
  }

  const current = front.card;

  // Each single card mirrors the quiz type it was failed in: reverse produces
  // the word from the meaning; recognition recalls the meaning from the word.
  // (Etymology-origin misses are grouped into origin family posts above.)
  const isReverse = current.sourceQuizType === QuizType.REVERSE;
  const promptText = isReverse ? current.meaning : current.entry;
  const answerLabel = isReverse ? "The word" : "Your meaning";
  const answerPlaceholder = isReverse ? "Type the word" : "Type the meaning";

  const submit = async (isSkipped: boolean) => {
    setSubmitting(true);
    setError(null);
    setPhase("feedback");
    try {
      const res = await quizClient.submitRelearnAnswer({
        noteId: current.noteId,
        answer: isSkipped ? "" : answer,
        isSkipped,
        responseTimeMs: BigInt(Date.now() - startRef.current),
      });
      setFeedback(res);
    } catch {
      setError("Grading failed. Please try again.");
      setPhase("answering");
    } finally {
      setSubmitting(false);
    }
  };

  const handleNext = () => {
    const effective = override ?? feedback?.correct ?? false;
    // The override only reshapes this session's working queue — relearn writes
    // no state, so there is nothing to reconcile with the backend.
    setAnswer("");
    setFeedback(null);
    setOverride(null);
    setPhase("answering");
    resolveFront(effective);
  };

  return (
    <Box maxW="sm" mx="auto" bg="gray.50" _dark={{ bg: "gray.900" }} minH="100vh" p={4}>
      <Text fontSize="xs" color="gray.500" _dark={{ color: "gray.400" }} mb={3} aria-live="polite">
        {wordsLeft} {wordsLeft === 1 ? "word" : "words"} left
      </Text>

      {/* Prompt card — mirrors the source quiz's format. */}
      <Box bg="white" _dark={{ bg: "gray.800" }} borderRadius="lg" borderWidth="1px" borderColor="gray.200" p={5} mb={4}>
        <Text fontSize="xs" color="purple.500" _dark={{ color: "purple.300" }} fontWeight="medium" mb={2}>
          {sourceLabel(current.sourceQuizType)}
        </Text>

        <Heading size="lg" textAlign="center" data-testid="relearn-prompt">
          {promptText}
        </Heading>

        {/* Hints: examples for recognition, masked contexts for reverse. */}
        {!isReverse && current.examples.length > 0 && (
          <Box mt={3} display="flex" flexDirection="column" gap={1}>
            {current.examples.map((ex, i) => (
              <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }}>
                {ex.speaker ? `${ex.speaker}: ` : ""}
                {ex.text}
              </Text>
            ))}
          </Box>
        )}
        {isReverse && current.contexts.length > 0 && (
          <Box mt={3} display="flex" flexDirection="column" gap={1}>
            {current.contexts.map((c, i) => (
              <Text key={i} fontSize="sm" color="gray.600" _dark={{ color: "gray.300" }}>
                {c.maskedContext || c.context}
              </Text>
            ))}
          </Box>
        )}
      </Box>

      {phase === "answering" ? (
        <Box display="flex" flexDirection="column" gap={3}>
          <AnswerInput
            label={answerLabel}
            value={answer}
            onChange={setAnswer}
            onSubmit={() => void submit(false)}
            onSkip={() => void submit(true)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && answer.trim()) void submit(false);
            }}
            placeholder={answerPlaceholder}
          />
          {error && (
            <Text color="red.500" fontSize="sm" role="alert">
              {error}
            </Text>
          )}
        </Box>
      ) : (
        <Box display="flex" flexDirection="column" gap={3}>
          {submitting || !feedback ? (
            <Box textAlign="center" py={6}>
              <Spinner />
            </Box>
          ) : (
            // Same feedback actions (banner + Mark-as-Correct/Incorrect + Next)
            // the other quizzes use, so the UI is consistent. The override here
            // is session-only — see handleNext (it persists nothing).
            <FeedbackActions
              isCorrect={override ?? feedback.correct}
              noteId={current.noteId}
              isOverridden={override !== null}
              isSkipped={false}
              showExclude={false}
              nextLabel="Next"
              onNext={() => void handleNext()}
              onOverride={() => setOverride(!feedback.correct)}
              onUndo={() => setOverride(null)}
            >
              {/* Show the word, its correct meaning, and what the learner typed
                  so they can see exactly what was off. */}
              <Box display="flex" flexDirection="column" gap={1}>
                <Text fontWeight="bold" data-testid={isReverse ? "relearn-answer" : undefined}>
                  {current.entry}
                </Text>
                <Text fontSize="sm" color="gray.700" _dark={{ color: "gray.200" }}>
                  <Text as="span" fontWeight="semibold">Meaning: </Text>
                  <Text as="span" data-testid={isReverse ? undefined : "relearn-answer"}>
                    {feedback.meaning || current.meaning}
                  </Text>
                </Text>
                {answer.trim() && (
                  <Text
                    fontSize="sm"
                    color={(override ?? feedback.correct) ? "gray.500" : "red.600"}
                    _dark={{ color: (override ?? feedback.correct) ? "gray.400" : "red.300" }}
                  >
                    <Text as="span" fontWeight="semibold">Your answer: </Text>
                    {answer}
                  </Text>
                )}
              </Box>
              {feedback.reason && (
                <Text fontSize="sm" fontStyle="italic" color="gray.500" _dark={{ color: "gray.400" }}>
                  {feedback.reason}
                </Text>
              )}
              <RelearnContext
                entry={current.entry}
                scenes={feedback.contextScenes ?? []}
                exampleWords={[]}
              />
            </FeedbackActions>
          )}
        </Box>
      )}
    </Box>
  );
}
