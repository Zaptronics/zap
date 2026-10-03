// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	zap "github.com/Zaptronics/zap/internal/zap"
)

var version = "0.11.1"

func main() {
	zap.Version = version
	passthrough := len(os.Args) > 1 && (strings.EqualFold(os.Args[1], "e") || strings.EqualFold(os.Args[1], "exec"))
	if err := zap.Run(os.Args[1:]); err != nil {
		var childExit *exec.ExitError
		if !passthrough || !errors.As(err, &childExit) {
			zap.PrintError(os.Stderr, err)
			fmt.Fprintln(os.Stderr)
		}
		os.Exit(zap.ExitCode(err))
	}
	if !passthrough {
		fmt.Fprintln(os.Stdout)
	}
}
