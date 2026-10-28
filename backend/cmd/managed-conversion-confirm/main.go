// managed-conversion-confirm records only the designated empty staging-02 conversion.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"time"

	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/config"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/deployment"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/migrate"
	"git.subcult.tv/PatrickFanella/pds-ass/backend/internal/nodeagent"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stagingDeployment = "2395105c-9958-47b5-ae8b-5b0424cbfc4f"
const stagingNode = "60684f10-c6b2-4833-a0b8-fb9f96a7869b"

type conversionProof struct {
	Version        int                               `json:"version"`
	DeploymentID   string                            `json:"deploymentId"`
	NodeID         string                            `json:"nodeId"`
	CreatedAt      time.Time                         `json:"createdAt"`
	SourceAccounts int                               `json:"sourceAccounts"`
	BackupVerified bool                              `json:"backupVerified"`
	Request        deployment.EmptyConversionRequest `json:"request"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	staging, err := config.LoadStagingProvisioning(os.Getenv("PDS_MODE"))
	if err != nil {
		return err
	}
	if !staging {
		return errors.New("conversion confirmation requires explicit production staging provisioning")
	}
	flags := flag.NewFlagSet("managed-conversion-confirm", flag.ContinueOnError)
	proofPath := flags.String("proof", "", "private verified operator proof")
	confirmed := flags.Bool("confirm-staging-02", false, "confirm only the designated empty disposable fixture")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if !*confirmed || *proofPath == "" || flags.NArg() != 0 {
		return errors.New("--proof FILE --confirm-staging-02 required")
	}
	info, err := os.Lstat(*proofPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 8192 {
		return errors.New("bounded private regular conversion proof required")
	}
	file, err := os.Open(*proofPath)
	if err != nil {
		return errors.New("conversion proof unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("conversion proof changed during inspection")
	}
	data, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(data) > 8192 {
		return errors.New("conversion proof unavailable or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var proof conversionProof
	if decoder.Decode(&proof) != nil || decoder.Decode(new(any)) != io.EOF || proof.Version != 1 || proof.DeploymentID != stagingDeployment || proof.NodeID != stagingNode || proof.CreatedAt.IsZero() || proof.CreatedAt.After(time.Now().Add(time.Minute)) || proof.SourceAccounts != 0 || !proof.BackupVerified || proof.Request.OperatorReceiptSHA256 != "" {
		return errors.New("verified empty staging-02 proof required")
	}
	digest := sha256.Sum256(data)
	proof.Request.OperatorReceiptSHA256 = hex.EncodeToString(digest[:])
	if err := proof.Request.Validate(); err != nil {
		return err
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("PDS_DATABASE_URL"))
	if err != nil || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != 35437 || cfg.ConnConfig.Database != "pds_local" || cfg.ConnConfig.User != "pds_staging" {
		return errors.New("dedicated staging-02 management registry required")
	}
	client, err := nodeagent.NewClient(os.Getenv("PDS_WORKER_AGENT_CA_FILE"), os.Getenv("PDS_WORKER_AGENT_CERT_FILE"), os.Getenv("PDS_WORKER_AGENT_KEY_FILE"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := migrate.Apply(ctx, pool); err != nil {
		return err
	}
	if err := deployment.Migrate(ctx, pool); err != nil {
		return err
	}
	store, err := deployment.New(pool)
	if err != nil {
		return err
	}
	store.Staging = true
	actor, err := user.Current()
	if err != nil {
		return errors.New("operator identity unavailable")
	}
	result, err := store.ConfirmEmptyManagedConversion(ctx, actor.Username, proof.DeploymentID, proof.NodeID, proof.Request, client)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
