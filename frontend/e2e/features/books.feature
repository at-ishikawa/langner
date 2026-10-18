# covers route: /books — the Project Gutenberg catalog: search for a public-domain
# book, import it, and a "Continue reading" list of already-imported shared books.
Feature: Books

  Scenario: Open the Gutenberg book catalog
    Given I am on the Books page
    Then I see the heading "Read a book"
