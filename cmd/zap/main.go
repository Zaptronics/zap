// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	zap "github.com/Zaptronics/zap/internal/zap"
)

var version = "dev"

func main() {
	zap.Version = version
	if err := zap.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "zap:", err)
		os.Exit(1)
	}
}
