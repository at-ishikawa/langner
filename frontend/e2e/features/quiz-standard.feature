Feature: Standard vocabulary quiz
  The user picks Standard mode, answers each card, and reaches the summary.

  Scenario: Finish a Standard quiz across two cards
    Given I am on the Quiz page
    When I choose the "Standard" quiz mode
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
