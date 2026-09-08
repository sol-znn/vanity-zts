// Command vanity-zts mines a Zenon token-standard (ZTS) address whose
// bech32 body matches a chosen prefix/suffix/substring, the same way
// znn-address-generator mines a vanity z1... account address.
//
// A ZTS is derived from the hash of the IssueToken send-block that creates
// it (see common/types.NewZenonTokenStandard and
// vm/embedded/implementation/token.go:newTokenID in go-zenon). That hash
// covers, among other things, the block's Nonce and the momentum it
// acknowledges (MomentumAcknowledged). Two facts make those fields free to
// search over without invalidating the block:
//
//  1. When the sender has enough already-fused plasma to cover the
//     IssueToken call, the node sets Difficulty=0 and skips the
//     proof-of-work check entirely (verifier/account_block.go:pow) - so
//     Nonce can be any 8 bytes, not just the all-zero value the standard
//     wallets always send.
//  2. MomentumAcknowledged only has to name a momentum that exists on
//     chain (verifier/account_block.go:momentumAcknowledged) - not
//     necessarily the current frontier - so any recently confirmed
//     momentum height works.
//
// This tool fetches your account's frontier and a window of recent
// momentums once, then mines Nonce x Momentum combinations entirely
// offline (in memory, in parallel) until the resulting ZTS matches your
// pattern, then signs and (optionally) submits the single winning block.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/wallet"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3"
// (see .github/workflows/release.yml); it defaults to "dev" for local
// `go build`/`go run`.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		prefix       = flag.String("prefix", "", "required characters right after \"zts1\"")
		suffix       = flag.String("suffix", "", "required characters at the end of the address")
		contains     = flag.String("contains", "", "required substring anywhere in the address")
		keystorePath = flag.String("keystore", "", "path to a Syrius/znn_cli keystore file (required)")
		password     = flag.String("password", "", "keystore password (prompted if omitted)")
		index        = flag.Uint("index", 0, "account index to derive from the keystore")
		rpcURL       = flag.String("url", "http://127.0.0.1:35997", "node JSON-RPC HTTP endpoint")
		workers      = flag.Int("workers", 0, "worker goroutines (default: number of CPUs)")
		momentumSpan = flag.Uint64("momentum-window", 1, "search across this many of the most recent momentums, in addition to nonce, for extra entropy")
		tokenName    = flag.String("name", "", "token name, e.g. My-Token (alphanumeric, optionally '-'/'_'/'.' separated; required)")
		tokenSymbol  = flag.String("symbol", "", "token symbol, e.g. MTK (required)")
		tokenDomain  = flag.String("domain", "", "token domain, e.g. example.com")
		totalSupply  = flag.String("total-supply", "", "initial total supply, in the token's smallest unit (required)")
		maxSupply    = flag.String("max-supply", "", "max supply, in the token's smallest unit (defaults to total-supply; protocol max is 2^255-1, see -limits)")
		decimals     = flag.Uint("decimals", 8, "token decimals (protocol max 18)")
		mintable     = flag.Bool("mintable", false, "allow minting more supply later (required if max-supply > total-supply)")
		burnable     = flag.Bool("burnable", false, "allow anyone to burn the token")
		utility      = flag.Bool("utility", false, "mark the token as a utility token")
		submit       = flag.Bool("submit", false, "broadcast the winning block once found (default: print it and exit)")
		yes          = flag.Bool("yes", false, "skip the confirmation prompt before submitting")
		limits       = flag.Bool("limits", false, "print the protocol's token-parameter limits and exit")
		showVersion  = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("vanity-zts " + version)
		return nil
	}

	if *limits {
		printLimits()
		return nil
	}

	pattern := Pattern{Prefix: strings.ToLower(*prefix), Suffix: strings.ToLower(*suffix), Contains: strings.ToLower(*contains)}
	if pattern.Empty() {
		return fmt.Errorf("at least one of -prefix, -suffix, -contains is required")
	}
	for _, s := range []string{pattern.Prefix, pattern.Suffix, pattern.Contains} {
		if err := ValidateBech32(s); err != nil {
			return err
		}
	}

	if *keystorePath == "" {
		return fmt.Errorf("-keystore is required")
	}
	if *tokenName == "" || *tokenSymbol == "" || *totalSupply == "" {
		return fmt.Errorf("-name, -symbol and -total-supply are required")
	}
	pass := *password
	if pass == "" {
		var err error
		pass, err = promptPassword("keystore password: ")
		if err != nil {
			return err
		}
	}

	keyFile, err := wallet.ReadKeyFile(*keystorePath)
	if err != nil {
		return fmt.Errorf("reading keystore: %w", err)
	}
	keyStore, err := keyFile.Decrypt(pass)
	if err != nil {
		return fmt.Errorf("decrypting keystore (wrong password?): %w", err)
	}
	_, keyPair, err := keyStore.DeriveForIndexPath(uint32(*index))
	if err != nil {
		return fmt.Errorf("deriving account %d: %w", *index, err)
	}
	address := keyPair.Address
	fmt.Printf("issuing from %s\n", address)

	total, ok := new(big.Int).SetString(*totalSupply, 10)
	if !ok {
		return fmt.Errorf("invalid -total-supply %q", *totalSupply)
	}
	max := total
	if *maxSupply != "" {
		max, ok = new(big.Int).SetString(*maxSupply, 10)
		if !ok {
			return fmt.Errorf("invalid -max-supply %q", *maxSupply)
		}
	}
	if *decimals > 255 {
		return fmt.Errorf("-decimals %d out of range for a uint8", *decimals)
	}
	issueParams := IssueParams{
		TokenName:   *tokenName,
		TokenSymbol: *tokenSymbol,
		TokenDomain: *tokenDomain,
		TotalSupply: total,
		MaxSupply:   max,
		Decimals:    uint8(*decimals),
		IsMintable:  *mintable,
		IsBurnable:  *burnable,
		IsUtility:   *utility,
	}
	if err := issueParams.Validate(); err != nil {
		return fmt.Errorf("invalid token parameters: %w", err)
	}
	data, err := issueParams.Encode()
	if err != nil {
		return fmt.Errorf("encoding IssueToken call: %w", err)
	}

	client := NewClient(*rpcURL)

	frontier, err := GetFrontierAccountBlock(client, address)
	if err != nil {
		return fmt.Errorf("fetching frontier account-block: %w", err)
	}
	height := uint64(1)
	previousHash := types.ZeroHash
	if frontier != nil {
		height = frontier.Height + 1
		previousHash = frontier.Hash
	}

	frontierMomentum, err := GetFrontierMomentum(client)
	if err != nil {
		return fmt.Errorf("fetching frontier momentum: %w", err)
	}
	fmt.Printf("connected node's network id: %d\n", frontierMomentum.ChainIdentifier)

	momentums := []types.HashHeight{frontierMomentum.Identifier()}
	if *momentumSpan > 1 {
		start := uint64(1)
		if frontierMomentum.Height > *momentumSpan {
			start = frontierMomentum.Height - *momentumSpan + 1
		}
		list, err := GetMomentumsByHeight(client, start, frontierMomentum.Height-start+1)
		if err != nil {
			return fmt.Errorf("fetching momentum window: %w", err)
		}
		momentums = momentums[:0]
		for _, m := range list {
			momentums = append(momentums, m.Identifier())
		}
	}

	powInfo, err := GetRequiredPoWForAccountBlock(client, address, nom.BlockTypeUserSend, types.TokenContract, data)
	if err != nil {
		return fmt.Errorf("checking required plasma: %w", err)
	}
	fusedPlasma := powInfo.BasePlasma
	difficulty := powInfo.RequiredDifficulty
	if difficulty == 0 {
		fusedPlasma = powInfo.AvailablePlasma
		fmt.Println("sender has enough fused plasma; nonce is fully free to search")
	} else {
		fmt.Printf("sender lacks fused plasma; nonce must also satisfy PoW difficulty %d\n", difficulty)
	}

	template := Template{
		ChainIdentifier: frontierMomentum.ChainIdentifier,
		Address:         address,
		PreviousHash:    previousHash,
		Height:          height,
		Data:            data,
		FusedPlasma:     fusedPlasma,
		Difficulty:      difficulty,
		Momentums:       momentums,
	}

	fmt.Printf("searching for zts1 %s...%s (contains %q) across %d momentum(s), ~%.0f attempts expected\n",
		pattern.Prefix, pattern.Suffix, pattern.Contains, len(momentums), pattern.EstimatedAttempts())

	stop := make(chan struct{})
	var attempts uint64
	done := make(chan *Result, 1)
	go func() {
		done <- Mine(template, pattern, *workers, stop, &attempts)
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	start := time.Now()
	var result *Result
loop:
	for {
		select {
		case result = <-done:
			break loop
		case <-ticker.C:
			n := atomic.LoadUint64(&attempts)
			elapsed := time.Since(start).Seconds()
			fmt.Printf("\r%d attempts, %.0f/s   ", n, float64(n)/elapsed)
		}
	}
	fmt.Println()

	if result == nil {
		return fmt.Errorf("no match found")
	}

	fmt.Printf("found: %s\n", result.ZTS)
	fmt.Printf("  nonce:              %x\n", result.Block.Nonce.Data)
	fmt.Printf("  momentumAcknowledged: %s @ %d\n", result.Block.MomentumAcknowledged.Hash, result.Block.MomentumAcknowledged.Height)

	signature, addr, pubKey, err := keyPair.Signer(result.Hash.Bytes())
	if err != nil {
		return fmt.Errorf("signing block: %w", err)
	}
	if *addr != address {
		return fmt.Errorf("internal error: signer address mismatch")
	}
	result.Block.Signature = signature
	result.Block.PublicKey = pubKey

	blockJSON, err := jsonMarshalIndent(result.Block)
	if err != nil {
		return fmt.Errorf("marshalling block: %w", err)
	}
	fmt.Println(string(blockJSON))

	if !*submit {
		fmt.Println("dry run (pass -submit to broadcast this block)")
		return nil
	}

	if !*yes {
		ok, err := promptYesNo(fmt.Sprintf("submit IssueToken block minting %s? [y/N] ", result.ZTS))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	if err := PublishRawTransaction(client, result.Block); err != nil {
		return fmt.Errorf("publishing transaction: %w", err)
	}
	fmt.Println("submitted")
	return nil
}

func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptYesNo(prompt string) (bool, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}
