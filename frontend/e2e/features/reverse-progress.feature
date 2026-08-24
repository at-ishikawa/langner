Feature: Reverse quiz progress reduces the due pool
  Answering a Reverse vocabulary word correctly must drop that word out of its
  notebook's "due for reverse review" count on the next visit to the quiz start
  screen. Concretely this guards that a correct reverse answer advances a word
  that already has prior (imported) reverse history: the DB read must order a
  word's logs newest-first so the runtime "understood" answer lands at [0]
  instead of a stale imported `misunderstood` reverse log — the DB-log-ordering
  regression that otherwise kept an answered word due forever.

  Harness realities (see reference_langner_e2e_harness): the stack runs the Go
  server in DB mode against ONE seeded Postgres, workers:1, with a full state
  reset before every scenario. Because cross-scenario spaced-repetition state
  accumulates, we NEVER assert an absolute count. Instead this scenario captures
  the count for the one word it answers and asserts a RELATIVE drop of exactly
  one.

  # The seeded "Idioms" notebook has two reverse-eligible cards ("break the ice"
  # and "lose one's temper"), each carrying a stale `misunderstood` reverse log
  # (2025-01-02, interval 1 day) that leaves both due for reverse review at the
  # baseline — so the notebook's reverse count is > 0 without the unstudied
  # toggle. We answer ONLY "break the ice" correctly and leave "lose one's
  # temper" untouched, so the notebook stays visible (count 1, not 0) and the
  # drop is attributable to the single word we answered.
  Scenario: Answering a Reverse word correctly reduces the notebook's due count
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    # Feedback interval 1 flushes (persists) the first answer on its own batch,
    # so "break the ice" is written to the DB while "lose one's temper" — never
    # submitted — is left due.
    And I set the feedback interval to 1
    And I capture the reverse review count for the "Idioms" notebook
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the heading "a way to start a conversation in a social setting"
    When I type the answer "break the ice"
    And I submit my answer
    # Waits for grading to finish (the persist RPC completes) before we leave;
    # advancing to "lose one's temper" writes nothing for it.
    And I continue to the next card

    # Back on the start screen the freshly-loaded count must reflect the write.
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    Then the reverse review count for the "Idioms" notebook dropped by one
