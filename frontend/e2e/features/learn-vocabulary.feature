Feature: Browse a vocabulary notebook
  The user opens a vocabulary notebook from the Learn hub and reads its words.

  Scenario: Open the Idioms notebook and see its cards
    Given I am on the Learn page
    When I open the "Idioms" notebook
    Then I see the heading "Idioms"
    And I see the word "break the ice"
    And I see the word "lose one's temper"

  Scenario: Expand a word card to see its meaning
    Given I am on the "Idioms" notebook detail page
    When I open the "break the ice" word card
    Then I see the example "She told a joke to break the ice."
