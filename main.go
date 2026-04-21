package main

import (
	"os"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

var groupName = os.Getenv("GROUP_NAME")

func main() {
	if groupName == "" {
		panic("GROUP_NAME environment variable must be set")
	}
	cmd.RunWebhookServer(groupName, &namedotcomSolver{})
}
