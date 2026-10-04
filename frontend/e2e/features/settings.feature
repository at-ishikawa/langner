Feature: Settings
  The /settings page lets a signed-in user register their own LLM provider and
  API key used for quiz grading. The key is write-only and never displayed back.

  # covers route: /settings
  Scenario: Open the Settings page
    Given I am on the Settings page
    Then I see the heading "Settings"
