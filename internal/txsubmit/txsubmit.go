// Copyright 2025 Blink Labs Software
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

package txsubmit

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/blinklabs-io/buidler-fest-2024-workshop/internal/config"
	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

var ntnTxBytes []byte
var ntnTxHash [32]byte
var ntnTxType uint
var ntnSentTx bool
var ntnDoneChan chan any

func SubmitTx(txBytes []byte) error {
	cfg := config.GetConfig()
	if cfg.Submit.Address != "" {
		return submitTxNtN(txBytes)
	} else if cfg.Submit.SocketPath != "" {
		return submitTxNtC(txBytes)
	} else if cfg.Submit.Url != "" {
		return submitTxApi(txBytes)
	} else {
		// Populate address info from indexer network
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			return fmt.Errorf("unknown network: %s", cfg.Network)
		}
		if len(network.BootstrapPeers) == 0 {
			return fmt.Errorf("no upstream configured for %s", cfg.Network)
		}
		peer := network.BootstrapPeers[0]
		cfg.Submit.Address = fmt.Sprintf("%s:%d", peer.Address, peer.Port)
		return submitTxNtN(txBytes)
	}
}

func submitTxNtN(txBytes []byte) error {
	cfg := config.GetConfig()

	// Record TX bytes in global for use in handler functions
	ntnTxBytes = txBytes[:]
	ntnSentTx = false

	// Determine transaction type (era)
	txType, err := ledger.DetermineTransactionType(txBytes)
	if err != nil {
		return fmt.Errorf(
			"could not parse transaction to determine type: %s",
			err,
		)
	}
	tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
	if err != nil {
		return fmt.Errorf("failed to parse transaction CBOR: %s", err)
	}
	txHashHex, err := hex.DecodeString(tx.Hash())
	if err != nil {
		return fmt.Errorf("failed to decode TX hash: %s", err)
	}
	ntnTxHash = [32]byte(txHashHex)
	ntnTxType = txType

	// Create connection
	conn, err := createClientConnection(cfg.Submit.Address)
	if err != nil {
		return err
	}
	errorChan := make(chan error)
	// Capture errors
	go func() {
		err, ok := <-errorChan
		if ok {
			panic(fmt.Errorf("async: %s", err))
		}
	}()
	network, ok := ouroboros.NetworkByName(cfg.Network)
	if !ok {
		return fmt.Errorf("cannot get network: %s", cfg.Network)
	}
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetwork(network),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithRequestTxIdsFunc(handleRequestTxIds),
				txsubmission.WithRequestTxsFunc(handleRequestTxs),
			),
		),
	)
	if err != nil {
		return err
	}

	// Start txSubmission loop
	ntnDoneChan = make(chan any)
	oConn.TxSubmission().Client.Init()
	<-ntnDoneChan
	// Sleep 2s to allow time for TX to enter remote mempool before closing connection
	time.Sleep(2 * time.Second)

	if err := oConn.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %s", err)
	}

	return nil
}

func submitTxNtC(txBytes []byte) error {
	// TODO
	return nil
}

func submitTxApi(txBytes []byte) error {
	cfg := config.GetConfig()
	reqBody := bytes.NewBuffer(txBytes)
	req, err := http.NewRequest(http.MethodPost, cfg.Submit.Url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %s", err)
	}
	req.Header.Add("Content-Type", "application/cbor")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf(
			"failed to send request: %s: %s",
			cfg.Submit.Url,
			err,
		)
	}
	if resp == nil {
		return fmt.Errorf(
			"failed parsing empty response from: %s",
			cfg.Submit.Url,
		)
	}
	// We have to read the entire response body and close it to prevent a memory leak
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 202 {
		return nil
	} else {
		return fmt.Errorf("failed to submit TX to API: %s: %d: %s", cfg.Submit.Url, resp.StatusCode, respBody)
	}
}

func createClientConnection(nodeAddress string) (net.Conn, error) {
	var err error
	var conn net.Conn
	var dialProto string
	var dialAddress string
	dialProto = "tcp"
	dialAddress = nodeAddress
	conn, err = net.Dial(dialProto, dialAddress)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func handleRequestTxIds(
	ctx txsubmission.CallbackContext,
	blocking bool,
	ack uint16,
	req uint16,
) ([]txsubmission.TxIdAndSize, error) {
	if ntnSentTx {
		// Terrible syncronization hack for shutdown
		close(ntnDoneChan)
		time.Sleep(5 * time.Second)
		return nil, nil
	}
	ret := []txsubmission.TxIdAndSize{
		{
			TxId: txsubmission.TxId{
				EraId: uint16(ntnTxType), // #nosec G115
				TxId:  ntnTxHash,
			},
			Size: uint32(len(ntnTxBytes)), // #nosec G115
		},
	}
	return ret, nil
}

func handleRequestTxs(
	ctx txsubmission.CallbackContext,
	txIds []txsubmission.TxId,
) ([]txsubmission.TxBody, error) {
	ret := []txsubmission.TxBody{
		{
			EraId:  uint16(ntnTxType), // #nosec G115
			TxBody: ntnTxBytes,
		},
	}
	ntnSentTx = true
	return ret, nil
}
