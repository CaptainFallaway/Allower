package main

import (
	"fmt"
	"os"
)

const appName = "Reverser"

func run() error {
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Printf("error running %s: %v", appName, err)
		os.Exit(1)
	}
}
