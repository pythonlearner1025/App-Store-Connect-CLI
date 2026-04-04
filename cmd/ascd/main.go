package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/cmd"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/helper"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func versionInfoString() string {
	return fmt.Sprintf("%s (commit: %s, date: %s)", version, commit, date)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == analyticsUploadArg {
		os.Exit(runAnalyticsUploadSubprocess())
	}

	if len(os.Args) > 1 && os.Args[1] == helper.CLISubprocessModeArg {
		version := versionInfoString()
		if override := strings.TrimSpace(os.Getenv(helper.CLIVersionEnvVar)); override != "" {
			version = override
		}
		startedAt := time.Now()
		exitCode := cmd.Run(os.Args[2:], version)
		enqueueAgentDirectAnalytics(
			cmd.CommandName(os.Args[2:], version),
			exitCode == cmd.ExitSuccess,
			startedAt,
		)
		os.Exit(exitCode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	server := helper.NewServer(os.Stdin, os.Stdout, versionInfoString())
	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ascd: %v\n", err)
		os.Exit(1)
	}
}
