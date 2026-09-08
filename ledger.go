package main

import (
	"github.com/zenon-network/go-zenon/chain/nom"
	"github.com/zenon-network/go-zenon/common/types"
)

// GetFrontierAccountBlock returns the latest confirmed account-block for
// address, or nil if the account has never sent/received anything.
func GetFrontierAccountBlock(c *Client, address types.Address) (*nom.AccountBlock, error) {
	var block *nom.AccountBlock
	if err := c.Call("ledger.getFrontierAccountBlock", []interface{}{address.String()}, &block); err != nil {
		return nil, err
	}
	return block, nil
}

// GetFrontierMomentum returns the current momentum-chain tip.
func GetFrontierMomentum(c *Client) (*nom.Momentum, error) {
	var momentum *nom.Momentum
	if err := c.Call("ledger.getFrontierMomentum", []interface{}{}, &momentum); err != nil {
		return nil, err
	}
	return momentum, nil
}

type momentumList struct {
	List  []*nom.Momentum `json:"list"`
	Count int             `json:"count"`
}

// GetMomentumsByHeight returns count confirmed momentums starting at height.
func GetMomentumsByHeight(c *Client, height, count uint64) ([]*nom.Momentum, error) {
	var list momentumList
	if err := c.Call("ledger.getMomentumsByHeight", []interface{}{height, count}, &list); err != nil {
		return nil, err
	}
	return list.List, nil
}

// getRequiredParam mirrors rpc/api/embedded.GetRequiredParam's wire format.
// Re-declared locally so we don't have to pull in the (heavy) rpc/api/embedded
// package, which transitively imports the whole node.
type getRequiredParam struct {
	Address   types.Address  `json:"address"`
	BlockType uint64         `json:"blockType"`
	ToAddress *types.Address `json:"toAddress"`
	Data      []byte         `json:"data"`
}

type getRequiredResult struct {
	AvailablePlasma    uint64 `json:"availablePlasma"`
	BasePlasma         uint64 `json:"basePlasma"`
	RequiredDifficulty uint64 `json:"requiredDifficulty"`
}

// GetRequiredPoWForAccountBlock asks the node how much of the sender's
// already-available plasma covers this call, and if it doesn't, what PoW
// difficulty is required to make up the difference.
func GetRequiredPoWForAccountBlock(c *Client, address types.Address, blockType uint64, toAddress types.Address, data []byte) (*getRequiredResult, error) {
	param := getRequiredParam{
		Address:   address,
		BlockType: blockType,
		ToAddress: &toAddress,
		Data:      data,
	}
	var result getRequiredResult
	if err := c.Call("embedded.plasma.getRequiredPoWForAccountBlock", []interface{}{param}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PublishRawTransaction submits a fully signed account-block to the node.
func PublishRawTransaction(c *Client, block *nom.AccountBlock) error {
	return c.Call("ledger.publishRawTransaction", []interface{}{block}, nil)
}

type accountBlockList struct {
	List  []*nom.AccountBlock `json:"list"`
	Count int                 `json:"count"`
	More  bool                `json:"more"`
}

// GetUnreceivedBlocksByAddress lists incoming send-blocks address has not
// yet claimed with a matching receive-block.
func GetUnreceivedBlocksByAddress(c *Client, address types.Address, pageIndex, pageSize uint32) ([]*nom.AccountBlock, error) {
	var list accountBlockList
	if err := c.Call("ledger.getUnreceivedBlocksByAddress", []interface{}{address.String(), pageIndex, pageSize}, &list); err != nil {
		return nil, err
	}
	return list.List, nil
}
