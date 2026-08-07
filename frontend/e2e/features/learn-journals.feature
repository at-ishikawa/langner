Feature: Browse a journal
  The user opens a journal from the Learn hub Journals tab and reads its
  story content (each entry's grammar mistakes are attached beneath the
  scene they came from on the journal detail page).

  # This scenario navigates to the journal detail page via the UI (Learn ->
  # Journals tab -> the "Practice Journal" card, whose href is /journals/practice),
  # so at runtime it lands on the /journals/[id] route. The concrete path below
  # keeps that click-driven navigation covered by the static coverage gate
  # (mirrors analytics.feature's /history/[date]).
  # covers route: /journals/[id] -> /journals/practice
  Scenario: Open the Practice Journal detail page from the Journals tab
    Given I am on the Learn page
    When I switch to the "Journals" tab
    And I open the "Practice Journal" notebook
    # The journal detail page renders the notebook name as its top heading and
    # each story entry's event as a sub-heading.
    Then I see the heading "Practice Journal"
    And I see the heading "A weekend party"
