package main

import (
    "context"
    "crypto/ecdsa"
    "math/big"
    "strings"

    "github.com/ethereum/go-ethereum/accounts/abi"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/crypto"
    "github.com/ethereum/go-ethereum/ethclient"
)

func Pay(cfg *Config, to string, amount int64) (string, error) {

    client, err := ethclient.Dial(cfg.RPC)
    if err != nil {
        return "", err
    }

    privateKey, err := crypto.HexToECDSA(cfg.PrivateKey)
    if err != nil {
        return "", err
    }

    publicKey := privateKey.Public()
    publicKeyECDSA := publicKey.(*ecdsa.PublicKey)
    fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

    nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
    if err != nil {
        return "", err
    }

    gasPrice, err := client.SuggestGasPrice(context.Background())
    if err != nil {
        return "", err
    }

    contractAddress := common.HexToAddress(cfg.USDC)
    parsedABI, err := abi.JSON(strings.NewReader(ERC20ABI))
    if err != nil {
        return "", err
    }

    data, err := parsedABI.Pack("transfer",
        common.HexToAddress(to),
        big.NewInt(amount),
    )
    if err != nil {
        return "", err
    }

    tx := types.NewTransaction(
        nonce,
        contractAddress,
        big.NewInt(0),
        100000,
        gasPrice,
        data,
    )

    chainID, err := client.NetworkID(context.Background())
    if err != nil {
        return "", err
    }

    signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
    if err != nil {
        return "", err
    }

    err = client.SendTransaction(context.Background(), signedTx)
    if err != nil {
        return "", err
    }

    return signedTx.Hash().Hex(), nil
}
