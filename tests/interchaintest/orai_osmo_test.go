package interchaintest

import (
	"context"
	"fmt"
	"testing"

	// "github.com/strangelove-ventures/interchaintest/v8"
	// "github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	// "github.com/strangelove-ventures/interchaintest/v8/ibc"
	// "github.com/strangelove-ventures/interchaintest/v8/relayer"
	// "github.com/strangelove-ventures/interchaintest/v8/testreporter"
	"github.com/stretchr/testify/require"
	// "go.uber.org/zap/zaptest"
)

// TestStartOrai is a basic test to assert that spinning up a Orai network with 1 validator works properly.
func TestOraiOsmoIbc(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	ctx := context.Background()

	chains := CreateChains(t, 1, 1, []string{"orai", "osmosis"})

	// orai, osmo := chains[0].(*cosmos.CosmosChain), chains[1].(*cosmos.CosmosChain)

	// Create relayer factory to utilize the go-relayer
	_, r, ctx, _, eRep, _ := BuildInitialChain(t, chains)

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
}
