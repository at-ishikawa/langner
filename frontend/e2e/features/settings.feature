Feature: Settings
  The /settings page lets a signed-in user register their OWN LLM provider keys
  used for quiz grading. A user can set a key per provider; one is the primary
  and the other is the automatic rate-limit/quota fallback. Keys are write-only
  and never displayed back.

  # covers route: /settings
  Scenario: Open the Settings page
    Given I am on the Settings page
    Then I see the heading "Settings"
    And I see the text "No API key configured yet."

  Scenario: Register two providers, choose the primary, then remove one
    Given I am on the Settings page
    Then I see the text "No API key configured yet."
    When I enter the "openai" API key "sk-openai-e2e"
    And I save the "openai" key
    Then I see the text "Saved."
    When I enter the "gemini" API key "gm-gemini-e2e"
    And I save the "gemini" key
    Then I see the text "Saved."
    When I make "gemini" the primary provider
    Then I see the text "Saved."
    When I remove the "openai" key
    Then I see the text "No key set."
