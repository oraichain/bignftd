package helpers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/strangelove-ventures/interchaintest/v8/chain/cosmos"
	"github.com/strangelove-ventures/interchaintest/v8/ibc"
	"github.com/strangelove-ventures/interchaintest/v8/testutil"
	"github.com/stretchr/testify/require"

	paramsutils "github.com/cosmos/cosmos-sdk/x/params/client/utils"
)

// Tx helpers
func TxTokenFactoryCreateDenom(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user ibc.Wallet,
	denomName string,
	gas uint64,
) (string, string) {
	tn := chain.GetNode()
	denom, txHash, err := tn.TokenFactoryCreateDenom(ctx, user, denomName, gas)
	require.NoError(t, err)

	return denom, txHash
}

func TxTokenFactoryMintToken(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user ibc.Wallet,
	denom string,
	amount uint64,
) string {
	tn := chain.GetNode()
	txHash, err := tn.TokenFactoryMintDenom(ctx, user.KeyName(), denom, amount)
	require.NoError(t, err)

	return txHash
}

func TxTokenFactoryForceTransfer(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user ibc.Wallet,
	denom string,
	amount uint64,
	fromAddr string,
	toAddr string,
) (string, error) {
	tn := chain.GetNode()
	txHash, err := tn.TokenFactoryForceTransferDenom(ctx, user.KeyName(), denom, amount, fromAddr, toAddr)

	return txHash, err
}

func TxTokenFactoryModifyMetadata(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user ibc.Wallet,
	denom, ticker, des string,
	exponent, gas uint64,
) (string, error) {
	tn := chain.GetNode()
	cmd := []string{"tokenfactory", "modify-metadata", denom, ticker, des, strconv.FormatUint(exponent, 10)}

	if gas != 0 {
		cmd = append(cmd, "--gas", strconv.FormatUint(gas, 10))
	}

	txHash, err := tn.ExecTx(ctx, user.KeyName(), cmd...)
	if err != nil {
		return "", err
	}

	return txHash, nil
}

// Proposal helpers
// ParamChangeProposal submits a param change proposal to the chain, signed by keyName.
func ProposalSubmitTokenFactoryParamChange(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	admin ibc.Wallet,
	prop *paramsutils.ParamChangeProposalJSON,
) (propId uint64, _ error) {
	tn := chain.GetNode()

	content, err := json.Marshal(prop)
	if err != nil {
		return 0, err
	}

	hash := sha256.Sum256(content)
	proposalFilename := fmt.Sprintf("%x.json", hash)
	err = tn.WriteFile(ctx, content, proposalFilename)
	if err != nil {
		return 0, fmt.Errorf("writing param change proposal: %w", err)
	}

	proposalPath := filepath.Join(tn.HomeDir(), proposalFilename)

	command := []string{
		"tokenfactory", "param-change", proposalPath,
	}

	txHash, err := tn.ExecTx(ctx, admin.KeyName(), command...)
	if err != nil {
		return 0, fmt.Errorf("failed to submit param change proposal: %w", err)
	}

	err = testutil.WaitForBlocks(ctx, 2, chain)
	require.NoError(t, err)

	txProposal, err := txProposal(chain, txHash)
	if err != nil {
		return 0, fmt.Errorf("failed to parse tx proposal information: %w", err)
	}

	propId, err = strconv.ParseUint(txProposal.ProposalID, 10, 64)
	require.NoError(t, err, "failed to convert proposal ID to uint64")

	return propId, nil
}

// Query helpers
// QueryParam returns the state and details of a subspace param.
func QueryTokenFactoryParam(t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
) (TokenFactoryParams, error) {
	tn := chain.GetNode()
	stdout, _, err := tn.ExecQuery(ctx, "tokenfactory", "params")
	if err != nil {
		return TokenFactoryParams{}, err
	}
	var param QueryTokenFactoryParamsResponse
	err = json.Unmarshal(stdout, &param)
	if err != nil {
		return TokenFactoryParams{}, err
	}
	return param.Params, nil
}

// QueryDenomsFromCreator returns all the denom that user created.
func QueryDenomsFromCreator(t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	user string,
) ([]string, error) {
	tn := chain.GetNode()
	stdout, _, err := tn.ExecQuery(ctx, "tokenfactory", "denoms-from-creator", user)
	if err != nil {
		return nil, err
	}
	var res QueryDenomsFromCreatorResponse
	err = json.Unmarshal(stdout, &res)
	if err != nil {
		return nil, err
	}

	return res.Denoms, nil
}

func QueryDenomAuthorityMetadata(t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	denom string,
) (string, error) {
	tn := chain.GetNode()
	stdout, _, err := tn.ExecQuery(ctx, "tokenfactory", "denom-authority-metadata", denom)
	if err != nil {
		return "", err
	}
	var res QueryDenomAuthorityMetadataResponse
	err = json.Unmarshal(stdout, &res)
	if err != nil {
		return "", err
	}

	return res.AuthorityMetadata.Admin, nil
}

func QueryBalance(
	t *testing.T,
	ctx context.Context,
	chain *cosmos.CosmosChain,
	denom string,
	userAddress string,
) (uint64, error) {
	tn := chain.GetNode()
	balance, err := tn.Chain.GetBalance(ctx, userAddress, denom)
	if err != nil {
		return 0, err
	}

	return balance.Uint64(), nil
}
