Feature: Login page
  The /login page hosts the Google sign-in entry point. It is the one route the
  auth guard never redirects away from, so it renders whether or not a session
  cookie is present.

  Scenario: The login page offers Google sign-in
    Given I am on the login page
    Then I see the "Sign in with Google" button
