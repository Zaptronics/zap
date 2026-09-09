// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"os"

	zap "github.com/Zaptronics/zap/internal/zap"
)

var version = "0.6.4"

func main() {
	zap.Version = version
	if err := zap.Run(os.Args[1:]); err != nil {
		zap.PrintError(os.Stderr, err)
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout)
}
