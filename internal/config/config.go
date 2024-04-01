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

package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Indexer IndexerConfig
	Wallet  WalletConfig
	Network string `envconfig:"NETWORK"`
}

type IndexerConfig struct {
	Address    string `envconfig:"INDEXER_TCP_ADDRESS"`
	SocketPath string `envconfig:"INDEXER_SOCKET_PATH"`
}

type WalletConfig struct {
	Mnemonic string `envconfig:"MNEMONIC"`
}

// Singleton config instance with default values
var globalConfig = &Config{
	Network: "preprod",
}

func Load() (*Config, error) {
	// Load any .env file
	if err := godotenv.Load(); err != nil {
		return nil, err
	}
	// Load config values from environment variables
	// We use "dummy" as the app name here to (mostly) prevent picking up env
	// vars that we hadn't explicitly specified in annotations above
	err := envconfig.Process("dummy", globalConfig)
	if err != nil {
		return nil, fmt.Errorf("error processing environment: %s", err)
	}
	return globalConfig, nil
}

// GetConfig returns the global config instance
func GetConfig() *Config {
	return globalConfig
}
