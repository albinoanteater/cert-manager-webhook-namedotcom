//go:build conformance

package main

import (
	"os"
	"testing"

	acmetest "github.com/cert-manager/cert-manager/test/acme"
)

var zone = os.Getenv("TEST_ZONE_NAME")

func TestRunsSuite(t *testing.T) {
	if zone == "" {
		t.Skip("TEST_ZONE_NAME not set; skipping conformance tests")
	}
	solver := &namedotcomSolver{}
	fixture := acmetest.NewFixture(solver,
		acmetest.SetResolvedZone(zone),
		acmetest.SetManifestPath("testdata/namedotcom-solver"),
		acmetest.SetDNSServer("8.8.8.8:53"),
		acmetest.SetUseAuthoritative(true),
	)
	fixture.RunConformance(t)
}
