Feature: Relearn Quiz
  The Relearn Quiz gathers words the learner recently got wrong across all quiz
  types into one looping recognition session that writes nothing to learning
  history. This scenario first seeds a fresh "misunderstood" log by answering a
  Standard card wrong (the mock grader marks any answer starting with "wrong"
  incorrect), then relearns it.
  #
  # Routes exercised: /quiz/relearn (start), /quiz/relearn/session (the loop),
  # and /quiz/relearn/complete (summary). The pool order is server-side map
  # order, so the scenario asserts on counts and routes rather than a specific
  # first card.

  Scenario: Relearn a word missed moments ago in a Standard quiz
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the card "break the ice"
    When I type the answer "wrong on purpose"
    And I submit my answer
    And I continue to the next card

    Then I see the card "lose one's temper"
    When I type the answer "to become angry"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page

    When I open the Relearn Quiz
    Then I see words to relearn
    When I start the relearn session
    Then I see a relearn card
    When I clear every remaining relearn card
    Then I should be on the Relearn Complete page

  # A grammar miss must resurface in Relearn using the SAME inline-correction
  # card the live Grammar quiz shows (the whole entry, the mistaken span
  # struck through, an inline box for the fix) — not a plain-text word/meaning
  # fallback. See docs/content/proposals/relearn-quiz and
  # .claude/rules/learning-history-invariants.md (grammar's storage key, the
  # correction's stable id, is unchanged end to end).
  Scenario: Relearn a grammar correction missed moments ago
    Given I open the Grammar quiz
    And I select the "Practice Journal" notebook
    And I start the grammar quiz
    Then I see the grammar correction input for "the John"

    When I correct "the John" with "John"
    And I correct "suggested to go" with "wrong on purpose"
    Then the graded post marks "the John" as correct
    And the graded post marks "suggested to go" as incorrect

    When I finish the grammar post
    Then I should be on the Grammar Complete page

    When I open the Relearn Quiz
    Then I see words to relearn
    When I start the relearn session
    # The pool may also hold leftover words from earlier scenarios in this
    # window, so this finds the grammar card wherever it falls in the queue
    # (skipping past any other-format card first), fixes it correctly, and
    # then drains whatever else remains.
    When I find and fix the grammar relearn card for "suggested to go" with "suggested going"
    And I clear every remaining relearn card
    Then I should be on the Relearn Complete page
