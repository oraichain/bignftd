package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	"github.com/ethereum/go-ethereum/common"
)

const (
	// ModuleName string name of module
	ModuleName = "evm"

	// StoreKey key for ethereum storage data, account code (StateDB) or block
	// related data for Web3.
	// The EVM module should use a prefix store.
	StoreKey = ModuleName

	// TransientKey is the key to access the EVM transient store, that is reset
	// during the Commit phase.
	TransientKey = "transient_" + ModuleName

	// RouterKey uses module name for routing
	RouterKey = ModuleName
)

// prefix bytes for the EVM persistent store
const (
	prefixCode = iota + 1
	prefixStorage
)

// prefix bytes for the EVM transient store
const (
	prefixTransientBloom = iota + 1
	prefixTransientTxIndex
	prefixTransientLogSize
	prefixTransientGasUsed
	prefixEvmAddressMappingStoreKey
	prefixCosmosAddressMappingStoreKey
)

// KVStore key prefixes
var (
	KeyPrefixCode    = []byte{prefixCode}
	KeyPrefixStorage = []byte{prefixStorage}
)

// Transient Store key prefixes
var (
	KeyPrefixTransientBloom   = []byte{prefixTransientBloom}
	KeyPrefixTransientTxIndex = []byte{prefixTransientTxIndex}
	KeyPrefixTransientLogSize = []byte{prefixTransientLogSize}
	KeyPrefixTransientGasUsed = []byte{prefixTransientGasUsed}
)

// evm mapping key prefixes
var (
	EvmAddressMappingStoreKeyPrefix    = []byte{prefixEvmAddressMappingStoreKey}
	CosmosAddressMappingStoreKeyPrefix = []byte{prefixCosmosAddressMappingStoreKey}
)

// AddressStoragePrefix returns a prefix to iterate over a given account storage.
func AddressStoragePrefix(address common.Address) []byte {
	return append(KeyPrefixStorage, address.Bytes()...)
}

// StateKey defines the full key under which an account state is stored.
func StateKey(address common.Address, key []byte) []byte {
	return append(AddressStoragePrefix(address), key...)
}

// AccountStoreKey turns an address to a key used to get the account from the store
func EvmAddressMappingStoreKey(cosmosAddress sdk.AccAddress) []byte {
	return append(EvmAddressMappingStoreKeyPrefix, address.MustLengthPrefix(cosmosAddress)...)
}

// AccountStoreKey turns an address to a key used to get the account from the store
func CosmosAddressMappingStoreKey(evmAddress common.Address) []byte {
	return append(CosmosAddressMappingStoreKeyPrefix, address.MustLengthPrefix(evmAddress.Bytes())...)
}