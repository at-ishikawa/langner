Feature: Freeform vocabulary quiz
  In Freeform mode, the user types both a word and its meaning each turn.
  Mock grader marks the answer correct unless the meaning starts with
  "wrong".

  # Exercise the FeedbackActions toolbar — the freeform variants don't use
  # BatchFeedback. FeedbackActions hides the Mark and Exclude buttons while
  # isOverridden is true, so the actionable order is Mark → Undo → Exclude.
  # Mark as Correct and Mark as Incorrect are the same button with a flipped
  # label depending on the card's current state, so this single scenario
  # exercises both override directions implicitly via Undo.
  Scenario: All per-card actions on the Freeform feedback view
    Given I am on the Quiz page
    When I choose the "Freeform" quiz mode
    And I start the quiz
    Then I see a freeform answer form

    When I type the word "break the ice"
    And I type the meaning "a way to start a conversation"
    And I submit my answer
    # FeedbackActions now visible — answer was correct.
    And I mark "break the ice" as incorrect
    And I undo the override for "break the ice"
    And I exclude "break the ice"
    And I finish the quiz

    Then I should be on the Quiz Complete page
    And the summary shows 1 total words
    And the summary shows 0 correct answers
    And the summary shows 0 incorrect answers

  Scenario: Submit one freeform answer and finish
    Given I am on the Quiz page
    When I choose the "Freeform" quiz mode
    And I start the quiz
    Then I see a freeform answer form

    When I type the word "break the ice"
    And I type the meaning "a way to start a conversation"
    And I submit my answer
    And I finish the quiz
    Then I should be on the Quiz Complete page
    And the summary shows 1 correct answers
    And the summary shows 0 incorrect answers
