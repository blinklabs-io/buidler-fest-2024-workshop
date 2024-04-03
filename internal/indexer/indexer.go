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

package indexer

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/config"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/wallet"

	"github.com/SundaeSwap-finance/kugo"
	"github.com/blinklabs-io/snek/event"
	filter_chainsync "github.com/blinklabs-io/snek/filter/chainsync"
	filter_event "github.com/blinklabs-io/snek/filter/event"
	input_chainsync "github.com/blinklabs-io/snek/input/chainsync"
	output_embedded "github.com/blinklabs-io/snek/output/embedded"
	"github.com/blinklabs-io/snek/pipeline"
)

type Indexer struct {
	pipeline *pipeline.Pipeline
}

// Singleton indexer instance
var globalIndexer = &Indexer{}

func (i *Indexer) Start() error {
	cfg := config.GetConfig()
	w := wallet.GetWallet()
	// Create pipeline
	i.pipeline = pipeline.New()
	// Configure pipeline input
	inputOpts := []input_chainsync.ChainSyncOptionFunc{
		input_chainsync.WithIntersectTip(true),
		input_chainsync.WithAutoReconnect(true),
		input_chainsync.WithNetwork(cfg.Network),
	}
	if cfg.Indexer.Address != "" {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithAddress(cfg.Indexer.Address),
		)
	}
	if cfg.Indexer.SocketPath != "" {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithSocketPath(cfg.Indexer.SocketPath),
		)
	}
	input := input_chainsync.New(
		inputOpts...,
	)
	i.pipeline.AddInput(input)
	// Configure pipeline filters
	// We only care about transaction events
	filterEvent := filter_event.New(
		filter_event.WithTypes([]string{"chainsync.transaction"}),
	)
	i.pipeline.AddFilter(filterEvent)
	// We only care about transactions on our wallet address
	filterChainsync := filter_chainsync.New(
		filter_chainsync.WithAddresses(
			[]string{w.PaymentAddress},
		),
	)
	i.pipeline.AddFilter(filterChainsync)
	// Configure pipeline output
	output := output_embedded.New(
		output_embedded.WithCallbackFunc(i.handleEvent),
	)
	i.pipeline.AddOutput(output)
	// Start pipeline
	if err := i.pipeline.Start(); err != nil {
		return fmt.Errorf("failed to start pipeline: %s", err)
	}
	// Start error handler
	go func() {
		err, ok := <-i.pipeline.ErrorChan()
		if ok {
			slog.Error(
				fmt.Sprintf("pipeline failed: %s", err),
			)
			os.Exit(1)
		}
	}()
	return nil
}

func (i *Indexer) handleEvent(evt event.Event) error {
	cfg := config.GetConfig()
	w := wallet.GetWallet()
	eventTx := evt.Payload.(input_chainsync.TransactionEvent)
	eventCtx := evt.Context.(input_chainsync.TransactionContext)
	// Query kupo (if configured) for TX input to get source wallet address
	// NOTE: this assumes only 1 input
	var inputAddr string = "(unknown)"
	if cfg.Kupo.Endpoint != "" {
		for _, txInput := range eventTx.Inputs {
			k := kugo.New(
				kugo.WithEndpoint(cfg.Kupo.Endpoint),
			)
			matches, err := k.Matches(
				context.Background(),
				kugo.Pattern(
					fmt.Sprintf(
						"%d@%s",
						txInput.Index(),
						txInput.Id().String(),
					),
				),
			)
			if err != nil {
				return err
			}
			if len(matches) == 0 {
				slog.Warn(
					fmt.Sprintf(
						"could not lookup TX input ref %d@%s in kupo (wrong network?)",
						txInput.Index(),
						txInput.Id().String(),
					),
				)
				continue
			}
			inputAddr = matches[0].Address
			break
		}
	}
	for _, txOutput := range eventTx.Outputs {
		if txOutput.Address().String() == w.PaymentAddress {
			slog.Info(
				fmt.Sprintf(
					"received TX %s: %s -> %s",
					eventCtx.TransactionHash,
					inputAddr,
					txOutput.Address().String(),
				),
			)
		}
	}
	// TODO: add TX build logic
	return nil
}

// GetIndexer returns the global indexer instance
func GetIndexer() *Indexer {
	return globalIndexer
}
