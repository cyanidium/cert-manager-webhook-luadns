package main

import (
	"os"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var (
	zone        = os.Getenv("TEST_ZONE_NAME")
	testDataDir = "../testdata"
)

func TestRunsSuite(t *testing.T) {
	// The manifest path should contain a file named config.json that is a
	// snippet of valid configuration that should be included on the
	// ChallengeRequest passed as part of the test cases.
	fixture := acmetest.NewFixture(&luaDNSProviderSolver{},
		acmetest.SetResolvedZone(zone),
		acmetest.SetAllowAmbientCredentials(false),
		acmetest.SetManifestPath(testDataDir),
		acmetest.SetUseAuthoritative(false),
		acmetest.SetDNSServer("ns1.luadns.net:53"),
		acmetest.SetStrict(true),
	)
	fixture.RunConformance(t)
	fixture.RunBasic(t)
	fixture.RunExtended(t)
}
