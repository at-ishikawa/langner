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
