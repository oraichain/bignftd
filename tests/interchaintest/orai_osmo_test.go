package interchaintest

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	transfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	"github.com/oraichain/wasmd/tests/interchaintest/helpers"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"

	"github.com/stretchr/testify/require"
)

// TestStartOrai is a basic test to assert that spinning up a Orai network with 1 validator works properly.
func TestOraiOsmoIbc(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	ctx := context.Background()

	chains := CreateChains(t, 1, 1, []string{"orai", "osmosis"})
	orai, osmo := chains[0].(*cosmos.CosmosChain), chains[1].(*cosmos.CosmosChain)

	// Create relayer factory to utilize the go-relayer
	ic, r, ctx, _, eRep, _ := BuildInitialChain(t, chains, pathOraiOsmo)
	t.Cleanup(func() {
		_ = ic.Close()
	})

	// Start the relayer
	require.NoError(t, r.StartRelayer(ctx, eRep, pathOraiOsmo))
	t.Cleanup(
		func() {
			err := r.StopRelayer(ctx, eRep)
			if err != nil {
				panic(fmt.Errorf("an error occurred while stopping the relayer: %s", err))
			}
		},
	)

	channel, err := ibc.GetTransferChannel(ctx, r, eRep, orai.Config().ChainID, osmo.Config().ChainID)
	require.NoError(t, err)

	users := CreateTestingUser(t, ctx, t.Name(), genesisWalletAmount, chains...)
	// Get our Bech32 encoded user addresses
	oraiUser, osmoUser := users[0], users[1]

	oraiUserAddress := sdk.MustBech32ifyAddressBytes(orai.Config().Bech32Prefix, oraiUser.Address())
	osmoUserAddr := sdk.MustBech32ifyAddressBytes(osmo.Config().Bech32Prefix, osmoUser.Address())

	_ = oraiUserAddress
	_ = osmoUserAddr
	gas := uint64(100_000_000)

	// create new token factory denom
	expectedDenom, _ := helpers.TxTokenFactoryCreateDenom(t, ctx, orai, oraiUser, "orai-usd", gas)
	denomCreated, err := helpers.QueryDenomsFromCreator(t, ctx, orai, oraiUserAddress)
	require.NoError(t, err)
	require.Contains(t, denomCreated, expectedDenom)

	// mint token
	tokenToMint := uint64(100_000_000_000)
	_ = helpers.TxTokenFactoryMintToken(t, ctx, orai, oraiUser, expectedDenom, tokenToMint)
	oraiUserBalance, err := helpers.QueryBalance(t, ctx, orai, expectedDenom, oraiUserAddress)
	require.NoError(t, err)
	require.Equal(t, tokenToMint, oraiUserBalance)

	// get escrowed address
	addr := types.GetEscrowAddress(channel.PortID, channel.ChannelID)
	escrowedAddress := sdk.MustBech32ifyAddressBytes(orai.Config().Bech32Prefix, addr.Bytes())

	// balance before transfer ibc must be 0
	escrowedBalance, err := helpers.QueryBalance(t, ctx, orai, expectedDenom, escrowedAddress)
	require.NoError(t, err)
	require.Equal(t, escrowedBalance, uint64(0))

	// ibc denom when transfer orai to osmosis
	// transfer/channel-0/factory/orai14zqwen0pqj7s6drrkwaqwded7ajrq5czyw7fhq/orai-usd
	oraiDenom := transfertypes.GetPrefixedDenom(channel.Counterparty.PortID, channel.Counterparty.ChannelID, expectedDenom)
	// ibc/F859A4CC5A5EA6533657F6A83F7C11A479A13DBDC53F68135CDA95B0F12E5892
	oraiIBCDenom := transfertypes.ParseDenomTrace(oraiDenom).IBCDenom()

	// osmosis user balance before transfer ibc must be 0
	userOsmosisBalance, err := helpers.QueryBalance(t, ctx, osmo, oraiIBCDenom, osmoUserAddr)
	require.NoError(t, err)
	require.Equal(t, userOsmosisBalance, uint64(0))

	// try to transfer token factory to osmosis
	transfer := ibc.WalletAmount{
		Address: osmoUserAddr,
		Denom:   expectedDenom,
		Amount:  amountToSend,
	}
	transferTx, err := orai.SendIBCTransfer(ctx, channel.Counterparty.ChannelID, oraiUserAddress, transfer, ibc.TransferOptions{})
	require.NoError(t, err)

	// waiting for ACK -> transfer successfully
	oraiHeight, err := orai.Height(ctx)
	require.NoError(t, err)
	_, err = testutil.PollForAck(ctx, orai, oraiHeight-5, oraiHeight+25, transferTx.Packet)
	require.NoError(t, err)

	// balance after transfer ibc must be equalt amount to send
	escrowedBalance, err = helpers.QueryBalance(t, ctx, orai, expectedDenom, escrowedAddress)
	fmt.Println("escrowed balance: ", escrowedBalance)
	require.NoError(t, err)
	require.Equal(t, escrowedBalance, uint64(amountToSend.Int64()))

	// osmosis user balance after transfer ibc must be equal amount to send
	userOsmosisBalance, err = helpers.QueryBalance(t, ctx, osmo, oraiIBCDenom, osmoUserAddr)
	require.NoError(t, err)
	require.Equal(t, userOsmosisBalance, uint64(amountToSend.Int64()))

	// try to force transfer tokenfactory from escrowed address
	_ = helpers.TxTokenFactoryForceTransfer(t, ctx, orai, oraiUser, expectedDenom, uint64(amountToSend.Int64()), escrowedAddress, oraiUserAddress)
	escrowedBalance, err = helpers.QueryBalance(t, ctx, orai, expectedDenom, escrowedAddress)
	fmt.Println("escrowed balance: ", escrowedBalance)
	require.NoError(t, err)
	require.Equal(t, escrowedBalance, uint64(amountToSend.Int64()))
}
