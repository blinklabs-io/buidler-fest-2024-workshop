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

package txbuilder

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/SundaeSwap-finance/kugo"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/config"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/txsubmit"
	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/wallet"

	"github.com/Salvionied/apollo"
	"github.com/Salvionied/apollo/constants"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/Asset"
	"github.com/Salvionied/apollo/serialization/AssetName"
	"github.com/Salvionied/apollo/serialization/Key"
	"github.com/Salvionied/apollo/serialization/MultiAsset"
	"github.com/Salvionied/apollo/serialization/Policy"
	"github.com/Salvionied/apollo/serialization/Transaction"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/TransactionOutput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/serialization/Value"
	"github.com/Salvionied/apollo/txBuilding/Backend/BlockFrostChainContext"
	"github.com/blinklabs-io/adder/event"
	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
)

func HandleEvent(evt event.Event) error {
	cfg := config.GetConfig()
	w := wallet.GetWallet()
	eventTx := evt.Payload.(input_chainsync.TransactionEvent)
	eventCtx := evt.Context.(input_chainsync.TransactionContext)
	// Determine source address from TX inputs
	// NOTE: this assumes only 1 input
	var inputAddr string = "(unknown)"
	for _, txInput := range eventTx.Inputs {
		utxo, err := getUtxoByRef(
			txInput.Id().String(),
			int(txInput.Index()),
		)
		if err != nil {
			slog.Warn(
				fmt.Sprintf(
					"failed to lookup TX input ref: %s", err,
				),
			)
			continue
		}
		if utxo == nil {
			slog.Warn(
				fmt.Sprintf(
					"could not lookup TX input ref %s#%d in backend (wrong network?)",
					txInput.Id().String(),
					txInput.Index(),
				),
			)
			continue
		}
		if utxo.Output.IsPostAlonzo {
			inputAddr = utxo.Output.PostAlonzo.Address.String()
		} else {
			inputAddr = utxo.Output.PreAlonzo.Address.String()
		}
		break
	}
	// Add up amounts to our address
	var totalOutputAmount uint64
	for _, txOutput := range eventTx.Outputs {
		txOutAddr := txOutput.Address().String()
		if txOutAddr == w.PaymentAddress || txOutAddr == cfg.Reward.RewardAddress {
			slog.Info(
				fmt.Sprintf(
					"received TX %s: %s -> %s (%d lovelace)",
					eventCtx.TransactionHash,
					inputAddr,
					txOutput.Address().String(),
					txOutput.Amount(),
				),
			)
			if txOutAddr == w.PaymentAddress {
				totalOutputAmount += txOutput.Amount()
			}
		}
	}
	// Skip further processing if there's no reward address defined
	if cfg.Reward.RewardAddress == "" {
		slog.Warn("skipping further processing: no reward address defined")
		return nil
	}
	// Skip further processing if transaction doesn't come from the configured source address
	if cfg.Reward.SourceAddress != "" && inputAddr != cfg.Reward.SourceAddress {
		slog.Warn("skipping further processing: source address doesn't match")
		return nil
	}
	// Skip further processing if transaction output amount is below the reward threshold
	if totalOutputAmount < cfg.Reward.MinLovelace {
		slog.Warn("skipping further processing: total output amount is below reward minimum")
		return nil
	}
	// Build reward transaction
	tx, err := BuildRewardTx()
	if err != nil {
		return err
	}
	// Submit TX
	txBytes, err := tx.Bytes()
	if err != nil {
		return err
	}
	if err := txsubmit.SubmitTx(txBytes); err != nil {
		return err
	}
	slog.Info(
		fmt.Sprintf(
			"submitted transaction %x",
			tx.Id().Payload,
		),
	)
	return nil
}

func BuildRewardTx() (*Transaction.Transaction, error) {
	cfg := config.GetConfig()
	w := wallet.GetWallet()
	cc := apollo.NewEmptyBackend()
	apollob := apollo.New(&cc)
	apollob = apollob.
		SetWalletFromBech32(w.PaymentAddress).
		SetWalletAsChangeAddress()

	utxos, err := getUtxosByAddress(w.PaymentAddress)
	if err != nil {
		return nil, err
	}
	apollob = apollob.AddLoadedUTxOs(utxos...)

	apollob = apollob.
		PayToAddressBech32(
			cfg.Reward.RewardAddress, int(cfg.Reward.RewardAmount),
		)
	tx, err := apollob.Complete()
	if err != nil {
		return nil, err
	}
	vKeyBytes, err := hex.DecodeString(w.PaymentVKey.CborHex)
	if err != nil {
		return nil, err
	}
	sKeyBytes, err := hex.DecodeString(w.PaymentExtendedSKey.CborHex)
	if err != nil {
		return nil, err
	}
	// Strip off leading 2 bytes as shortcut for CBOR decoding to unwrap bytes
	vKeyBytes = vKeyBytes[2:]
	sKeyBytes = sKeyBytes[2:]
	// Strip out public key portion of extended private key
	sKeyBytes = append(sKeyBytes[:64], sKeyBytes[96:]...)
	vkey := Key.VerificationKey{Payload: vKeyBytes}
	skey := Key.SigningKey{Payload: sKeyBytes}
	tx, err = tx.SignWithSkey(vkey, skey)
	if err != nil {
		return nil, err
	}
	return tx.GetTx(), nil
}

func getBlockfrostContext() (*BlockFrostChainContext.BlockFrostChainContext, error) {
	cfg := config.GetConfig()
	var ret BlockFrostChainContext.BlockFrostChainContext
	switch cfg.Network {
	case "preprod":
		ret = BlockFrostChainContext.NewBlockfrostChainContext(
			constants.BLOCKFROST_BASE_URL_PREPROD,
			int(constants.PREPROD),
			cfg.TxBuilder.BlockfrostApiKey,
		)
	// TODO: add more networks
	default:
		return nil, fmt.Errorf("unsupported network: %s", cfg.Network)
	}
	return &ret, nil
}

func getKupoClient() (*kugo.Client, error) {
	cfg := config.GetConfig()
	k := kugo.New(
		kugo.WithEndpoint(cfg.TxBuilder.KupoUrl),
	)
	return k, nil
}

func getUtxosByAddress(addr string) ([]UTxO.UTxO, error) {
	cfg := config.GetConfig()
	if cfg.TxBuilder.BlockfrostApiKey != "" {
		bfc, err := getBlockfrostContext()
		if err != nil {
			return nil, err
		}
		serAddr, err := serAddress.DecodeAddress(addr)
		if err != nil {
			return nil, err
		}
		utxos := bfc.Utxos(serAddr)
		return utxos, nil
	} else if cfg.TxBuilder.KupoUrl != "" {
		k, err := getKupoClient()
		if err != nil {
			return nil, err
		}
		matches, err := k.Matches(
			context.Background(),
			kugo.Pattern(addr),
		)
		if err != nil {
			return nil, err
		}
		var ret []UTxO.UTxO
		for _, match := range matches {
			tmpUtxo := kupoMatchToApolloUtxo(match)
			ret = append(ret, tmpUtxo)
		}
		return ret, nil
	}
	return nil, fmt.Errorf("no valid Blockfrost or Kupo/Ogmios config found")
}

func getUtxoByRef(txId string, idx int) (*UTxO.UTxO, error) {
	cfg := config.GetConfig()
	if cfg.TxBuilder.BlockfrostApiKey != "" {
		bfc, err := getBlockfrostContext()
		if err != nil {
			return nil, err
		}
		utxo := bfc.GetUtxoFromRef(txId, idx)
		return utxo, nil
	} else if cfg.TxBuilder.KupoUrl != "" {
		k, err := getKupoClient()
		if err != nil {
			return nil, err
		}
		matches, err := k.Matches(
			context.Background(),
			kugo.Pattern(
				fmt.Sprintf("%d@%s", idx, txId),
			),
		)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			slog.Warn(
				fmt.Sprintf(
					"could not lookup TX input ref %d@%s in kupo (wrong network?)",
					idx,
					txId,
				),
			)
			return nil, nil
		}
		ret := kupoMatchToApolloUtxo(matches[0])
		return &ret, nil
	}
	return nil, fmt.Errorf("no valid Blockfrost or Kupo/Ogmios config found")
}

func kupoMatchToApolloUtxo(match kugo.Match) UTxO.UTxO {
	serAddr, _ := serAddress.DecodeAddress(match.Address)
	txIdBytes, _ := hex.DecodeString(match.TransactionID)
	multiAssets := make(MultiAsset.MultiAsset[int64])
	totalLovelace := uint64(0)
	for policyId, assets := range match.Value {
		for assetId, assetAmount := range assets {
			if policyId == "ada" && assetId == "lovelace" {
				totalLovelace = assetAmount.Uint64()
				continue
			}
			tmpPolicyId := Policy.PolicyId{Value: policyId}
			tmpAssetName := AssetName.NewAssetNameFromString(assetId)
			if _, ok := multiAssets[tmpPolicyId]; !ok {
				multiAssets[tmpPolicyId] = Asset.Asset[int64]{}
			}
			multiAssets[tmpPolicyId][tmpAssetName] = assetAmount.Int64()
		}
	}
	val := Value.SimpleValue(
		int64(totalLovelace),
		multiAssets,
	)
	ret := UTxO.UTxO{
		Input: TransactionInput.TransactionInput{
			TransactionId: txIdBytes,
			Index:         match.OutputIndex,
		},
		Output: TransactionOutput.SimpleTransactionOutput(
			serAddr,
			val,
		),
	}
	return ret
}
