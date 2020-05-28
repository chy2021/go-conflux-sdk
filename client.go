// Copyright 2019 Conflux Foundation. All rights reserved.
// Conflux is free software and distributed under GNU General Public License.
// See http://www.gnu.org/licenses/

package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/Conflux-Chain/go-conflux-sdk/constants"
	"github.com/Conflux-Chain/go-conflux-sdk/rpc"
	"github.com/Conflux-Chain/go-conflux-sdk/types"
	"github.com/Conflux-Chain/go-conflux-sdk/utils"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Client represents a client to interact with Conflux blockchain.
type Client struct {
	nodeURL        string
	rpcRequester   rpcRequester
	accountManager AccountManagerOperator
}

// NewClient creates a new instance of Client with specified conflux node url.
func NewClient(nodeURL string) (*Client, error) {
	client, err := NewClientWithRetry(nodeURL, 0, 0)
	return client, err
}

// NewClientWithRetry creates a retryable new instance of Client with specified conflux node url and retry options.
//
// the retryInterval will be set to 1 second if pass 0
func NewClientWithRetry(nodeURL string, retryCount int, retryInterval time.Duration) (*Client, error) {

	var client Client
	client.nodeURL = nodeURL

	rpcClient, err := rpc.Dial(nodeURL)
	if err != nil {
		return nil, types.WrapError(err, "dail failed")
	}

	if retryCount == 0 {
		client.rpcRequester = rpcClient
	} else {
		// Interval 0 is meaningless and may lead full node busy, so default sets it to 1 second
		if retryInterval == 0 {
			retryInterval = time.Second
		}

		client.rpcRequester = &rpcClientWithRetry{
			inner:      rpcClient,
			retryCount: retryCount,
			interval:   retryInterval,
		}
	}

	return &client, nil
}

// NewClientWithRPCRequester creates client with specified rpcRequester
func NewClientWithRPCRequester(rpcRequester rpcRequester) (*Client, error) {
	return &Client{
		rpcRequester: rpcRequester,
	}, nil
}

type rpcClientWithRetry struct {
	inner      *rpc.Client
	retryCount int
	interval   time.Duration
}

func (r *rpcClientWithRetry) Call(resultPtr interface{}, method string, args ...interface{}) error {
	err := r.inner.Call(resultPtr, method, args...)
	if err == nil {
		return nil
	}

	if r.retryCount <= 0 {
		return err
	}

	remain := r.retryCount
	for {
		if err = r.inner.Call(resultPtr, method, args...); err == nil {
			return nil
		}

		remain--
		if remain == 0 {
			msg := fmt.Sprintf("timeout when call %v with args %v", method, args)
			return types.WrapError(err, msg)
		}

		if r.interval > 0 {
			time.Sleep(r.interval)
		}
	}
}

func (r *rpcClientWithRetry) BatchCall(b []rpc.BatchElem) error {
	err := r.inner.BatchCall(b)
	if err == nil {
		return nil
	}

	if r.retryCount <= 0 {
		return err
	}

	remain := r.retryCount
	for {
		if err = r.inner.BatchCall(b); err == nil {
			return nil
		}

		remain--
		if remain == 0 {
			msg := fmt.Sprintf("timeout when batch call %+v", b)
			return types.WrapError(err, msg)
		}

		if r.interval > 0 {
			time.Sleep(r.interval)
		}
	}
}

func (r *rpcClientWithRetry) Close() {
	if r != nil && r.inner != nil {
		r.inner.Close()
	}
}

func (client *Client) GetNodeUrl() string {
	return client.nodeURL
}

// CallRPC performs a JSON-RPC call with the given arguments and unmarshals into
// result if no error occurred.
//
// The result must be a pointer so that package json can unmarshal into it. You
// can also pass nil, in which case the result is ignored.
func (client *Client) CallRPC(result interface{}, method string, args ...interface{}) error {
	return client.rpcRequester.Call(result, method, args...)
}

func (client *Client) BatchCallRPC(b []rpc.BatchElem) error {
	return client.rpcRequester.BatchCall(b)
}

// SetAccountManager sets account manager for sign transaction
func (client *Client) SetAccountManager(accountManager AccountManagerOperator) {
	client.accountManager = accountManager
}

// GetGasPrice returns the recent mean gas price.
func (client *Client) GetGasPrice() (*big.Int, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_gasPrice"); err != nil {
		msg := "rpc request cfx_gasPrice error"
		return nil, types.WrapError(err, msg)
	}

	return hexutil.DecodeBig(result.(string))
}

// GetNextNonce returns the next transaction nonce of address
func (client *Client) GetNextNonce(address types.Address, epoch *types.Epoch) (*big.Int, error) {
	var result interface{}
	args := []interface{}{address}
	if epoch != nil {
		args = append(args, epoch)
	}

	if err := client.rpcRequester.Call(&result, "cfx_getNextNonce", args...); err != nil {
		msg := fmt.Sprintf("rpc request cfx_getNextNonce %+v error", address)
		return nil, types.WrapErrorf(err, msg)
	}
	return hexutil.DecodeBig(result.(string))

	// // remove prefix "0x"
	// result = string([]byte(result.(string))[2:])
	// nonce, err := strconv.ParseUint(result.(string), 16, 64)
	// if err != nil {
	// 	msg := fmt.Sprintf("parse uint %+v error", result)
	// 	return 0, types.WrapError(err, msg)
	// }

	// return nonce, nil
}

// GetEpochNumber returns the highest or specified epoch number.
func (client *Client) GetEpochNumber(epoch ...*types.Epoch) (*big.Int, error) {
	var result interface{}

	var args []interface{}
	if len(epoch) > 0 {
		args = append(args, epoch[0])
	}

	if err := client.rpcRequester.Call(&result, "cfx_epochNumber", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_epochNumber %+v error", args)
		return nil, types.WrapError(err, msg)
	}

	return hexutil.DecodeBig(result.(string))
}

// GetBalance returns the balance of specified address at epoch.
func (client *Client) GetBalance(address types.Address, epoch ...*types.Epoch) (*big.Int, error) {
	var result interface{}

	args := []interface{}{address}
	if len(epoch) > 0 {
		args = append(args, epoch[0])
	}

	if err := client.rpcRequester.Call(&result, "cfx_getBalance", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBalance %+v error", args)
		return nil, types.WrapError(err, msg)
	}

	return hexutil.DecodeBig(result.(string))
}

// GetCode returns the bytecode in HEX format of specified address at epoch.
func (client *Client) GetCode(address types.Address, epoch ...*types.Epoch) (string, error) {
	var result interface{}

	args := []interface{}{address}
	if len(epoch) > 0 {
		args = append(args, epoch[0])
	}

	if err := client.rpcRequester.Call(&result, "cfx_getCode", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_getCode %+v error", args)
		return "", types.WrapError(err, msg)
	}

	return result.(string), nil
}

// GetBlockSummaryByHash returns the block summary of specified blockHash
// If the block is not found, return nil.
func (client *Client) GetBlockSummaryByHash(blockHash types.Hash) (*types.BlockSummary, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getBlockByHash", blockHash, false); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBlockByHash %+v error", blockHash)
		return nil, types.WrapError(err, msg)
	}

	if result == nil {
		return nil, nil
	}

	var block types.BlockSummary
	if err := unmarshalRPCResult(result, &block); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &block, nil
}

// GetBlockByHash returns the block of specified blockHash
// If the block is not found, return nil.
func (client *Client) GetBlockByHash(blockHash types.Hash) (*types.Block, error) {
	var result interface{}

	// fmt.Println("start get block by hash in function GetBlockByHash: ", blockHash, "; client node url is", client.GetNodeUrl())
	if err := client.rpcRequester.Call(&result, "cfx_getBlockByHash", blockHash, true); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBlockByHash %+v error", blockHash)
		return nil, types.WrapError(err, msg)
	}
	fmt.Println("get block by hash done")

	if result == nil {
		return nil, nil
	}

	fmt.Println("start unmarshal block result")

	var block types.Block
	if err := unmarshalRPCResult(result, &block); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &block, nil
}

// GetBlockSummaryByEpoch returns the block summary of specified epoch.
// If the epoch is invalid, return the concrete error.
func (client *Client) GetBlockSummaryByEpoch(epoch *types.Epoch) (*types.BlockSummary, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getBlockByEpochNumber", epoch, false); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBlockByEpochNumber %+v error", epoch)
		return nil, types.WrapError(err, msg)
	}

	var block types.BlockSummary
	if err := unmarshalRPCResult(result, &block); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &block, nil
}

// GetBlockByEpoch returns the block of specified epoch.
// If the epoch is invalid, return the concrete error.
func (client *Client) GetBlockByEpoch(epoch *types.Epoch) (*types.Block, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getBlockByEpochNumber", epoch, true); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBlockByEpochNumber %+v error", epoch)
		return nil, types.WrapError(err, msg)
	}

	var block types.Block
	if err := unmarshalRPCResult(result, &block); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &block, nil
}

// GetBestBlockHash returns the current best block hash.
func (client *Client) GetBestBlockHash() (types.Hash, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getBestBlockHash"); err != nil {
		msg := "rpc cfx_getBestBlockHash error"
		return "", types.WrapError(err, msg)
	}

	return types.Hash(result.(string)), nil
}

// GetBlockConfirmRiskByHash indicates the risk coefficient that
// the pivot block of the epoch where the block is located becomes an normal block.
func (client *Client) GetBlockConfirmRiskByHash(blockhash types.Hash) (*big.Int, error) {
	var result interface{}

	args := []interface{}{blockhash}

	if err := client.rpcRequester.Call(&result, "cfx_getConfirmationRiskByHash", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_getConfirmationRiskByHash %+v error", args)
		return nil, types.WrapError(err, msg)
	}

	fmt.Printf("GetTransactionConfirmRiskByHash of block %v result:%v\n", blockhash, result)

	if result == nil {

		block, err := client.GetBlockSummaryByHash(blockhash)
		if err != nil {
			msg := fmt.Sprintf("get block by hash %+v error", blockhash)
			return nil, types.WrapError(err, msg)
		}
		if block != nil && block.EpochNumber != nil {
			return big.NewInt(0), nil
		}

		return constants.MaxUint256, nil
	}

	return hexutil.DecodeBig(result.(string))
}

// GetBlockRevertRateByHash indicates the probability that
// the pivot block of the epoch where the block is located becomes an ordinary block.
//
// it's (confirm risk coefficient/ (2^256-1))
func (client *Client) GetBlockRevertRateByHash(blockHash types.Hash) (*big.Float, error) {
	risk, err := client.GetBlockConfirmRiskByHash(blockHash)
	if err != nil {
		msg := fmt.Sprintf("get block confirmation risk by hash %+v error", blockHash)
		return nil, types.WrapError(err, msg)
	}
	if risk == nil {
		return nil, nil
	}

	riskFloat := new(big.Float).SetInt(risk)
	maxUint256Float := new(big.Float).SetInt(constants.MaxUint256)

	riskRate := new(big.Float).Quo(riskFloat, maxUint256Float)
	return riskRate, nil
}

// SendTransaction signs and sends transaction to conflux node and returns the transaction hash.
func (client *Client) SendTransaction(tx *types.UnsignedTransaction) (types.Hash, error) {

	err := client.ApplyUnsignedTransactionDefault(tx)
	if err != nil {
		msg := fmt.Sprintf("apply transaction {%+v} default fields error", *tx)
		return "", types.WrapError(err, msg)
	}

	// commet it becasue there are some contract need not pay gas.
	//
	// //check balance, return error if balance not enough
	// epoch := types.NewEpochNumber(tx.EpochHeight.ToInt())
	// balance, err := c.GetBalance(*tx.From, epoch)
	// if err != nil {
	// 	msg := fmt.Sprintf("get balance of %+v at ephoc %+v error", tx.From, epoch)
	// 	return "", types.WrapError(err, msg)
	// }
	// need := big.NewInt(int64(tx.Gas))
	// need = need.Add(tx.StorageLimit.ToInt(), need)
	// need = need.Mul(tx.GasPrice.ToInt(), need)
	// need = need.Add(tx.Value.ToInt(), need)
	// need = need.Add(tx.StorageLimit.ToInt(), need)

	// if balance.Cmp(need) < 0 {
	// 	msg := fmt.Sprintf("out of balance, need %+v but your balance is %+v", need, balance)
	// 	return "", types.WrapError(err, msg)
	// }

	//sign
	// fmt.Printf("ready to send transaction %+v\n\n", tx)

	if client.accountManager == nil {
		msg := fmt.Sprintf("sign transaction need account manager, please call SetAccountManager to set it.")
		return "", errors.New(msg)
	}

	rawData, err := client.accountManager.SignTransaction(*tx)
	if err != nil {
		msg := fmt.Sprintf("sign transaction {%+v} error", *tx)
		return "", types.WrapError(err, msg)
	}

	// fmt.Printf("signed raw data: %x", rawData)
	//send raw tx
	txhash, err := client.SendRawTransaction(rawData)
	if err != nil {
		msg := fmt.Sprintf("send raw transaction 0x%+x error", rawData)
		return "", types.WrapError(err, msg)
	}
	return txhash, nil
}

// SendRawTransaction sends signed transaction and returns its hash.
func (client *Client) SendRawTransaction(rawData []byte) (types.Hash, error) {
	var result interface{}
	// fmt.Printf("send raw transaction %x\n", rawData)
	if err := client.rpcRequester.Call(&result, "cfx_sendRawTransaction", hexutil.Encode(rawData)); err != nil {
		msg := fmt.Sprintf("rpc cfx_sendRawTransaction 0x%+x error", rawData)
		return "", types.WrapError(err, msg)
	}

	return types.Hash(result.(string)), nil
}

// SignEncodedTransactionAndSend signs RLP encoded transaction "encodedTx" by signature "r,s,v" and sends it to node,
// and returns responsed transaction.
func (client *Client) SignEncodedTransactionAndSend(encodedTx []byte, v byte, r, s []byte) (*types.Transaction, error) {
	tx := new(types.UnsignedTransaction)
	err := tx.Decode(encodedTx)
	if err != nil {
		msg := fmt.Sprintf("Decode rlp encoded data {%+v} to unsignTransction error", encodedTx)
		return nil, types.WrapError(err, msg)
	}
	// tx.From = from

	respondTx, err := client.signTransactionAndSend(tx, v, r, s)
	if err != nil {
		msg := fmt.Sprintf("sign transaction and send {tx: %+v, r:%+x, s:%+x, v:%v} error", tx, r, s, v)
		return nil, types.WrapError(err, msg)
	}

	return respondTx, nil
}

func (client *Client) signTransactionAndSend(tx *types.UnsignedTransaction, v byte, r, s []byte) (*types.Transaction, error) {
	rlp, err := tx.EncodeWithSignature(v, r, s)
	if err != nil {
		msg := fmt.Sprintf("encode tx %+v with signature { v:%+x, r:%+x, s:%v} error", tx, v, r, s)
		return nil, types.WrapError(err, msg)
	}

	hash, err := client.SendRawTransaction(rlp)
	if err != nil {
		msg := fmt.Sprintf("send signed tx %+x error", rlp)
		return nil, types.WrapError(err, msg)
	}

	respondTx, err := client.GetTransactionByHash(hash)
	if err != nil {
		msg := fmt.Sprintf("get transaction by hash %+v error", hash)
		return nil, types.WrapError(err, msg)
	}
	return respondTx, nil
}

// Call executes a message call transaction "request" at specified epoch,
// which is directly executed in the VM of the node, but never mined into the block chain
// and returns the contract execution result.
func (client *Client) Call(request types.CallRequest, epoch *types.Epoch) (*string, error) {
	var rpcResult interface{}

	args := []interface{}{request}
	// if len(epoch) > 0 {
	if epoch != nil {
		// args = append(args, epoch[0])
		args = append(args, epoch)
	}

	if err := client.rpcRequester.Call(&rpcResult, "cfx_call", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_call {%+v} error", args)
		return nil, types.WrapError(err, msg)
	}

	var resultHexStr string
	if err := unmarshalRPCResult(rpcResult, &resultHexStr); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", rpcResult)
		return nil, types.WrapError(err, msg)
	}
	return &resultHexStr, nil
}

// GetLogs returns logs that matching the specified filter.
func (client *Client) GetLogs(filter types.LogFilter) ([]types.Log, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getLogs", filter); err != nil {
		msg := fmt.Sprintf("rpc cfx_getLogs of {%+v} error", filter)
		return nil, types.WrapError(err, msg)
	}

	var log []types.Log
	if err := unmarshalRPCResult(result, &log); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return log, nil
}

// GetTransactionByHash returns transaction for the specified txHash.
// If the transaction is not found, return nil.
func (client *Client) GetTransactionByHash(txHash types.Hash) (*types.Transaction, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getTransactionByHash", txHash); err != nil {
		msg := fmt.Sprintf("rpc cfx_getTransactionByHash {%+v} error", txHash)
		return nil, types.WrapError(err, msg)
	}

	if result == nil {
		return nil, nil
	}

	var tx types.Transaction
	if err := unmarshalRPCResult(result, &tx); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &tx, nil
}

// EstimateGasAndCollateral excutes a message call "request"
// and returns the amount of the gas used and storage for collateral
func (client *Client) EstimateGasAndCollateral(request types.CallRequest) (*types.Estimate, error) {
	var result interface{}

	args := []interface{}{request}

	if err := client.rpcRequester.Call(&result, "cfx_estimateGasAndCollateral", args...); err != nil {
		msg := fmt.Sprintf("rpc cfx_estimateGasAndCollateral of {%+v} error", args)
		return nil, types.WrapError(err, msg)
	}
	var estimate types.Estimate
	if err := unmarshalRPCResult(result, &estimate); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &estimate, nil
}

// GetBlocksByEpoch returns the blocks hash in the specified epoch.
func (client *Client) GetBlocksByEpoch(epoch *types.Epoch) ([]types.Hash, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getBlocksByEpoch", epoch); err != nil {
		msg := fmt.Sprintf("rpc cfx_getBlocksByEpoch {%+v} error", epoch)
		return nil, types.WrapError(err, msg)
	}

	var blocks []types.Hash
	if err := unmarshalRPCResult(result, &blocks); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return blocks, nil
}

// GetTransactionReceipt returns the receipt of specified transaction hash.
// If no receipt is found, return nil.
func (client *Client) GetTransactionReceipt(txHash types.Hash) (*types.TransactionReceipt, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, "cfx_getTransactionReceipt", txHash); err != nil {
		msg := fmt.Sprintf("rpc cfx_getTransactionReceipt of {%+v} error", txHash)
		return nil, types.WrapError(err, msg)
	}

	if result == nil {
		return nil, nil
	}

	var receipt types.TransactionReceipt
	if err := unmarshalRPCResult(result, &receipt); err != nil {
		msg := fmt.Sprintf("UnmarshalRPCResult %+v error", result)
		return nil, types.WrapError(err, msg)
	}

	return &receipt, nil
}

// CreateUnsignedTransaction creates an unsigned transaction by parameters,
// and the other fields will be set to values fetched from conflux node.
func (client *Client) CreateUnsignedTransaction(from types.Address, to types.Address, amount *hexutil.Big, data []byte) (*types.UnsignedTransaction, error) {
	tx := new(types.UnsignedTransaction)
	tx.From = &from
	tx.To = &to
	tx.Value = amount
	tx.Data = data

	err := client.ApplyUnsignedTransactionDefault(tx)
	if err != nil {
		msg := fmt.Sprintf("apply default field of transaction {%+v} error", tx)
		return nil, types.WrapError(err, msg)
	}

	return tx, nil
}

// ApplyUnsignedTransactionDefault set empty fields to value fetched from conflux node.
func (client *Client) ApplyUnsignedTransactionDefault(tx *types.UnsignedTransaction) error {

	if client != nil {
		if tx.From == nil {
			if client.accountManager != nil {
				defaultAccount, err := client.accountManager.GetDefault()
				if err != nil {
					return types.WrapError(err, "get default account error")
				}

				if defaultAccount == nil {
					return errors.New("no account exist in keystore directory")
				}
				tx.From = defaultAccount
			}
		}

		if tx.Nonce == nil {
			nonce, err := client.GetNextNonce(*tx.From, nil)
			if err != nil {
				msg := fmt.Sprintf("get nonce of {%+v} error", tx.From)
				return types.WrapError(err, msg)
			}
			tmp := hexutil.Big(*nonce)
			tx.Nonce = &tmp
		}

		if tx.GasPrice == nil {
			gasPrice, err := client.GetGasPrice()
			if err != nil {
				msg := "get gas price error"
				return types.WrapError(err, msg)
			}

			// conflux responsed gasprice offen be 0, but the min gasprice is 1 when sending transaction, so do this
			if gasPrice.Cmp(big.NewInt(constants.MinGasprice)) < 1 {
				gasPrice = big.NewInt(constants.MinGasprice)
			}
			tmp := hexutil.Big(*gasPrice)
			tx.GasPrice = &tmp
		}

		if tx.EpochHeight == nil {
			epoch, err := client.GetEpochNumber(types.EpochLatestState)
			if err != nil {
				msg := fmt.Sprintf("get epoch number of {%+v} error", types.EpochLatestState)
				return types.WrapError(err, msg)
			}
			tx.EpochHeight = (*hexutil.Big)(epoch)
		}

		// The gas and storage limit may be influnced by all fileds of transaction ,so set them at last step.
		if tx.StorageLimit == nil || tx.Gas == nil {
			callReq := new(types.CallRequest)
			callReq.FillByUnsignedTx(tx)

			sm, err := client.EstimateGasAndCollateral(*callReq)
			if err != nil {
				msg := fmt.Sprintf("get estimate gas and collateral by {%+v} error", *callReq)
				return types.WrapError(err, msg)
			}

			if tx.Gas == nil {
				tx.Gas = sm.GasUsed
			}

			if tx.StorageLimit == nil {
				tx.StorageLimit = sm.StorageCollateralized
			}
		}

		tx.ApplyDefault()
	}

	return nil
}

// Debug calls the Conflux debug API.
func (client *Client) Debug(method string, args ...interface{}) (interface{}, error) {
	var result interface{}

	if err := client.rpcRequester.Call(&result, method, args...); err != nil {
		msg := fmt.Sprintf("rpc call method {%+v} with args {%+v} error", method, args)
		return nil, types.WrapError(err, msg)
	}

	return result, nil
}

// DeployContract deploys a contract by abiJSON, bytecode and consturctor params.
// It returns a ContractDeployState instance which contains 3 channels for notifying when state changed.
func (client *Client) DeployContract(option *types.ContractDeployOption, abiJSON []byte,
	bytecode []byte, constroctorParams ...interface{}) *ContractDeployResult {

	doneChan := make(chan struct{})
	result := ContractDeployResult{DoneChannel: doneChan}

	go func() {

		defer func() {
			doneChan <- struct{}{}
			close(doneChan)
		}()

		//generate ABI
		var abi abi.ABI
		err := abi.UnmarshalJSON([]byte(abiJSON))
		if err != nil {
			msg := fmt.Sprintf("unmarshal json {%+v} to ABI error", abiJSON)
			result.Error = types.WrapError(err, msg)
			return
		}

		tx := new(types.UnsignedTransaction)
		if option != nil {
			tx.UnsignedTransactionBase = types.UnsignedTransactionBase(option.UnsignedTransactionBase)
		}

		//recreate contract bytecode with consturctor params
		if len(constroctorParams) > 0 {
			input, err := abi.Pack("", constroctorParams...)
			if err != nil {
				msg := fmt.Sprintf("encode constrctor with args %+v error", constroctorParams)
				result.Error = types.WrapError(err, msg)
				return
			}

			bytecode = append(bytecode, input...)
		}
		tx.Data = bytecode

		//deploy contract
		txhash, err := client.SendTransaction(tx)
		if err != nil {
			msg := fmt.Sprintf("send transaction {%+v} error", tx)
			result.Error = types.WrapError(err, msg)
			return
		}
		result.TransactionHash = &txhash

		// timeout := time.After(time.Duration(_timeoutIns) * time.Second)
		timeout := time.After(3600 * time.Second)
		if option != nil && option.Timeout != 0 {
			timeout = time.After(option.Timeout)
		}

		ticker := time.Tick(2000 * time.Millisecond)
		// Keep trying until we're time out or get a result or get an error
		for {
			select {
			// Got a timeout! fail with a timeout error
			case t := <-timeout:
				msg := fmt.Sprintf("deploy contract time out after %v, txhash is %+v", t, txhash)
				result.Error = errors.New(msg)
				return
			// Got a tick
			case <-ticker:
				transaction, err := client.GetTransactionByHash(txhash)
				if err != nil {
					msg := fmt.Sprintf("get transaction receipt of txhash %+v error", txhash)
					result.Error = types.WrapError(err, msg)
					return
				}

				if transaction.Status != nil {
					if transaction.Status.ToInt().Uint64() == 1 {
						msg := fmt.Sprintf("transaction is packed but it is failed,the txhash is %+v", txhash)
						result.Error = errors.New(msg)
						return
					}

					result.DeployedContract = &Contract{abi, client, transaction.ContractCreated}
					return
				}
			}
		}
	}()
	return &result
}

// GetContract creates a contract instance according to abi json and it's deployed address
func (client *Client) GetContract(abiJSON []byte, deployedAt *types.Address) (*Contract, error) {
	var abi abi.ABI
	err := abi.UnmarshalJSON([]byte(abiJSON))
	if err != nil {
		msg := fmt.Sprintf("unmarshal json {%+v} to ABI error", abiJSON)
		return nil, types.WrapError(err, msg)
	}

	contract := &Contract{abi, client, deployedAt}
	return contract, nil
}

// BatchGetTxByHashs ...
func (client *Client) BatchGetTxByHashs(txhashs []types.Hash) ([]*types.Transaction, error) {
	bes := make([]rpc.BatchElem, len(txhashs))
	txs := make([]types.Transaction, len(txhashs))
	for i := range txhashs {
		be := rpc.BatchElem{
			Method: "cfx_getTransactionByHash",
			Args:   []interface{}{txhashs[i]},
			Result: &txs[i],
		}
		bes[i] = be
	}
	// fmt.Printf("send BatchItems: %+v \n", bes)
	if err := client.BatchCallRPC(bes); err != nil {
		return nil, err
	}

	results := make([]*types.Transaction, len(txhashs))
	for i := range txs {
		if (txs[i] == types.Transaction{}) {
			results[i] = nil
			continue
		}
		results[i] = &txs[i]
	}

	return results, nil
}

// BatchGetBlockRevertRates ...
func (client *Client) BatchGetBlockRevertRates(blockhashs []*types.Hash) ([]*big.Float, error) {

	// fmt.Printf("start batch get block revert rate of blockhashs %v\n", blockhashs)
	if len(blockhashs) == 0 {
		return []*big.Float{}, nil
	}

	// batch get block revert rate
	bElems := make([]rpc.BatchElem, 0, len(blockhashs))
	risks := make([]string, len(blockhashs))

	for i := range blockhashs {
		if blockhashs[i] == nil {
			risks[i] = hexutil.EncodeBig(constants.MaxUint256)
			continue
		}

		be := rpc.BatchElem{
			Method: "cfx_getConfirmationRiskByHash",
			Args:   []interface{}{blockhashs[i]},
			Result: &risks[i],
		}
		bElems = append(bElems, be)
	}
	if err := client.BatchCallRPC(bElems); err != nil {
		msg := fmt.Sprintf("batch call cfx_getConfirmationRiskByHash with BatchItem %v error", bElems)
		return nil, types.WrapError(err, msg)
	}

	// filter blockhashs without revert rate
	noRiskBlockhashs := make([]*types.Hash, 0)
	// reduplicated block hash may exists, so set the value to a slice
	indexMap := make(map[types.Hash][]int)
	for i := range blockhashs {
		if len(risks[i]) == 0 {
			noRiskBlockhashs = append(noRiskBlockhashs, blockhashs[i])
			if indexMap[*blockhashs[i]] == nil {
				indexMap[*blockhashs[i]] = make([]int, 0)
			}
			indexMap[*blockhashs[i]] = append(indexMap[*blockhashs[i]], i)
		}
	}

	if len(noRiskBlockhashs) > 0 {
		// get blockSummarys, if block is valid set risk to 0, otherwise set to MaxUint256
		summarys := make([]types.BlockSummary, len(noRiskBlockhashs))
		bElems = make([]rpc.BatchElem, len(noRiskBlockhashs))

		for i := range noRiskBlockhashs {
			be := rpc.BatchElem{
				Method: "cfx_getBlockByHash",
				Args:   []interface{}{*noRiskBlockhashs[i], false},
				Result: &summarys[i],
			}
			bElems = append(bElems, be)
		}
		if err := client.BatchCallRPC(bElems); err != nil {
			msg := fmt.Sprintf("batch call cfx_getBlockByHash with BatchItem %v error", bElems)
			return nil, types.WrapError(err, msg)
		}

		for i := range bElems {
			indexs := indexMap[summarys[i].Hash]
			var risk string
			if summarys[i].EpochNumber != nil {
				risk = big.NewInt(0).String()
			} else {
				risk = constants.MaxUint256.String()
			}

			for _, iRisks := range indexs {
				risks[iRisks] = risk
			}

		}
	}

	// calculate risk to revert rate
	rates := make([]*big.Float, len(risks))
	for i := range risks {
		b, err := hexutil.DecodeBig(risks[i])
		if err != nil {
			msg := fmt.Sprintf("create big int by risk value %v error", risks[i])
			return nil, types.WrapError(err, msg)
		}
		rates[i] = utils.CalcBlockRevertRate(b)
	}
	return rates, nil
}

// Close closes the client, aborting any in-flight requests.
func (client *Client) Close() {
	client.rpcRequester.Close()
}

func unmarshalRPCResult(result interface{}, v interface{}) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		msg := fmt.Sprintf("json marshal %v error", result)
		return types.WrapError(err, msg)
	}

	if err = json.Unmarshal(encoded, v); err != nil {
		msg := fmt.Sprintf("json unmarshal %v error", encoded)
		return types.WrapError(err, msg)
	}

	return nil
}
