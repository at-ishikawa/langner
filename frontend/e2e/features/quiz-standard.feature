Feature: Standard vocabulary quiz
  The user picks Standard mode, sees each card's expression, types the
  meaning, and reaches the summary. With `quiz.disable_shuffle: true` the
  cards appear in fixture order — "break the ice" first, then "lose one's
  temper". The mock grader marks every non-empty answer correct except for
  the literal "I don't know" and any answer starting with "wrong".

  Scenario: Finish a Standard quiz with both cards correct
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the card "break the ice"
    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card

    Then I see the card "lose one's temper"
    When I type the answer "to become angry"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 2 total words
    And the summary shows 2 correct answers
    And the summary shows 0 incorrect answers

  # "wrong …" is the sentinel the mock grader uses to mark an answer
  # incorrect; any other non-empty answer is graded as correct.
  Scenario: Finish a Standard quiz with one correct and one wrong answer
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
    And the summary shows 2 total words
    And the summary shows 1 correct answers
    And the summary shows 1 incorrect answers

  # Pressing "Don't Know" on AnswerInput records "I don't know" for the card.
  # The mock grader treats that as incorrect, so both cards end up wrong.
  Scenario: Skip both cards with "Don't Know" and see two incorrect answers
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the card "break the ice"
    When I skip the card
    And I continue to the next card

    Then I see the card "lose one's temper"
    When I skip the card
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 2 total words
    And the summary shows 0 correct answers
    And the summary shows 2 incorrect answers

  # On the BatchFeedback view (rendered after the final card), each result
  # exposes a "Mark as Correct/Incorrect" toggle. Overriding the first card
  # flips one correct answer to incorrect before reaching the summary.
  Scenario: Override the first answer on BatchFeedback flips one correct to incorrect
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the card "break the ice"
    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card

    Then I see the card "lose one's temper"
    When I type the answer "to become angry"
    And I submit my answer
    And I override the first answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 1 correct answers
    And the summary shows 1 incorrect answers

  # The Quiz Complete summary exposes the same override toggle plus an
  # "Exclude" outline button, and a "Back to Start" button to begin a new
  # quiz. Excluded answers are dropped from both counts.
  Scenario: Override, exclude, then restart from the summary
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the card "break the ice"
    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card

    Then I see the card "lose one's temper"
    When I type the answer "to become angry"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page

    When I override the first answer
    And I exclude the first answer
    # Resume the excluded card so later scenarios still see it.
    And I resume the first answer
    And I follow the "Back to Start" button
    Then I should be on the Quiz page

  # Exercise every per-card action available on the BatchFeedback view:
  # Mark as Correct, Mark as Incorrect, Undo override, and Exclude. Starts
  # with one wrong and one correct so both "Mark as Correct" and "Mark as
  # Incorrect" buttons are visible at the same time.
  Scenario: All per-card actions on the Standard BatchFeedback view
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

    # BatchFeedback now visible: "break the ice"=incorrect, "lose one's temper"=correct.
    When I mark "break the ice" as correct
    And I undo the override for "break the ice"
    And I mark "lose one's temper" as incorrect
    And I exclude "break the ice"
    # Resume the excluded card so later scenarios still see it.
    And I resume "break the ice"
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 2 total words
    And the summary shows 1 incorrect answers

  # page.route intercepts the first BatchSubmitAnswers RPC and 500s it, so
  # the standard quiz page surfaces its "Retry grading" outline button.
  # Clicking retry replays the batch successfully (route lets the second call
  # through). feedbackInterval=1 makes flushBatch fire on the very first
  # submission so the test doesn't depend on how many cards the notebook
  # currently has — accumulated state from earlier scenarios changes that.
  Scenario: Retry grading after a transient failure
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I set the feedback interval to 1
    And the next answer submission will fail once
    And I start the quiz

    Then I see the card "break the ice"
    When I type the answer "an answer to grade"
    And I submit my answer
    And I retry grading
    Then I see the heading "Feedback"
