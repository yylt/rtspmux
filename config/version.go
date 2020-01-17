package config

import (
	"fmt"
)

var (
	// Version is the semantic version (added at compile time)
	Version string = "1.0"

	Dirty   string
	Branch  string
	// Revision is the git commit id (added at compile time)
	Revision string
)


func printVersion() {
	fmt.Println("version:",Version)
	fmt.Println("dirtry:",Dirty)
	fmt.Println("reversion:",Revision)
	fmt.Println("branch:",Branch)
}
