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
