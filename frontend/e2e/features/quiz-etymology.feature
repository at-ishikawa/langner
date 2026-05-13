Feature: Etymology quiz modes
  The user runs each of the three etymology quiz modes to completion. With
  `quiz.disable_shuffle: true` the origins appear in fixture order — "graph"
  first, then "tele".

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
    And the summary shows 1 correct answers
    And the summary shows 0 incorrect answers

  # Etymology Standard heading is the origin name; the user types its meaning.
  Scenario: Finish an etymology quiz in Standard mode with both cards correct
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz

    Then I see the heading "graph"
    When I type the answer "writing"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "tele"
    When I type the answer "distant"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 2 correct answers
    And the summary shows 0 incorrect answers

  # Exercise every per-card action on the Etymology Standard BatchFeedback
  # view: Mark as Correct, Mark as Incorrect, Undo override, Exclude.
  Scenario: All per-card actions on the Etymology Standard BatchFeedback view
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Standard" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz

    Then I see the heading "graph"
    When I type the answer "wrong on purpose"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "tele"
    When I type the answer "distant"
    And I submit my answer

    # BatchFeedback now visible: "graph"=incorrect, "tele"=correct.
    When I mark "graph" as correct
    And I undo the override for "graph"
    And I mark "tele" as incorrect
    And I exclude "graph"
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 0 correct answers
    And the summary shows 1 incorrect answers

  # Etymology Reverse heading is the origin's meaning; the user types the
  # origin name.
  Scenario: Finish an etymology quiz in Reverse mode with one wrong answer
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz

    Then I see the heading "writing"
    When I type the answer "graph"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "distant"
    When I type the answer "wrong"
    And I submit my answer
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 1 correct answers
    And the summary shows 1 incorrect answers

  # Exercise every per-card action on the Etymology Reverse BatchFeedback.
  Scenario: All per-card actions on the Etymology Reverse BatchFeedback view
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Reverse" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz

    Then I see the heading "writing"
    When I type the answer "wrong"
    And I submit my answer
    And I continue to the next card

    Then I see the heading "distant"
    When I type the answer "tele"
    And I submit my answer

    # BatchFeedback now visible: "graph"=incorrect, "tele"=correct.
    When I mark "graph" as correct
    And I undo the override for "graph"
    And I mark "tele" as incorrect
    And I exclude "graph"
    And I continue to the next card

    Then I should be on the Quiz Complete page
    And the summary shows 0 correct answers
    And the summary shows 1 incorrect answers

  # Exercise the FeedbackActions toolbar on Etymology Freeform.
  Scenario: All per-card actions on the Etymology Freeform feedback view
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Freeform" quiz mode
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type the origin "graph"
    And I type the meaning "writing"
    And I submit my answer
    # FeedbackActions now visible — answer was correct.
    And I mark "graph" as incorrect
    And I undo the override for "graph"
    And I mark "graph" as incorrect
    And I exclude "graph"
    And I finish the quiz

    Then I should be on the Quiz Complete page
    And the summary shows 0 correct answers
    And the summary shows 0 incorrect answers
