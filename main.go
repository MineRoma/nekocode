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

func main() {
	continueLatest := flag.Bool("continue", false, "continue the latest session for this project")
	resume := flag.String("resume", "", "resume a session by ID")
	yolo := flag.Bool("yolo", false, "auto-approve allowed actions; hard safety blocks remain active")
	backgroundJobs := flag.Int("background-jobs", background.MaxAgents, "maximum concurrent background agents (1-25)")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *showVersion {
		fmt.Println("neko " + version)
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
