# CHANGELOG

## v0.50.2

<!--
    Add a summary for the release here.

    If you don't change this message, or if this file is empty, the release
    will not be created. -->

Upgrade Oraichain mainnet to v0.50.2 to fix some IBC, cosmos sdk and iavl bugs.

### BUG FIXES

- Failed ChanOpenInit ChannelSide: channel open init
  callback failed for port ID: cannot claim nil capability
  ([\#34](https://github.com/oraichain/wasmd/issues/34))
- Failed to prune store err=Value missing for key
  ([\#27](https://github.com/oraichain/wasmd/issues/27))
- No cosmos.msg.v1.signer option found for message
  cosmwasm.tokenfactory.v1beta1.MsgSetDenomMetadata
  ([\#33](https://github.com/oraichain/wasmd/issues/33))

## v0.50.1

<!--
    Add a summary for the release here.

    If you don't change this message, or if this file is empty, the release
    will not be created. -->

Upgrade Oraichain mainnet to v0.50.1 to fix some IBC and cosmos sdk bugs.

### BUG FIXES

- Can't broadcast certain transactions (eg: `slashing unjail`) when using oraid CLI
  ([\#30](https://github.com/oraichain/wasmd/issues/30))
- Cannot use IBC Hooks
  ([\#28](https://github.com/oraichain/wasmd/issues/28))
- ICA module caller does not own capability for channel, port ID
  ([\#29](https://github.com/oraichain/wasmd/issues/29))

## v0.50.0

<!--
    Add a summary for the release here.

    If you don't change this message, or if this file is empty, the release
    will not be created. -->

Upgrade Oraichain mainnet to v0.50.0 with Cosmos SDK v0.50.10, CometBFT 0.38.12 and more. This upgrade allows Oraichain to apply strategic features, improve performances and user 
experiences.

### BREAKING CHANGE

- ABCI 2.0, inheriting numerous new features like: IAVL enhancing
  performance; ABCI Optimistic Execution reducing block time,
  custom mempools allowing private transactions, front-running,...
  ([\#19](https://github.com/oraichain/wasmd/issues/19))
- IAVL 1.0 enhances performance
  ([\#19](https://github.com/oraichain/wasmd/issues/19))
- Stop supporting cosmwasm contracts version <= 0.13.x
  ([\#23](https://github.com/oraichain/wasmd/issues/23))
- Stop using `evmutil` module ([\#25](https://github.com/oraichain/wasmd/issues/25))
- Stop using `intertx` module
  ([\#24](https://github.com/oraichain/wasmd/issues/24))
- Upgrade Oraichain mainnet to Cosmos SDK 0.50.10 with module
  enhancements like gov module emergency proposals; advanced IBC features
  ([\#19](https://github.com/oraichain/wasmd/issues/19))

### BUG FIXES

- Decrease inflation max to be equal to min when upgrading
  ([\#17](https://github.com/oraichain/wasmd/issues/17))
- Support reading old cosmwasm proposals by adding backward compatible logic
  ([\#16](https://github.com/oraichain/wasmd/issues/16))

### IMPROVEMENTS

- Drastically reduce load time when running a forked node using a heavy genesis
  state. Also, significantly reduce memory consumption when loading a large
  genesis file ([\#20](https://github.com/oraichain/wasmd/issues/20))

---

Oraichain Wasmd is a fork of [Wasmd](https://github.com/cosmwasm/wasmd) as of October 2024.

## CHANGELOG

Read our [README](../README.md) for mor information
