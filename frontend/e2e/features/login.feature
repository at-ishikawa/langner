Feature: Login page
  The /login page hosts the Google sign-in entry point. It is the one route the
  auth guard never redirects away from, so it renders whether or not a session
  cookie is present. Sign-in scenarios run the real OAuth round-trip against the
  e2e mock OAuth provider (e2e/mock-oauth-server.mjs) in place of Google.

  Scenario: The login page offers Google sign-in
    Given I am on the login page
    Then I see the "Sign in with Google" button

  # covers route: /auth/callback — the web sign-in lands here with the access
  # token in the URL fragment, then forwards to the app.
  Scenario: Sign in with Google from the login page
    Given I am signed out on the login page
    When I sign in with Google
    Then I am signed in

  # covers route: /auth/device — the CLI device-login approval page that
  # `langner login` prints. Approving runs the Google sign-in, which must return
  # to the same host that set the OAuth state cookie.
  Scenario: Approve a CLI device login in the browser
    Given I started "langner login" in a terminal
    When I open the printed sign-in page and approve the code
    Then I see the heading "Approved"
    And the CLI is logged in
