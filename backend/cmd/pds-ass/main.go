package main

import (
	"context"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/customercli"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(customercli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
