package main

import (
	"errors"
	"os"

	"github.com/junaid18183/sanduk/cli"
	"github.com/junaid18183/sanduk/vault"
)

func main() {
	if err := cli.Execute(); err != nil {
		var exit *vault.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.Code)
		}
		os.Exit(1)
	}
}
