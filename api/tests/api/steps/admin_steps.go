package steps

import "github.com/cucumber/godog"

// RegisterAdminSteps registers step definitions used by admin-related features.
func RegisterAdminSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Step(`^I set the header "([^"]*)" to "([^"]*)"$`, tc.SetHeader)
	ctx.Step(`^I send a POST request to "([^"]*)" with body:$`, tc.SendPOSTRequest)
}
