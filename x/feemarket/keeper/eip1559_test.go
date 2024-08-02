package keeper_test

import (
	"fmt"
	"math/big"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
)

func (suite *KeeperTestSuite) TestCalculateBaseFee() {
	testCases := []struct {
		name      string
		NoBaseFee bool
		malleate  func()
		expFee    *big.Int
	}{
		{
			"without BaseFee",
			true,
			func() {},
			nil,
		},
		{
			"with BaseFee - initial EIP-1559 block",
			false,
			func() {
				suite.ctx = suite.ctx.WithBlockHeight(0)
			},
			suite.app.FeeMarketKeeper.GetParams(suite.ctx).BaseFee.BigInt(),
		},
		{
			"with BaseFee - parent block used the same gas as its target",
			false,
			func() {
				// non initial block
				suite.ctx = suite.ctx.WithBlockHeight(1)

				// Set gas used
				suite.app.FeeMarketKeeper.SetBlockGasUsed(suite.ctx, 100)

				// Set target/gasLimit through Consensus Param MaxGas
				blockParams := cmtproto.BlockParams{
					MaxGas:   100,
					MaxBytes: 10,
				}
				consParams := cmtproto.ConsensusParams{Block: &blockParams}
				suite.ctx = suite.ctx.WithConsensusParams(consParams)

				// set ElasticityMultiplier
				params := suite.app.FeeMarketKeeper.GetParams(suite.ctx)
				params.ElasticityMultiplier = 1
				suite.app.FeeMarketKeeper.SetParams(suite.ctx, params)
			},
			suite.app.FeeMarketKeeper.GetParams(suite.ctx).BaseFee.BigInt(),
		},
		{
			"with BaseFee - parent block used more gas than its target",
			false,
			func() {
				suite.ctx = suite.ctx.WithBlockHeight(1)

				suite.app.FeeMarketKeeper.SetBlockGasUsed(suite.ctx, 200)

				blockParams := cmtproto.BlockParams{
					MaxGas:   100,
					MaxBytes: 10,
				}
				consParams := cmtproto.ConsensusParams{Block: &blockParams}
				suite.ctx = suite.ctx.WithConsensusParams(consParams)

				params := suite.app.FeeMarketKeeper.GetParams(suite.ctx)
				params.ElasticityMultiplier = 1
				suite.app.FeeMarketKeeper.SetParams(suite.ctx, params)
			},
			big.NewInt(1125000000),
		},
		{
			"with BaseFee - Parent gas used smaller than parent gas target",
			false,
			func() {
				suite.ctx = suite.ctx.WithBlockHeight(1)

				suite.app.FeeMarketKeeper.SetBlockGasUsed(suite.ctx, 50)

				blockParams := cmtproto.BlockParams{
					MaxGas:   100,
					MaxBytes: 10,
				}
				consParams := cmtproto.ConsensusParams{Block: &blockParams}
				suite.ctx = suite.ctx.WithConsensusParams(consParams)

				params := suite.app.FeeMarketKeeper.GetParams(suite.ctx)
				params.ElasticityMultiplier = 1
				suite.app.FeeMarketKeeper.SetParams(suite.ctx, params)
			},
			big.NewInt(937500000),
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest() // reset
			params := suite.app.FeeMarketKeeper.GetParams(suite.ctx)
			params.NoBaseFee = tc.NoBaseFee
			suite.app.FeeMarketKeeper.SetParams(suite.ctx, params)

			tc.malleate()

			fee := suite.app.FeeMarketKeeper.CalculateBaseFee(suite.ctx)
			if tc.NoBaseFee {
				suite.Require().Nil(fee, tc.name)
			} else {
				suite.Require().Equal(tc.expFee, fee, tc.name)
			}
		})
	}
}
