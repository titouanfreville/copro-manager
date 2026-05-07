package api

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"

	"github.com/titouanfreville/copro-manager/api/tests/api/steps"
)

var opts = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
	Paths:  []string{"features"},
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options:             &opts,
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero exit status from godog")
	}
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	tc := steps.NewTestContext()
	steps.RegisterHomeSteps(ctx, tc)
	steps.RegisterAdminSteps(ctx, tc)
}
