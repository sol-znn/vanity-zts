package main

import (
	"testing"
	"time"

	"github.com/zenon-network/go-zenon/common/types"
)

func TestMineFindsMatchingPattern(t *testing.T) {
	template := Template{
		ChainIdentifier: 69,                   // e.g. devnet's network id; must end up on the mined block
		Address:         types.PillarContract, // any fixed address works for this offline test
		PreviousHash:    types.ZeroHash,
		Height:          1,
		Data:            []byte("test-issue-token-data"),
		FusedPlasma:     1000000,
		Difficulty:      0, // free nonce: fused plasma covers the call
		Momentums:       []types.HashHeight{{Hash: types.ZeroHash, Height: 1}},
	}

	pattern := Pattern{Prefix: "q"} // single-char prefix: should be found almost immediately

	stop := make(chan struct{})
	defer close(stop)
	var attempts uint64

	result := Mine(template, pattern, 4, stop, &attempts)
	if result == nil {
		t.Fatal("expected a match, got nil")
	}
	if !pattern.Match(result.ZTS.String()) {
		t.Fatalf("result %s does not match pattern %+v", result.ZTS, pattern)
	}

	// The block's own ComputeHash must reproduce the token standard, i.e.
	// the miner didn't return a stale/incorrect hash.
	recomputed := result.Block.ComputeHash()
	if recomputed != result.Hash {
		t.Fatalf("stored hash %s does not match recomputed hash %s", result.Hash, recomputed)
	}
	recomputedZTS := types.NewZenonTokenStandard(recomputed.Bytes())
	if recomputedZTS != result.ZTS {
		t.Fatalf("stored ZTS %s does not match recomputed ZTS %s", result.ZTS, recomputedZTS)
	}

	if result.Block.ChainIdentifier != template.ChainIdentifier {
		t.Fatalf("block has chain identifier %d, want %d (the node would reject this with a network-id mismatch)", result.Block.ChainIdentifier, template.ChainIdentifier)
	}
}

func TestMineRespectsStop(t *testing.T) {
	template := Template{
		Address:      types.PillarContract,
		PreviousHash: types.ZeroHash,
		Height:       1,
		Data:         []byte("test"),
		FusedPlasma:  1000000,
		Difficulty:   0,
		Momentums:    []types.HashHeight{{Hash: types.ZeroHash, Height: 1}},
	}
	// A pattern long enough that it's very unlikely to be found before stop fires.
	pattern := Pattern{Prefix: "qpzry9x8gf2tvdw0"}

	stop := make(chan struct{})
	var attempts uint64
	done := make(chan *Result, 1)
	go func() { done <- Mine(template, pattern, 2, stop, &attempts) }()

	time.Sleep(50 * time.Millisecond)
	close(stop)

	select {
	case result := <-done:
		if result != nil {
			t.Fatalf("did not expect a match this quickly, got %s", result.ZTS)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Mine did not stop promptly after stop was closed")
	}
}

func TestPatternMatch(t *testing.T) {
	p := Pattern{Prefix: "ab", Suffix: "yz", Contains: "mm"}
	if !p.Match("zts1abcmmxyz") {
		t.Fatal("expected match")
	}
	if p.Match("zts1abcxxxyz") {
		t.Fatal("expected no match: missing contains")
	}
	if p.Match("zts1zzcmmxyz") {
		t.Fatal("expected no match: wrong prefix")
	}
}

func TestValidateBech32Rejects(t *testing.T) {
	if err := ValidateBech32("abc1"); err == nil {
		t.Fatal("expected error for '1', which is excluded from the bech32 charset")
	}
	if err := ValidateBech32("qpzry"); err != nil {
		t.Fatalf("unexpected error for valid charset: %v", err)
	}
}
