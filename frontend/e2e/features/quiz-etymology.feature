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

  # There is no skip control: submitting a card with every word left blank
  # records one wrong result, so the flow can be exercised without depending on
  # the derived word family's meanings.
  Scenario: Finish the Etymology Origin quiz by submitting blank origins
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I select the "Word Stems" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I submit my answer
    And I continue to the next card
    And I submit my answer
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
    And I submit my answer
    And I continue to the next card
    When I type "wrong" for the family word "synopsis"
    And I submit my answer
    And I continue to the next card
    And I submit my answer

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

    When I submit my answer
    And I continue to the next card
    And I submit my answer
    And I continue to the next card
    When I type "a brief summary or overview" for the family word "synopsis"
    And I submit my answer
    And I continue to the next card
    And I submit my answer

    Then I do not see the concept-cluster block

  # Leaving one derived family word blank must not affect grading for a sibling
  # word answered on the same card: the answered word is graded on its own
  # merits and the blank word is graded incorrect (a normal miss), never a
  # distinct "skipped" state and never marking the origin as excluded from
  # future quizzes (that stays a distinct, explicit action).
  #
  # feedbackInterval=1 flushes the batch on the very first submission so this
  # scenario grades and asserts the "graph" card (photograph + autograph) on
  # its own, without walking the rest of the deck. The number of *other* due
  # origins varies with the learning history accumulated by earlier scenarios
  # (a correctly answered origin drops out of the due set, a missed one stays),
  # so submitting once and asserting immediately keeps this scenario robust to
  # that ordering instead of assuming a fixed card count.
  Scenario: Leaving one family word blank grades it incorrect without excluding the origin
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "Etymology Origin" quiz mode
    And I include unstudied words
    And I set the feedback interval to 1
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type "a picture made with light" for the family word "photograph"
    And I submit my answer

    Then I see the family word "photograph" marked correct
    And I see the family word "autograph" marked incorrect
    And I do not see the origin marked excluded
