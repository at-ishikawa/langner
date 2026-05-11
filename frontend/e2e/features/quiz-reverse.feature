Feature: Reverse vocabulary quiz
  In Reverse mode, the user is shown a meaning and types the matching word.

  Scenario: Finish a Reverse quiz across two cards
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz
    Then I see a meaning prompt

    When I type the answer "break the ice"
    And I submit my answer
    And I continue to the next card
    Then I see a meaning prompt

    When I type the answer "lose one's temper"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page
    And the summary shows 2 total words
