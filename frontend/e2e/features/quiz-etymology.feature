Feature: Etymology Origin quiz
  The user runs the single Etymology Origin quiz mode. Each screen shows one
  word origin (with its principal parts, language and type) together with the
  full family of words that derive from it, and asks the user to type each
  derived word's meaning. The origin is graded and tracked as one result.

  Scenario: Start the Etymology Origin quiz and see an origin prompt
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

  # "Don't Know" skips the whole origin screen (recorded as one wrong result)
  # so the flow can be exercised without depending on the derived word family.
  Scenario: Finish the Etymology Origin quiz by skipping each origin
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Stems" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I skip the card
    And I continue to the next card
    And I skip the card
    And I continue to the next card

    Then I should be on the Quiz Complete page

  # Bug-2 regression: the feedback screen lets the learner override each
  # derived family word's correctness independently of the origin's other
  # words, without disturbing the origin's own aggregate record.
  Scenario: Mark an individual family word correct without affecting its sibling
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type "wrong" for the family word "photograph"
    And I type "wrong" for the family word "autograph"
    And I submit my answer
    And I continue to the next card
    And I skip the card
    And I continue to the next card
    When I type "wrong" for the family word "synopsis"
    And I submit my answer
    And I continue to the next card
    And I skip the card

    Then I see the family word "photograph" marked incorrect
    And I see the family word "autograph" marked incorrect

    When I mark the family word "autograph" as correct
    Then I see the family word "autograph" marked correct
    And I see the family word "photograph" marked incorrect

  # Bug-1 regression: the old standard-mode etymology quiz's "concept cluster"
  # scaffold (a block reading "Fill in the blank to complete this concept")
  # must never render on the Etymology Origin feedback screen, even for an
  # origin (opsis) that belongs to a concept group.
  Scenario: The feedback screen never shows the leftover concept-cluster block
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I skip the card
    And I continue to the next card
    And I skip the card
    And I continue to the next card
    When I type "a brief summary or overview" for the family word "synopsis"
    And I submit my answer
    And I continue to the next card
    And I skip the card

    Then I do not see the concept-cluster block

  # Bug-1 regression: tapping "Don't Know" for one derived family word must
  # not affect grading for a sibling word answered on the same card, and
  # must never mark the origin as excluded from future quizzes — excluding
  # an origin is a distinct, explicit action that a per-word skip never
  # triggers as a side effect.
  Scenario: Skip one family word while answering its sibling, without excluding the origin
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type "a picture made with light" for the family word "photograph"
    And I skip the family word "autograph"
    And I submit my answer
    And I continue to the next card
    And I skip the card
    And I continue to the next card
    And I skip the card
    And I continue to the next card
    And I skip the card

    Then I see the family word "photograph" marked correct
    And I see the family word "autograph" marked as skipped
    And I do not see the origin marked excluded
