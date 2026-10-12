Feature: Settings
  The /settings page lets a signed-in user register their own LLM provider and
  API key used for quiz grading. The key is write-only and never displayed back.

  # covers route: /settings
  Scenario: Open the Settings page
    Given I am on the Settings page
    Then I see the heading "Settings"
    And I see the text "No API key configured yet."

  Scenario: Register an API key, then remove it
    Given I am on the Settings page
    Then I see the text "No API key configured yet."
    When I enter the API key "sk-e2e-test-key"
    And I save the API key
    Then I see the text "API key saved."
    And I see the text "key set."
    When I remove the API key
    Then I see the text "No API key configured yet."
