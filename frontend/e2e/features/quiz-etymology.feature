Feature: Etymology quiz modes
  The user runs each of the three etymology quiz modes to completion.

  # Freeform must run before Standard/Reverse because their submitted answers
  # are persisted to the DB and push the origin's next-review date into the
  # future, which disables Submit on the freeform page ("Not due until …").
  Scenario: Finish an etymology quiz in Freeform mode
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Freeform" quiz mode
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type the origin "graph"
    And I type the meaning "writing"
    And I submit my answer
    And I finish the quiz
    Then I should be on the Quiz Complete page

  Scenario: Finish an etymology quiz in Standard mode
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type the answer "writing"
    And I submit my answer
    And I continue to the next card
    When I type the answer "distant"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page

  Scenario: Finish an etymology quiz in Reverse mode
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type the answer "graph"
    And I submit my answer
    And I continue to the next card
    When I type the answer "tele"
    And I submit my answer
    And I continue to the next card
    Then I should be on the Quiz Complete page
