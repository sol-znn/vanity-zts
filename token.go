package main

import (
	"fmt"
	"math/big"
	"regexp"

	"github.com/zenon-network/go-zenon/vm/constants"
	"github.com/zenon-network/go-zenon/vm/embedded/definition"
)

// tokenIssueAmount is the fixed ZNN fee burned by an IssueToken call.
func tokenIssueAmount() *big.Int {
	return new(big.Int).Set(constants.TokenIssueAmount)
}

// IssueParams are the token metadata the caller wants to mint. They are
// fixed inputs to the mined block's Data field; nothing about the token
// itself is varied by the vanity search.
type IssueParams struct {
	TokenName   string
	TokenSymbol string
	TokenDomain string
	TotalSupply *big.Int
	MaxSupply   *big.Int
	Decimals    uint8
	IsMintable  bool
	IsBurnable  bool
	IsUtility   bool
}

var (
	tokenNameRegexp   = regexp.MustCompile(`^([a-zA-Z0-9]+[-._]?)*[a-zA-Z0-9]$`)
	tokenSymbolRegexp = regexp.MustCompile(`^[A-Z0-9]+$`)
	tokenDomainRegexp = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9-]{0,61}[A-Za-z0-9]\.)+[A-Za-z]{2,}$`)
)

// Validate mirrors go-zenon's vm/embedded/implementation/token.go:checkToken
// exactly, so invalid parameters are rejected here instead of after the
// (possibly lengthy) vanity search, or worse, after being rejected by the
// node once submitted.
func (p IssueParams) Validate() error {
	if len(p.TokenName) == 0 || len(p.TokenName) > constants.TokenNameLengthMax {
		return fmt.Errorf("token name must be 1-%d characters", constants.TokenNameLengthMax)
	}
	if !tokenNameRegexp.MatchString(p.TokenName) {
		return fmt.Errorf("token name %q must be alphanumeric, optionally separated by a single '-', '_' or '.'", p.TokenName)
	}

	if len(p.TokenSymbol) == 0 || len(p.TokenSymbol) > constants.TokenSymbolLengthMax {
		return fmt.Errorf("token symbol must be 1-%d characters", constants.TokenSymbolLengthMax)
	}
	if !tokenSymbolRegexp.MatchString(p.TokenSymbol) {
		return fmt.Errorf("token symbol %q must be uppercase letters and digits only", p.TokenSymbol)
	}
	if p.TokenSymbol == "ZNN" || p.TokenSymbol == "QSR" {
		return fmt.Errorf("token symbol %q is reserved", p.TokenSymbol)
	}

	if len(p.TokenDomain) > constants.TokenDomainLengthMax {
		return fmt.Errorf("token domain must be at most %d characters", constants.TokenDomainLengthMax)
	}
	if p.TokenDomain != "" && !tokenDomainRegexp.MatchString(p.TokenDomain) {
		return fmt.Errorf("token domain %q is not a valid domain name", p.TokenDomain)
	}

	if int(p.Decimals) > constants.TokenMaxDecimals {
		return fmt.Errorf("decimals must be at most %d", constants.TokenMaxDecimals)
	}

	if p.MaxSupply == nil || p.MaxSupply.Sign() <= 0 {
		return fmt.Errorf("max supply must be positive")
	}
	if p.MaxSupply.Cmp(constants.TokenMaxSupplyBig) > 0 {
		return fmt.Errorf("max supply exceeds the protocol maximum of %s (2^255-1)", constants.TokenMaxSupplyBig)
	}
	if p.TotalSupply == nil || p.TotalSupply.Sign() < 0 {
		return fmt.Errorf("total supply must not be negative")
	}
	if p.MaxSupply.Cmp(p.TotalSupply) < 0 {
		return fmt.Errorf("max supply (%s) must be >= total supply (%s)", p.MaxSupply, p.TotalSupply)
	}
	if !p.IsMintable && p.MaxSupply.Cmp(p.TotalSupply) != 0 {
		return fmt.Errorf("a non-mintable token must have total supply == max supply (got total %s, max %s); pass -mintable if you want them to differ", p.TotalSupply, p.MaxSupply)
	}
	return nil
}

// printLimits prints the protocol's IssueToken parameter limits, pulled
// directly from vm/constants so it can never drift from what the node
// actually enforces.
func printLimits() {
	fmt.Println("IssueToken protocol limits (vm/constants):")
	fmt.Printf("  name:      1-%d chars, matching %s\n", constants.TokenNameLengthMax, tokenNameRegexp.String())
	fmt.Printf("  symbol:    1-%d chars, matching %s (ZNN and QSR are reserved)\n", constants.TokenSymbolLengthMax, tokenSymbolRegexp.String())
	fmt.Printf("  domain:    optional, up to %d chars, matching %s\n", constants.TokenDomainLengthMax, tokenDomainRegexp.String())
	fmt.Printf("  decimals:  0-%d\n", constants.TokenMaxDecimals)
	fmt.Printf("  maxSupply: 1 to %s (2^255-1)\n", constants.TokenMaxSupplyBig)
	fmt.Println("  totalSupply: 0 to maxSupply; must equal maxSupply unless -mintable is set")
	fmt.Printf("  issue fee: %s (1 ZNN, fixed)\n", tokenIssueAmount())
}

// Encode ABI-packs the IssueToken call data exactly as go-zenon's embedded
// token contract expects it.
func (p IssueParams) Encode() ([]byte, error) {
	return definition.ABIToken.PackMethod(
		definition.IssueMethodName,
		p.TokenName,
		p.TokenSymbol,
		p.TokenDomain,
		p.TotalSupply,
		p.MaxSupply,
		p.Decimals,
		p.IsMintable,
		p.IsBurnable,
		p.IsUtility,
	)
}
