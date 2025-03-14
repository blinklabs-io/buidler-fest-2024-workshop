// Copyright 2024 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/config"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/indexer"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/wallet"
	"github.com/spf13/cobra"
)

const (
	programName = "workshop"
)

func main() {
	cmd := &cobra.Command{
		Use: programName,
		// Throw an error if any args are provided
		Args: cobra.ExactArgs(0),
		Run:  workshopRun,
	}

	if err := cmd.Execute(); err != nil {
		// NOTE: we purposely don't display the error, since cobra will have already displayed it
		os.Exit(1)
	}
}

func workshopRun(cmd *cobra.Command, args []string) {
	// Configure logger
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)
	// Load config
	cfg, err := config.Load()
	if err != nil {
		slog.Error(
			fmt.Sprintf("failed to load config: %s", err),
		)
		os.Exit(1)
	}
	// Setup wallet
	w, err := wallet.Setup()
	if err != nil {
		slog.Error(
			fmt.Sprintf("failed to configure wallet: %s", err),
		)
		os.Exit(1)
	}
	slog.Info(
		"loaded mnemonic for address: " + w.PaymentAddress,
	)
	// Start indexer
	slog.Info(
		"starting indexer on network " + cfg.Network,
	)
	if err := indexer.GetIndexer().Start(); err != nil {
		slog.Error(
			fmt.Sprintf("failed to start indexer: %s", err),
		)
		os.Exit(1)
	}
	// Wait forever
	select {}
}
