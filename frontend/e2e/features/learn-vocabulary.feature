Feature: Browse a vocabulary notebook
  The user opens a vocabulary notebook from the Learn hub and reads its words.

  Scenario: Open the Idioms flashcard notebook from the Learn hub
    Given I am on the Learn page
    When I open the "Idioms" notebook
    Then I see the heading "Idioms"

  Scenario: Expand a flashcard word card to see its meaning
    Given I am on the "Idioms" notebook detail page
    When I open the "Common Idioms" story
    Then I see the word "break the ice"
    And I see the word "lose one's temper"

    When I open the "break the ice" word card
    Then I see the example "She told a joke to break the ice."

  # /learn/[id] only renders for notebooks whose stories have prose or
  # dialogue. The "Short Tales" fixture is a story notebook so the Read
  # button on /notebooks/short-tales links to /learn/short-tales.
  Scenario: Open the Short Tales story reader
    Given I am on the "Short Tales" learn content page
    Then I should be on the Learn content page
    And I see the heading "Short Tales"
    And I see the example "Talking about the rain is one way to"

  # GetLatestStatus uses learned_logs[0] (logs are prepended), which in our
  # fixture is the freeform "understood" entry — both cards therefore show
  # learningStatus=understood. Filtering by Understood keeps both visible.
  Scenario: Filter Idioms words by Understood learning status
    Given I am on the "Idioms" notebook detail page
    When I open the "Common Idioms" story
    And I filter by the "Understood" status
    Then I see the word "break the ice"
    And I see the word "lose one's temper"

  # The per-quiz-type skip checkboxes route Skip/Resume RPCs from the notebook
  # detail page. Toggle once, assert, then untoggle so the word stays
  # available to later quiz scenarios.
  Scenario: Toggle a per-quiz-type skip on a word card
    Given I am on the "Idioms" notebook detail page
    When I open the "Common Idioms" story
    And I open the "break the ice" word card
    And I check the "Standard" skip for "break the ice"
    Then the "Standard" skip for "break the ice" is checked
    When I uncheck the "Standard" skip for "break the ice"
    Then the "Standard" skip for "break the ice" is not checked
