Feature: Admin endpoints
  As the copro maintainer
  I want admin endpoints gated behind the global AdminKey
  So that nobody else can mint foyer accounts

  Scenario: Reject admin call with no AdminKey
    When I send a POST request to "/admin/foyers" with body:
      """
      {"floor":"rdc","name":"Foyer RDC","email":"rdc@example.com"}
      """
    Then the response status should be 401
    And the response body should contain "UNAUTHORIZED"

  Scenario: Reject admin call with wrong AdminKey
    Given I set the header "Authorization" to "AdminKey nope-this-is-not-the-key"
    When I send a POST request to "/admin/foyers" with body:
      """
      {"floor":"rdc","name":"Foyer RDC","email":"rdc@example.com"}
      """
    Then the response status should be 401
    And the response body should contain "UNAUTHORIZED"
