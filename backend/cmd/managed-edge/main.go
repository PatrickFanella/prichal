// managed-edge generates a candidate only; it has no reload or deployment authority.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/managededge"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("managed-edge", flag.ContinueOnError)
	inventoryPath := flags.String("inventory", "", "private operator inventory")
	certificates := flags.String("certificates", "", "absolute source directory containing public ingress certificates")
	out := flags.String("out", "", "new candidate file (never an existing live configuration)")
	revision := flags.String("revision", "", "optional canonical publication UUID for immutable trust paths")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *inventoryPath == "" || *certificates == "" || *out == "" || flags.NArg() != 0 {
		return errors.New("usage: managed-edge --inventory FILE --certificates DIRECTORY --out NEW_FILE")
	}
	info, err := os.Lstat(*inventoryPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("inventory must be a private regular file")
	}
	file, err := os.Open(*inventoryPath)
	if err != nil {
		return err
	}
	defer file.Close()
	inventory, err := managededge.Decode(file)
	if err != nil {
		return err
	}
	var candidate string
	if *revision == "" {
		candidate, err = managededge.Render(inventory, *certificates, time.Now())
	} else {
		candidate, err = managededge.RenderRevision(inventory, *certificates, time.Now(), *revision)
	}
	if err != nil {
		return err
	}
	return writeCandidate(*out, candidate)
}

func writeCandidate(path, candidate string) error {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("candidate output must be a new file")
	}
	defer output.Close()
	if _, err = output.WriteString(candidate); err != nil {
		return err
	}
	return output.Sync()
}
