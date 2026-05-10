Feature: Home navigation
  From the home page, the user can reach the Learn hub or the Quiz hub.

  Scenario: Open the Learn hub from home
    Given I am on the home page
    When I follow the "Learn" link
    Then I should be on the Learn page
    And I see the notebook "Idioms"

  Scenario: Open the Quiz hub from home
    Given I am on the home page
    When I follow the "Quiz" link
    Then I should be on the Quiz page
    And I see the quiz mode "Standard"
