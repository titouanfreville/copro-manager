package steps

import "github.com/cucumber/godog"

// RegisterHomeSteps registers all step definitions for the home feature.
func RegisterHomeSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Step(`^I send a GET request to "([^"]*)"$`, tc.SendGETRequest)
	ctx.Step(`^the response status should be (\d+)$`, tc.AssertResponseStatus)
	ctx.Step(`^the response body should contain "([^"]*)"$`, tc.AssertBodyContains)
}
