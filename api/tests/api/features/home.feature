Feature: Home endpoint
  As a consumer of the API
  I want to verify the home and health endpoints work correctly

  Scenario: Get home greeting
    When I send a GET request to "/"
    Then the response status should be 200
    And the response body should contain "Copro manager API"

  Scenario: Health check
    When I send a GET request to "/ping"
    Then the response status should be 200

  Scenario: Get app uptime
    When I send a GET request to "/uptime"
    Then the response status should be 200
    And the response body should contain "uptime"

  Scenario: Not found route
    When I send a GET request to "/nonexistent"
    Then the response status should be 404
    And the response body should contain "NOT_FOUND"
