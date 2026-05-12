Feature: Standard vocabulary quiz
  The user picks Standard mode, answers each card, and reaches the summary.

  # The backend shuffles cards, so we don't assert on which expression shows
  # up on each turn — we just assert the flow advances through both cards and
  # lands on the summary page.

  Scenario: Finish a Standard quiz across two cards
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card
    When I type the answer "to become angry"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page
    And the summary shows 2 total words

  # Pressing "Don't Know" on AnswerInput records "I don't know" for the card
  # and advances; the mock grader treats that as incorrect.
  Scenario: Skip a Standard quiz card with "Don't Know"
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    When I skip the card
    And I continue to the next card
    When I skip the card
    And I continue to the next card
    Then I should be on the Quiz Complete page
    And the summary shows 2 total words

  # On the BatchFeedback view (rendered after the final card), each result
  # exposes a "Mark as Correct/Incorrect" toggle. Overriding flips the answer
  # before navigating to the summary.
  Scenario: Override an answer on the BatchFeedback view
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card
    When I type the answer "to become angry"
    And I submit my answer
    And I override the first answer
    And I continue to the next card
    Then I should be on the Quiz Complete page

  # The Quiz Complete summary exposes the same override toggle plus an
  # "Exclude" outline button, and a "Back to Start" button to begin a new
  # quiz.
  Scenario: Override, exclude, then restart from the summary
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    When I type the answer "a way to start a conversation"
    And I submit my answer
    And I continue to the next card
    When I type the answer "to become angry"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page

    When I override the first answer
    And I exclude the first answer
    And I follow the "Back to Start" button
    Then I should be on the Quiz page

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

    When I type the answer "an answer to grade"
    And I submit my answer
    And I retry grading
    Then I see the heading "Feedback"
