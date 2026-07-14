Feature: Quiz Analytics
  Analytics pages must render against the real MySQL-backed AnalyticsService
  used in the e2e backend. These scenarios guard against backend bugs that
  only show up in DB mode (sql_mode quirks, missing JOIN columns, etc.) by
  asserting the pages do NOT render the "Failed to load…" error banner.

  Scenario: Open the Analytics overview from home
    Given I am on the home page
    When I follow the "Analytics" link
    Then I should be on the Analytics page
    And the Analytics page is not in an error state

  # covers route: /analytics/[date]
  Scenario: Open a Day Detail page with seeded wrong words
    Given I open the Analytics Day Detail for "2025-01-02"
    Then I should be on the Analytics Day Detail page
    And the Day Detail page is not in an error state
    And I see the word "break the ice"

  # The Day Detail must include etymology quiz failures even when MySQL is
  # configured. Etymology results are only persisted to YAML today, so a
  # DB-only analytics path silently drops them.
  Scenario: Day Detail surfaces etymology results, not just vocabulary
    Given I open the Analytics Day Detail for "2025-01-02"
    Then I should be on the Analytics Day Detail page
    And the Day Detail page is not in an error state
    And I see the word "scribo"
