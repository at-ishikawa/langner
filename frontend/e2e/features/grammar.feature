Feature: Grammar Quiz
  The Grammar Quiz turns a journal post into inline grammar practice: the whole
  post is shown, each due mistake is corrected in a textbox beside the wrong
  span, and the batch is graded at once. This scenario fixes one mistake right
  and one wrong (the mock grader marks any answer starting with "wrong"
  incorrect), reviews the graded pills, then on the summary exercises the shared
  Mark-as-Correct / Exclude actions the vocabulary quiz uses.
  #
  # Routes exercised: /quiz/grammar (session) and /quiz/grammar/complete (summary).

  Scenario: Correct a journal post's mistakes, then adjust grades on the summary
    When I open the Grammar quiz
    And I select the "Practice Journal" notebook
    And I start the grammar quiz
    Then I see the grammar correction input for "the John"

    When I correct "the John" with "John"
    And I correct "suggested to go" with "wrong on purpose"
    Then the graded post marks "the John" as correct
    And the graded post marks "suggested to go" as incorrect

    When I finish the grammar post
    Then I should be on the Grammar Complete page

    When I mark "suggested to go" as correct
    Then the summary shows an overridden correction
    When I exclude "the John"
    Then the summary shows an excluded correction
