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
