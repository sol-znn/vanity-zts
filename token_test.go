package main

import (
	"math/big"
	"testing"

	"github.com/zenon-network/go-zenon/vm/constants"
)

func validParams() IssueParams {
	return IssueParams{
		TokenName:   "My-Token",
		TokenSymbol: "MTK",
		TokenDomain: "example.com",
		TotalSupply: big.NewInt(1000),
		MaxSupply:   big.NewInt(1000),
		Decimals:    8,
	}
}

func TestValidateAcceptsValidParams(t *testing.T) {
	if err := validParams().Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsReservedSymbol(t *testing.T) {
	p := validParams()
	p.TokenSymbol = "ZNN"
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for reserved symbol ZNN")
	}
}

func TestValidateRejectsLowercaseSymbol(t *testing.T) {
	p := validParams()
	p.TokenSymbol = "mtk"
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for lowercase symbol")
	}
}

func TestValidateRejectsOversizedName(t *testing.T) {
	p := validParams()
	name := ""
	for len(name) <= constants.TokenNameLengthMax {
		name += "a"
	}
	p.TokenName = name
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for oversized name")
	}
}

func TestValidateRejectsMaxSupplyAboveProtocolLimit(t *testing.T) {
	p := validParams()
	tooBig := new(big.Int).Add(constants.TokenMaxSupplyBig, big.NewInt(1))
	p.MaxSupply = tooBig
	p.TotalSupply = tooBig
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for max supply above 2^255-1")
	}
}

func TestValidateRejectsMismatchedSupplyWithoutMintable(t *testing.T) {
	p := validParams()
	p.MaxSupply = big.NewInt(2000)
	p.TotalSupply = big.NewInt(1000)
	p.IsMintable = false
	if err := p.Validate(); err == nil {
		t.Fatal("expected error: non-mintable token needs total == max supply")
	}
}

func TestValidateAllowsMismatchedSupplyWithMintable(t *testing.T) {
	p := validParams()
	p.MaxSupply = big.NewInt(2000)
	p.TotalSupply = big.NewInt(1000)
	p.IsMintable = true
	if err := p.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRejectsExcessiveDecimals(t *testing.T) {
	p := validParams()
	p.Decimals = uint8(constants.TokenMaxDecimals + 1)
	if err := p.Validate(); err == nil {
		t.Fatal("expected error for decimals above protocol max")
	}
}
