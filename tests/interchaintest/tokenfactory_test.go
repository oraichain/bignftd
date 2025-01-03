package interchaintest

import (
	"encoding/json"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/oraichain/wasmd/tests/interchaintest/helpers"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/stretchr/testify/require"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
)

// TestStartOrai is a basic test to assert that spinning up a Orai network with 1 validator works properly.
func TestTokenfactoryParamChange(t *testing.T) {
	// set up testing env
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()
	chains := CreateChains(t, 1, 1, []string{"orai", "gaia"})
	orai := chains[0].(*cosmos.CosmosChain)
	ic, _, ctx, _, _, _ := BuildInitialChain(t, chains, pathOraiGaia)
	t.Cleanup(func() {
		_ = ic.Close()
	})
	users := CreateTestingUser(t, ctx, t.Name(), genesisWalletAmount, chains...)
	oraiUser := users[0] // orai is chains[0] so oraiUser should be in slot 0

	// create param change proposal
	paramChangeValue := sdk.NewCoins(sdk.NewInt64Coin("orai", 10_000_000))
	rawValue, err := json.Marshal(paramChangeValue)
	require.NoError(t, err)
	paramChange := CreateParamChangeProposal(".", ".", "tokenfactory", "DenomCreationFee", sdk.NewCoin("orai", math.NewInt(1000000000)), rawValue)

	// submit param change token factory proposal
	propId, err := helpers.ProposalSubmitTokenFactoryParamChange(t, ctx, orai, oraiUser, paramChange)
	require.NoError(t, err, "error submitting param change proposal tx")

	// vote on created proposal and waiting for passed proposal
	err = orai.VoteOnProposalAllValidators(ctx, propId, cosmos.ProposalVoteYes)
	require.NoError(t, err, "failed to submit votes")
	height, _ := orai.Height(ctx)
	_, err = cosmos.PollForProposalStatus(ctx, orai, height, height+10, propId, govv1beta1.StatusPassed)
	require.NoError(t, err, "proposal status did not change to passed in expected number of blocks")

	// check result
	newParam, err := helpers.QueryTokenFactoryParam(t, ctx, orai)
	require.NoError(t, err)
	require.Equal(t, paramChangeValue, newParam.DenomCreationFee)
}

func TestTokenfactorySetMetadata(t *testing.T) {
	// set up testing env
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()
	chains := CreateChain(t, 1, 1)
	orai := chains[0].(*cosmos.CosmosChain)
	ic, ctx := BuildInitialChainNoIbc(t, orai)
	t.Cleanup(func() {
		_ = ic.Close()
	})
	users := CreateTestingUser(t, ctx, t.Name(), genesisWalletAmount, chains...)
	oraiUser := users[0]

	// create new token
	expectedDenom, _ := helpers.TxTokenFactoryCreateDenom(t, ctx, orai, oraiUser, "usd", 100_000_000)
	denomCreated, err := helpers.QueryDenomsFromCreator(t, ctx, orai, oraiUser.FormattedAddress())
	require.NoError(t, err)
	require.Contains(t, denomCreated, expectedDenom)

	authorityAdmin, err := helpers.QueryDenomAuthorityMetadata(t, ctx, orai, expectedDenom)
	require.NoError(t, err)
	require.Equal(t, oraiUser.FormattedAddress(), authorityAdmin)

	// set denom metadata
	ticker := "TICKER"
	desc := "desc"
	exponent := 6

	expectedMetadata := banktypes.Metadata{
		Description: desc,
		Display:     expectedDenom,
		Symbol:      ticker,
		Name:        expectedDenom,
		DenomUnits: []*banktypes.DenomUnit{
			{
				Denom:    expectedDenom,
				Exponent: 0, // must be 0 for the base denom
				Aliases:  []string{ticker},
			},
			{
				Denom:    ticker,
				Exponent: uint32(exponent),
				Aliases:  []string{expectedDenom},
			},
		},
		Base: expectedDenom,
	}

	// case 1: unauthorized
	// fixture
	unauthorizedWallet := CreateTestingUser(t, ctx, "unauthorized", genesisWalletAmount, chains...)
	unauthorizedUser := unauthorizedWallet[0]
	// test
	_, err = helpers.TxTokenFactoryModifyMetadata(t, ctx, orai, unauthorizedUser, expectedDenom, ticker, desc, uint64(exponent), 1000000)
	require.Error(t, err)

	// case 2: happy case
	_, err = helpers.TxTokenFactoryModifyMetadata(t, ctx, orai, oraiUser, expectedDenom, ticker, desc, uint64(exponent), 1000000)
	require.NoError(t, err)

	// check denom meta data
	metadata, err := orai.BankQueryDenomMetadata(ctx, expectedDenom)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	require.Equal(t, expectedMetadata, *metadata)
}
