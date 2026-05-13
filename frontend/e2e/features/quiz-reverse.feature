Feature: Reverse vocabulary quiz
  In Reverse mode, the heading is the card's meaning and the user types the
  matching word. With `quiz.disable_shuffle: true` the cards appear in
  fixture order — "break the ice" first, then "lose one's temper".

  Scenario: Finish a Reverse quiz with both cards correct
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the heading "a way to start a conversation in a social setting"
    When I type the answer "break the ice"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "to become angry"
    When I type the answer "lose one's temper"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 2 total words
    And the summary shows 2 correct answers
    And the summary shows 0 incorrect answers

  # The mock ValidateWordForm classifies a non-matching user answer as wrong.
  Scenario: Finish a Reverse quiz with one correct and one wrong answer
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the heading "a way to start a conversation in a social setting"
    When I type the answer "wrong word"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "to become angry"
    When I type the answer "lose one's temper"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 1 correct answers
    And the summary shows 1 incorrect answers

  # Exercise every per-card action on the Reverse BatchFeedback view.
  Scenario: All per-card actions on the Reverse BatchFeedback view
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I start the quiz

    Then I see the heading "a way to start a conversation in a social setting"
    When I type the answer "wrong word"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "to become angry"
    When I type the answer "lose one's temper"
    And I submit my answer

    # BatchFeedback now visible: "break the ice"=incorrect, "lose one's temper"=correct.
    When I mark "break the ice" as correct
    And I undo the override for "break the ice"
    And I mark "lose one's temper" as incorrect
    And I exclude "break the ice"
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 0 correct answers
    And the summary shows 1 incorrect answers

  # Only "lose one's temper" has no example in the fixture, so the missing-
  # context filter narrows the Reverse queue to one card.
  Scenario: Reverse quiz with the "List words missing context" filter
    Given I am on the Quiz page
    When I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Idioms" notebook
    And I enable "List words missing context"
    And I start the quiz

    Then I see the heading "to become angry"
    When I type the answer "lose one's temper"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 1 total words
    And the summary shows 1 correct answers
    And the summary shows 0 incorrect answers
