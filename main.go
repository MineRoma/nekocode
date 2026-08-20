// Neko is a terminal coding agent.
// Copyright (C) 2026 M1neRoma
//
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version. It is distributed WITHOUT ANY WARRANTY; see the LICENSE file or
// <https://www.gnu.org/licenses/> for details.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/m1neroma/neko/internal/app"
	"github.com/m1neroma/neko/internal/background"
)

const version = "0.4.2"

// licenseNotice is the Appropriate Legal Notice the GPL asks interactive
// programs to make available.
const licenseNotice = `Copyright (C) 2026 M1neRoma
License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>
This is free software: you are free to change and redistribute it.
There is NO WARRANTY, to the extent permitted by law.`

func main() {
	continueLatest := flag.Bool("continue", false, "continue the latest session for this project")
	resume := flag.String("resume", "", "resume a session by ID")
	yolo := flag.Bool("yolo", false, "auto-approve allowed actions; hard safety blocks remain active")
	backgroundJobs := flag.Int("background-jobs", background.MaxAgents, "maximum concurrent background agents (1-25)")
	showVersion := flag.Bool("version", false, "print the version and license")
	flag.Parse()
	if *showVersion {
		fmt.Println("neko " + version)
		fmt.Println(licenseNotice)
		return
	}
	if flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "neko: unexpected arguments; start the CLI without a prompt and type /help")
		os.Exit(2)
	}
	if *backgroundJobs < 1 || *backgroundJobs > background.MaxAgents {
		fmt.Fprintf(os.Stderr, "neko: --background-jobs must be between 1 and %d\n", background.MaxAgents)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	options := app.Options{Continue: *continueLatest, Resume: *resume, YOLO: *yolo, BackgroundJobs: *backgroundJobs}
	if err := app.Run(ctx, options); err != nil {
		fmt.Fprintln(os.Stderr, "neko:", err)
		os.Exit(1)
	}
}
