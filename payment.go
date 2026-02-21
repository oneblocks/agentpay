package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func Pay(cfg *Config, recipient string, amount int64) (string, error) {

	fmt.Println("===== Pay Start =====")

	if recipient == "" {
		fmt.Println("ERROR: recipient empty")
		return "", errors.New("recipient required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Println("Connecting RPC:", cfg.RPCURL)

	client, err := ethclient.Dial(cfg.RPCURL)
	if err != nil {
		fmt.Println("ERROR: RPC connect failed:", err)
		return "", err
	}

	pk := strings.TrimPrefix(cfg.PrivateKey, "0x")

	privateKeyBytes, err := hex.DecodeString(pk)
	if err != nil {
		fmt.Println("ERROR: private key decode failed:", err)
		return "", err
	}

	privateKey, err := crypto.ToECDSA(privateKeyBytes)
	if err != nil {
		fmt.Println("ERROR: private key convert failed:", err)
		return "", err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA := publicKey.(*ecdsa.PublicKey)
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	fmt.Println("From Address:", fromAddress.Hex())
	fmt.Println("Recipient:", recipient)
	fmt.Println("Amount:", amount)

	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		fmt.Println("ERROR: get nonce failed:", err)
		return "", err
	}

	fmt.Println("Nonce:", nonce)

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		fmt.Println("ERROR: suggest gas price failed:", err)
		return "", err
	}

	fmt.Println("GasPrice:", gasPrice)

	parsedABI, err := abi.JSON(strings.NewReader(ERC20ABI))
	if err != nil {
		fmt.Println("ERROR: parse ABI failed:", err)
		return "", err
	}

	value := big.NewInt(amount)

	toAddress := common.HexToAddress(recipient)
	contractAddress := common.HexToAddress(cfg.USDCAddress)

	data, err := parsedABI.Pack(
		"transfer",
		toAddress,
		value,
	)
	if err != nil {
		fmt.Println("ERROR: pack transfer failed:", err)
		return "", err
	}

	msg := ethereum.CallMsg{
		From: fromAddress,
		To:   &contractAddress,
		Data: data,
	}

	gasLimit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		fmt.Println("WARN: estimate gas failed, fallback to 100000:", err)
		gasLimit = 100000
	}

	fmt.Println("GasLimit:", gasLimit)

	tx := types.NewTransaction(
		nonce,
		contractAddress,
		big.NewInt(0),
		gasLimit,
		gasPrice,
		data,
	)

	chainID, err := client.NetworkID(ctx)
	if err != nil {
		fmt.Println("ERROR: get chainID failed:", err)
		return "", err
	}

	fmt.Println("ChainID:", chainID)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		fmt.Println("ERROR: sign tx failed:", err)
		return "", err
	}

	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		fmt.Println("ERROR: send tx failed:", err)
		return "", err
	}

	fmt.Println("SUCCESS: tx sent:", signedTx.Hash().Hex())
	fmt.Println("===== Pay End =====")

	return signedTx.Hash().Hex(), nil
}
