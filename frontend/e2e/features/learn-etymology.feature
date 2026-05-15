Feature: Browse an etymology notebook
  The user opens an etymology notebook, sees origins, and views the mindmap.

  Scenario: Open the Word Roots etymology notebook
    Given I am on the Learn page
    When I switch to the "Etymology" tab
    And I open the "Word Roots" notebook
    # The etymology detail page header is literally "Etymology" — the
    # notebook name is not shown in a heading; origins are listed as cards.
    Then I see the heading "Etymology"
    And I see the origin "graph"
    And I see the origin "tele"

  Scenario: Open the mindmap for an origin
    Given I am on the "Word Roots" etymology notebook page
    When I open the mindmap for "graph"
    Then I should be on the mindmap page
    # The focused origin renders as a ReactFlow node whose label is
    # "<origin>\n(<meaning>)" — both "graph" and "writing" appear.
    And I see the node "graph"
