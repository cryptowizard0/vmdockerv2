package main

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/everFinance/goether"
	"github.com/hymatrix/hymx/pay"
	"github.com/hymatrix/hymx/pay/schema"
	"github.com/permadao/goar"
	"github.com/spf13/viper"
)

func LoadPayConfig() (*pay.Pay, error) {
	if !viper.GetBool("enablePayment") {
		return nil, nil
	}

	axURL := viper.GetString("payment.axURL")
	prvKey := viper.GetString("payment.prvKey")

	signer, err := goether.NewSigner(prvKey)
	if err != nil {
		return nil, err
	}
	bundler, err := goar.NewBundler(signer)
	if err != nil {
		return nil, err
	}

	settlementAddrStr := viper.GetString("payment.settlementAddress")
	axToken := viper.GetString("payment.axToken")
	txFeeStr := viper.GetString("payment.txFee")
	spawnFeeStr := viper.GetString("payment.spawnFee")
	residencyFeeStr := viper.GetString("payment.residencyFee")
	dailyLimit := viper.GetInt64("payment.dailyLimit")
	devRatioStr := viper.GetString("payment.developerShareRatio")
	if !common.IsHexAddress(settlementAddrStr) {
		return nil, fmt.Errorf("invalid payment.settlementAddress: %q", settlementAddrStr)
	}
	txFee, err := parseBigInt("payment.txFee", txFeeStr)
	if err != nil {
		return nil, err
	}
	spawnFee, err := parseBigInt("payment.spawnFee", spawnFeeStr)
	if err != nil {
		return nil, err
	}
	residencyFee, err := parseBigInt("payment.residencyFee", residencyFeeStr)
	if err != nil {
		return nil, err
	}
	devRatio, err := parseBigInt("payment.developerShareRatio", devRatioStr)
	if err != nil {
		return nil, err
	}

	cfg := &schema.Config{
		SettlementAddress:   common.HexToAddress(settlementAddrStr).String(),
		AxToken:             axToken,
		TxFee:               txFee,
		SpawnFee:            spawnFee,
		ResidencyFee:        residencyFee,
		DailyLimit:          dailyLimit,
		DeveloperShareRatio: devRatio,
	}

	return pay.New(axURL, bundler, cfg), nil
}

func parseBigInt(field, s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	bi, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("%s must be a base-10 integer, got %q", field, s)
	}
	return bi, nil
}
