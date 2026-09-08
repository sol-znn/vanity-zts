package main

import (
	"crypto/rand"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
	"github.com/zenon-network/go-zenon/pow"
)

// Template holds every field of the IssueToken send-block that is fixed
// before mining starts. Only Nonce and MomentumAcknowledged are varied by
// the search (see the package doc in README.md for why those two fields are
// free to pick without invalidating the block).
type Template struct {
	ChainIdentifier uint64 // must match the connected node's network id
	Address         types.Address
	PreviousHash    types.Hash
	Height          uint64
	Data            []byte
	FusedPlasma     uint64
	Difficulty      uint64
	Momentums       []types.HashHeight // candidate values for MomentumAcknowledged
}

// Build returns a fresh, unsigned account-block for the given nonce/momentum
// pair, with Hash left uncomputed.
func (t Template) build(nonce [8]byte, momentum types.HashHeight) *nom.AccountBlock {
	return &nom.AccountBlock{
		Version:              1,
		ChainIdentifier:      t.ChainIdentifier,
		BlockType:            nom.BlockTypeUserSend,
		PreviousHash:         t.PreviousHash,
		Height:               t.Height,
		MomentumAcknowledged: momentum,
		Address:              t.Address,
		ToAddress:            types.TokenContract,
		Amount:               tokenIssueAmount(),
		TokenStandard:        types.ZnnTokenStandard,
		Data:                 t.Data,
		FusedPlasma:          t.FusedPlasma,
		Difficulty:           t.Difficulty,
		Nonce:                nom.Nonce{Data: nonce},
	}
}

// Result is a winning candidate: a fully-formed (but unsigned) block whose
// resulting token standard matches the requested pattern.
type Result struct {
	Block *nom.AccountBlock
	Hash  types.Hash
	ZTS   types.ZenonTokenStandard
}

// Mine searches nonce/momentum combinations across workers goroutines until
// one produces a token standard matching pattern, or stop is closed.
func Mine(t Template, pattern Pattern, workers int, stop <-chan struct{}, attempts *uint64) *Result {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	var found atomic.Pointer[Result]
	done := make(chan struct{})
	var closeOnce sync.Once
	signalDone := func() { closeOnce.Do(func() { close(done) }) }

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var nonce [8]byte
			var local uint64
			for {
				select {
				case <-stop:
					atomic.AddUint64(attempts, local)
					return
				case <-done:
					atomic.AddUint64(attempts, local)
					return
				default:
				}

				if _, err := rand.Read(nonce[:]); err != nil {
					continue
				}
				momentum := t.Momentums[fastRandIndex(nonce, len(t.Momentums))]

				block := t.build(nonce, momentum)
				if t.Difficulty != 0 && !pow.CheckPoWNonce(block) {
					local++
					continue
				}

				hash := block.ComputeHash()
				zts := types.NewZenonTokenStandard(hash.Bytes())
				local++

				if local&0xFFF == 0 {
					atomic.AddUint64(attempts, local)
					local = 0
				}

				if pattern.Match(zts.String()) {
					block.Hash = hash
					found.Store(&Result{Block: block, Hash: hash, ZTS: zts})
					signalDone()
					atomic.AddUint64(attempts, local)
					return
				}
			}
		}()
	}
	wg.Wait()
	return found.Load()
}

func fastRandIndex(nonce [8]byte, n int) int {
	if n <= 1 {
		return 0
	}
	var v uint64
	for _, b := range nonce {
		v = v<<8 | uint64(b)
	}
	return int(v % uint64(n))
}
