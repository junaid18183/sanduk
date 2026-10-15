package main

import (
	"os"

	"github.com/junaid18183/sanduk/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
