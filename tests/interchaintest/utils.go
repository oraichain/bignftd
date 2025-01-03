package interchaintest

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/strangelove-ventures/interchaintest/v8"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	paramsutils "github.com/cosmos/cosmos-sdk/x/params/client/utils"
)

func CreateTestingUser(
	t *testing.T,
	ctx context.Context,
	keyNamePrefix string,
	amount math.Int,
	chains ...ibc.Chain,
) []ibc.Wallet {
	// Create some user accounts on both chains
	users := interchaintest.GetAndFundTestUsers(t, ctx, t.Name(), amount, chains...)
	// Wait a few blocks for user accounts to be created
	err := testutil.WaitForBlocks(ctx, 5, chains[0])
	require.NoError(t, err)

	return users
}

func CreateParamChangeProposal(
	title, des, ss, key string,
	depAmount sdk.Coin,
	value []byte,
) *paramsutils.ParamChangeProposalJSON {
	return &paramsutils.ParamChangeProposalJSON{
		Title:       title,
		Description: des,
		Changes: paramsutils.ParamChangesJSON{
			paramsutils.ParamChangeJSON{
				Subspace: ss,
				Key:      key,
				Value:    value,
			},
		},
		Deposit: depAmount.String(),
	}
}
