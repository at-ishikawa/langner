Feature: Browse an etymology notebook
  The user opens an etymology notebook, sees origins, and views the mindmap.

  Scenario: Open the Word Roots etymology notebook
    Given I am on the Learn page
    When I switch to the "Etymology" tab
    And I open the "Word Roots" notebook
    Then I see the heading "Word Roots"
    And I see the origin "graph"

  Scenario: Open the mindmap for an origin
    Given I am on the "Word Roots" etymology notebook page
    When I open the mindmap for "graph"
    Then I should be on the mindmap page
    And I see the node "graphology"
