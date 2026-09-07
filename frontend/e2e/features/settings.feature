Feature: Settings page
  The /settings page lets the signed-in user register their own LLM provider
  API key, used to grade quizzes. It renders authenticated (the injected e2e
  session cookie), showing the API-key form.

  Scenario: The settings page shows the LLM API key form
    Given I am on the settings page
    Then I see the heading "Settings"
    And I see the heading "LLM API key"
