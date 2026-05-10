Feature: Etymology quiz modes
  The user runs each of the three etymology quiz modes to completion.

  Scenario Outline: Finish an etymology quiz in <mode> mode
    Given I am on the Quiz page
    When I switch to the "Etymology" tab
    And I choose the "<mode>" quiz mode
    And I select the "Word Roots" notebook
    And I start the quiz
    Then I see an etymology prompt

    When I type the answer "<answer>"
    And I submit my answer
    And I finish the quiz
    Then I should be on the Quiz Complete page

    Examples:
      | mode     | answer            |
      | Standard | writing           |
      | Reverse  | graph             |
      | Freeform | graph means writing |
