package interchaintest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"cosmossdk.io/math"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/docker/docker/client"
	"github.com/icza/dyno"
	"github.com/strangelove-ventures/interchaintest/v8"
	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos/wasm"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/relayer"
	"github.com/strangelove-ventures/interchaintest/v8/testreporter"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

const (
	votingPeriod     = "15s"
	maxDepositPeriod = "15s"

	// Chain and relayer version inf
	IBCRelayerImage     = "ghcr.io/cosmos/relayer"
	IBCRelayerVersion   = "latest"
	GaiaImageVersion    = "v21.0.0"
	OsmosisImageVersion = "v28.0.0"
)

var (
	repo, version = GetDockerImageInfo()

	oraiImage = ibc.DockerImage{
		Repository: repo,
		Version:    version,
		UidGid:     "1025:1025",
	}

	oraiConfig = ibc.ChainConfig{
		Type:                "cosmos",
		Name:                "orai",
		ChainID:             "orai-1",
		Images:              []ibc.DockerImage{oraiImage},
		Bin:                 "oraid",
		Bech32Prefix:        "orai",
		Denom:               "orai",
		CoinType:            "118",
		GasPrices:           "0.005orai",
		GasAdjustment:       10,
		TrustingPeriod:      "112h",
		NoHostMount:         false,
		ModifyGenesis:       modifyGenesisShortProposals(votingPeriod, maxDepositPeriod),
		ConfigFileOverrides: nil,
		EncodingConfig:      oraiEncoding(),
	}
	genesisWalletAmount = math.NewInt(100_000_000_000)
	amountToSend        = math.NewInt(1_000_000_000)

	pathOraiGaia = "IbcPathOraiGaia"
	pathOraiOsmo = "IbcPathOraiOsmo"
)

var (
	chainSpecs = map[string]*interchaintest.ChainSpec{
		"orai": {
			Name:        "orai",
			ChainConfig: oraiConfig,
		},
		"gaia": {
			Name:    "gaia",
			Version: GaiaImageVersion,
			ChainConfig: ibc.ChainConfig{
				GasPrices: "1uatom",
			},
		},
		"osmosis": {
			Name:    "osmosis",
			Version: OsmosisImageVersion,
			ChainConfig: ibc.ChainConfig{
				GasPrices: "1uosmo",
			},
		},
	}
)

// oraiEncoding registers the Orai specific module codecs so that the associated types and msgs
// will be supported when writing to the blocksdb sqlite database.
func oraiEncoding() *moduletestutil.TestEncodingConfig {
	cfg := wasm.WasmEncoding()

	return cfg
}

// GetDockerImageInfo returns the appropriate repo and branch version string for integration with the CI pipeline.
// The remote runner sets the BRANCH_CI env var. If present, interchaintest will use the docker image pushed up to the repo.
// If testing locally, user should run `make docker-build-debug` and interchaintest will use the local image.
func GetDockerImageInfo() (repo, version string) {
	branchVersion, found := os.LookupEnv("BRANCH_CI")
	if !found {
		// make local-image
		fmt.Println("Testing local image")
		repo = "orai"
		branchVersion = "debug"
	}

	// github converts / to - for pushed docker images
	branchVersion = strings.ReplaceAll(branchVersion, "/", "-")
	return repo, branchVersion
}

func modifyGenesisShortProposals(
	votingPeriod string,
	maxDepositPeriod string,
) func(ibc.ChainConfig, []byte) ([]byte, error) {
	return func(chainConfig ibc.ChainConfig, genbz []byte) ([]byte, error) {
		g := make(map[string]interface{})
		if err := json.Unmarshal(genbz, &g); err != nil {
			return nil, fmt.Errorf("failed to unmarshal genesis file: %w", err)
		}
		if err := dyno.Set(g, votingPeriod, "app_state", "gov", "params", "voting_period"); err != nil {
			return nil, fmt.Errorf("failed to set voting period in genesis json: %w", err)
		}
		if err := dyno.Set(g, "1", "initial_height"); err != nil {
			return nil, fmt.Errorf("failed to set initial height in genesis json: %w", err)
		}
		if err := dyno.Set(g, maxDepositPeriod, "app_state", "gov", "params", "max_deposit_period"); err != nil {
			return nil, fmt.Errorf("failed to set voting period in genesis json: %w", err)
		}
		if err := dyno.Set(g, chainConfig.Denom, "app_state", "gov", "params", "min_deposit", 0, "denom"); err != nil {
			return nil, fmt.Errorf("failed to set voting period in genesis json: %w", err)
		}
		out, err := json.Marshal(g)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal genesis bytes to json: %w", err)
		}
		return out, nil
	}
}

// CreateChains create testing chain. Currently we instantiate 2 chain, first is Orai, seconds is gaia
func CreateChains(t *testing.T, numVals, numFullNodes int, chainIds []string, opts ...func(*ibc.ChainConfig)) []ibc.Chain {
	for _, opt := range opts {
		opt(&oraiConfig)
	}

	var specs []*interchaintest.ChainSpec
	for _, chainId := range chainIds {
		if chainSpecs[chainId] == nil {
			fmt.Println("not supported this chain: ", chainId)
			continue
		}

		chainSpecs[chainId].NumValidators = &numVals
		chainSpecs[chainId].NumFullNodes = &numFullNodes

		specs = append(specs, chainSpecs[chainId])
	}

	cf := interchaintest.NewBuiltinChainFactory(zaptest.NewLogger(t), specs)

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)
	return chains
}

// CreateChain create only one testing chain, suitable for non-ibc testing logic -> faster
func CreateChain(t *testing.T, numVals, numFullNodes int, opts ...func(*ibc.ChainConfig)) []ibc.Chain {
	for _, opt := range opts {
		opt(&oraiConfig)
	}

	chainSpecs["orai"].NumFullNodes = &numFullNodes
	chainSpecs["orai"].NumValidators = &numVals

	cf := interchaintest.NewBuiltinChainFactory(zaptest.NewLogger(t), []*interchaintest.ChainSpec{
		chainSpecs["orai"],
	})

	// Get chains from the chain factory
	chains, err := cf.Chains(t.Name())
	require.NoError(t, err)
	return chains
}

func BuildInitialChainNoIbc(t *testing.T, chain ibc.Chain) (*interchaintest.Interchain, context.Context) {
	// Create a new Interchain object which describes the chains, relayers, and IBC connections we want to use
	ic := interchaintest.NewInterchain()
	ic = ic.AddChain(chain)
	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)
	ctx := context.Background()
	client, network := interchaintest.DockerSetup(t)
	err := ic.Build(ctx, eRep, interchaintest.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,
		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: interchaintest.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	return ic, ctx
}

func BuildInitialChain(t *testing.T, chains []ibc.Chain, ibcPath string) (*interchaintest.Interchain, ibc.Relayer, context.Context, *client.Client, *testreporter.RelayerExecReporter, string) {
	// Create a new Interchain object which describes the chains, relayers, and IBC connections we want to use
	require.Equal(t, len(chains), 2) // we only initial 2 chain for now
	ic := interchaintest.NewInterchain()

	for _, chain := range chains {
		ic = ic.AddChain(chain)
	}

	rep := testreporter.NewNopReporter()
	eRep := rep.RelayerExecReporter(t)

	// setupp relayer
	ctx := context.Background()
	client, network := interchaintest.DockerSetup(t)
	r := interchaintest.NewBuiltinRelayerFactory(
		ibc.CosmosRly,
		zaptest.NewLogger(t),
		relayer.CustomDockerImage(IBCRelayerImage, IBCRelayerVersion, "100:1000"),
	).Build(t, client, network)

	ic.AddRelayer(r, "rly").
		AddLink(interchaintest.InterchainLink{
			Chain1:  chains[0],
			Chain2:  chains[1],
			Relayer: r,
			Path:    ibcPath,
		})

	err := ic.Build(ctx, eRep, interchaintest.InterchainBuildOptions{
		TestName:         t.Name(),
		Client:           client,
		NetworkID:        network,
		SkipPathCreation: false,
		// This can be used to write to the block database which will index all block data e.g. txs, msgs, events, etc.
		// BlockDatabaseFile: interchaintest.DefaultBlockDatabaseFilepath(),
	})
	require.NoError(t, err)

	return ic, r, ctx, client, eRep, network
}
