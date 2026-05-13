Feature: Freeform vocabulary quiz
  In Freeform mode, the user types both a word and its meaning each turn.
  Mock grader marks the answer correct unless the meaning starts with
  "wrong".

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
