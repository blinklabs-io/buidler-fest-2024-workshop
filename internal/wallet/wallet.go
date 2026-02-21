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

package wallet

import (
	"errors"
	"log/slog"
	"os"

	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/config"
	"github.com/blinklabs-io/bursa"
)

var globalWallet *bursa.Wallet

func Setup() (*bursa.Wallet, error) {
	// Return existing wallet instance if available
	if globalWallet != nil {
		return globalWallet, nil
	}
	// Setup wallet
	cfg := config.GetConfig()
	mnemonic := cfg.Wallet.Mnemonic
	if mnemonic == "" {
		// Read seed.txt if it exists
		if data, err := os.ReadFile("seed.txt"); err == nil {
			slog.Info("read mnemonic from seed.txt")
			mnemonic = string(data)
		} else if errors.Is(err, os.ErrNotExist) {
			mnemonic, err = bursa.NewMnemonic()
			if err != nil {
				return nil, err
			}
			// Write seed.txt
			// WARNING: this will clobber existing files
			f, err := os.Create("seed.txt")
			if err != nil {
				return nil, err
			}
			l, err := f.WriteString(mnemonic)
			slog.Debug("wrote bytes to seed.txt", "bytes", l) // #nosec G706
			if err != nil {
				f.Close()
				return nil, err
			}
			err = f.Close()
			if err != nil {
				return nil, err
			}
			slog.Info("wrote generated mnemonic to seed.txt")
		} else {
			return nil, err
		}
	}
	wallet, err := bursa.NewWallet(
		mnemonic,
		cfg.Network,
		"", 0, 0, 0, 0,
	)
	if err != nil {
		return nil, err
	}
	globalWallet = wallet
	return globalWallet, nil
}

func GetWallet() *bursa.Wallet {
	return globalWallet
}
