# vanity-zts

Mine a Zenon token-standard address (`zts1...`) with a chosen
prefix/suffix/substring, the same way
[znn-address-generator](https://github.com/sol-znn/znn-address-generator) mines a vanity `z1...`
account address.

## How a ZTS is actually chosen

A token's ZTS is `NewZenonTokenStandard(sendBlockHash)` — the first 10 bytes
of the SHA3-256 hash of the `IssueToken` send-block that creates it
(`common/types/tokenstandard.go`, `vm/embedded/implementation/token.go` in
go-zenon). The block hash covers every field of that send-block: your
address, the previous block on your account-chain, the momentum you
acknowledge, the ABI-encoded token metadata, and the block's `Nonce`
(`chain/nom/account_block.go:ComputeHash`).

Two of those fields turn out to be free to pick, not fixed by chain state:

- **Nonce.** A send-block only needs a real proof-of-work nonce when
  `Difficulty != 0`; if the difficulty is `0` the check is skipped outright
  (`verifier/account_block.go:pow`). The difficulty is `0` whenever your
  account already has enough fused plasma to cover the call — normal wallets
  just send `0000000000000000` in that case, but *any* 8 bytes are equally
  valid. That's a full 2^64 nonce values to search, no proof-of-work
  required, entirely offline.
- **MomentumAcknowledged.** The verifier only checks that the named momentum
  exists on chain (`verifier/account_block.go:momentumAcknowledged`) — it
  does not have to be the current frontier. Any recently confirmed momentum
  height works, which is an extra, free search dimension on top of the
  nonce.

So mining a vanity ZTS means: fetch your account's frontier and a window of
recent momentums once, then try random `(nonce, momentum)` pairs in memory,
computing the resulting block hash and checking the encoded ZTS against your
pattern — completely offline, no PoW, no repeated network round-trips.
Only once a match is found do you sign and broadcast the single winning
block.

If your account does *not* have enough fused plasma, the node instead
returns a required PoW `difficulty`. The nonce then has to additionally pass
`pow.CheckPoWNonce`, which this tool checks for you — the search still
works, it's just slower (roughly `1/difficulty` of random nonces qualify, so
combine that with your pattern's odds).

## Usage

```bash
go build -o vanity-zts .

./vanity-zts \
  -keystore ~/.znn/wallet/my-wallet.json \
  -prefix cafe \
  -name My-Token -symbol MTK \
  -total-supply 100000000000 -decimals 8
```

By default this only *finds* a match, signs it, and prints the resulting
block as JSON — it does not touch the network beyond reading your account
frontier and the current momentum window. Pass `-submit` to actually
broadcast the winning block once found:

```bash
./vanity-zts -keystore ... -prefix cafe -name My-Token -symbol MTK \
  -total-supply 100000000000 -submit
```

### Flags

| Flag | Meaning |
| --- | --- |
| `-prefix` / `-suffix` / `-contains` | pattern to match against the address body right after `zts1` (at least one required) |
| `-keystore` | path to a Syrius / znn_cli keystore file |
| `-password` | keystore password (prompted if omitted) |
| `-index` | account index to derive (default 0) |
| `-url` | node RPC endpoint, default `http://127.0.0.1:35997` |
| *(no flag)* | the block's network id (`ChainIdentifier`) is read from the connected node's frontier momentum and never needs to be set manually — a mismatch here (e.g. mining against devnet's id 69 while pointed at mainnet's id 1) is what a `different network Id` error from `publishRawTransaction` means |
| `-workers` | mining goroutines, default: number of CPUs |
| `-momentum-window` | also vary `MomentumAcknowledged` across this many recent momentums (default 1: just the current frontier) |
| `-name`, `-symbol`, `-domain`, `-total-supply`, `-max-supply`, `-decimals`, `-mintable`, `-burnable`, `-utility` | `IssueToken` parameters — fixed inputs, not part of the search |
| `-submit` | broadcast once a match is found (default: dry run) |
| `-yes` | skip the confirmation prompt before submitting |
| `-limits` | print the protocol's token-parameter limits (name/symbol/domain format, max decimals, max supply, issue fee) and exit |

Parameters are validated locally against the exact same rules go-zenon's
embedded token contract enforces (`vm/embedded/implementation/token.go:checkToken`)
before any mining starts, so a typo in `-symbol` or an out-of-range
`-decimals` fails immediately instead of after a long search or a rejected
broadcast. Run `./vanity-zts -limits` to see the current limits, notably:

- **max supply**: `2^255-1` = `57896044618658097711785492504343953926634992332820282019728792003956564819967`
- **decimals**: 0-18
- a non-mintable token must have `total-supply == max-supply`; pass `-mintable` if you want to mint more later

### Receiving the minted tokens

`IssueToken` only creates the token and mints `total-supply` of it into the
*token contract's* balance; the contract then auto-generates its own
send-block (`TokenContract` → you, for `total-supply`) as a side effect of
processing your call (`vm/embedded/implementation/token.go:IssueMethod.ReceiveBlock`).
That send-block just sits as an unreceived block on your account until you
claim it with an ordinary receive transaction — this is true for every
account-to-account transfer on Zenon, not something specific to vanity
mining, but it's easy to miss since wallets normally auto-receive for you.
This tool has no receive support; use any wallet (Syrius, znn_cli, ...) to
receive it once the `IssueToken` block you submitted is confirmed.

### Pattern length and cost

The bech32 data alphabet has 32 symbols (`qpzry9x8gf2tvdw0s3jn54khce6mua7l`
— note `1`, `b`, `i`, `o` don't appear), so an anchored (`-prefix`/`-suffix`)
pattern of length *n* takes on the order of 32^n attempts; `-contains`
patterns are cheaper since they can start anywhere. At millions of attempts
per second per core (SHA3-256 over ~150 bytes, no PoW needed in the common
case), 4-5 anchored characters is quick; much beyond that gets slow fast.

## Why this needed its own tool

`znn_sdk_dart` / `znn.ts` always hardcode the nonce to
`0000000000000000` when plasma is fused (see
`znn.ts/lib/src/utils/block.ts:_setDifficulty`) — there's no supported way to
ask an existing wallet or CLI for a different one, let alone search across
many. This tool builds the exact same block those SDKs would build, but
searches the (nonce, momentum) space that the protocol always allowed to
vary, before signing and submitting the one block you actually want.
