package tx

import (
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/CosmWasm/wasmd/app/params"
	"github.com/CosmWasm/wasmd/indexer"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/libs/pubsub/query/syntax"
	cometbftindexer "github.com/cometbft/cometbft/state/indexer"
	"github.com/cometbft/cometbft/state/indexer/sink/psql"
	"github.com/cometbft/cometbft/state/txindex/kv"
	"github.com/cometbft/cometbft/types"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/hashicorp/go-hclog"
)

// EventSink is an indexer backend providing the tx/block index services.  This
// implementation stores records in a PostgreSQL database using the schema
// defined in state/indexer/sink/psql/schema.sql.
type TxEventSink struct {
	es             *psql.EventSink
	encodingConfig params.EncodingConfig
}

const (
	TableTxRequests = "tx_requests"
	TxSearchLimit   = uint16(5000)
)

var _ indexer.ModuleEventSinkIndexer = (*TxEventSink)(nil)

func NewTxEventSinkIndexer(es *psql.EventSink, encodingConfig params.EncodingConfig) *TxEventSink {
	return &TxEventSink{es: es, encodingConfig: encodingConfig}
}

func (cs *TxEventSink) InsertModuleEvents(req *abci.RequestFinalizeBlock, res *abci.ResponseFinalizeBlock) error {
	// unmarshal txs
	hclog.Default().Debug("before unmarshal txs")
	for i, txBz := range req.Txs {
		cosmosTx, err := indexer.UnmarshalTxBz(cs, txBz)
		if err != nil {
			return err
		}
		fullMsgsBz, err := indexer.MarshalMsgsAny(cs.encodingConfig, cosmosTx.Body.Messages)
		if err != nil {
			return err
		}
		feeBz, err := cs.encodingConfig.Codec.MarshalJSON(cosmosTx.AuthInfo.Fee)
		if err != nil {
			return err
		}

		// Index the hash of the underlying transaction as a hex string.
		txHash := fmt.Sprintf("%X", types.Tx(txBz).Hash())
		if err := psql.RunInTransaction(cs.es.DB(), func(dbtx *sql.Tx) error {

			// just in case the cometbft indexer has not finished indexing block events, we index it by ourselves
			err := cs.es.IndexBlockEvents(types.EventDataNewBlockEvents{Height: req.Height, Events: res.Events, NumTxs: int64(len(req.Txs))})
			if err != nil {
				return err
			}

			// Find the block associated with this transaction. The block header
			// must have been indexed prior to the transactions belonging to it.
			blockID, err := psql.QueryWithID(dbtx, `
SELECT rowid FROM `+psql.TableBlocks+` WHERE height = $1 AND chain_id = $2;
`, req.Height, cs.es.ChainID())
			if err != nil {
				return err
			}

			// Insert a record for this tx_requests and capture its ID for indexing events.
			// NOTE: for tx index, it is the tx index in the list of txs. Ref: https://github.com/oraichain/cometbft/blob/5c0462aa0de4250a0c1ab43a80f8ea8adb84fa33/state/execution.go#L710; https://github.com/oraichain/cometbft/blob/5c0462aa0de4250a0c1ab43a80f8ea8adb84fa33/state/execution.go#L749
			_, err = psql.QueryWithID(dbtx, `
INSERT INTO `+TableTxRequests+` (block_id, index, height, created_at, tx_hash, messages, fee, memo)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT DO NOTHING
RETURNING rowid;
`, blockID, i, req.Height, req.Time, txHash, fullMsgsBz, string(feeBz), cosmosTx.Body.Memo)
			if err == sql.ErrNoRows {
				return nil // we already saw this transaction; quietly succeed
			} else if err != nil {
				return fmt.Errorf("indexing tx_requests: %w", err)
			}

			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// TODO: add limit & filters based on non-height conditions
func (cs *TxEventSink) SearchTxs(q *query.Query, limit uint16) ([]*txtypes.GetTxResponse, uint64, error) {
	count := uint64(0)
	txResponses := []*txtypes.GetTxResponse{}

	conditions := q.Syntax()
	// conditions to skip because they're handled before "everything else"
	// If we are not matching events and tx.height = 3 occurs more than once, the later value will
	// overwrite the first one.
	conditions, heightInfo := kv.DedupHeight(conditions)

	// extract ranges
	// if both upper and lower bounds exist, it's better to get them in order not
	// no iterate over kvs that are not within range.
	ranges, indexes, heightRange := cometbftindexer.LookForRangesWithHeight(conditions)
	heightInfo.SetheightRange(heightRange)
	whereConditions, args, argsCount := CreateHeightRangeWhereConditions(heightInfo)
	whereConditions, err := cs.createCursorPaginationCondition(whereConditions)
	if err != nil {
		return nil, 0, err
	}
	filterTableClause, filterArgs := CreateNonHeightConditionFilterTable(conditions, ranges, indexes, argsCount)
	queryClause := fmt.Sprintf(`
	-- get all heights <= x that have txs, and limit the number of heights to y
	WITH filtered_heights AS (
    SELECT distinct tr.rowid, height
    FROM tx_results tr %s
    ORDER BY height desc
		LIMIT %d
	),
	-- filter all attributes within the filtered heights. This makes sure we still have limit & pagination without filtering out events
	filtered_tx_event_attributes as (
  SELECT
    events.block_id,
    height,
    tx_id
  FROM
    events
		JOIN filtered_heights fh on (fh.rowid = events.tx_id)
    JOIN attributes ON (events.rowid = attributes.event_id)
  WHERE tx_id is NOT null 
	ORDER BY tx_id DESC 
	),
	-- filter txs based on input composite key conditions
	filtered_tx_ids as (
		select distinct tx_id
		from filtered_tx_event_attributes te
		%s
	)
	-- join everything to get the final table with sufficient data
	select
		tr.height,
		tr.created_at,
		tr.tx_hash,
		messages,
		memo,
		fee,
		tr.tx_result
	from
		filtered_tx_ids ftx
		join tx_results tr on tr.rowid = ftx.tx_id
		join tx_requests on (
			tx_requests.block_id = tr.block_id
			and tx_requests.index = tr.index
		)
	ORDER BY ftx.tx_id DESC;
	`, whereConditions, min(TxSearchLimit, limit), filterTableClause)
	if err := psql.RunInTransaction(cs.es.DB(), func(dbtx *sql.Tx) error {

		// query txs. FIXME: Need filters and limit!
		row, err := dbtx.Query(queryClause, append(args, filterArgs...)...)
		if err != nil {
			return err
		}

		for {
			hasNext := row.Next()
			if !hasNext {
				break
			}
			count++
			var height int64
			var createdAt time.Time
			var txHash string
			var messages []byte
			var memo string
			var fee string
			var txResultBz []byte
			var txResult abci.TxResult

			err = row.Scan(&height, &createdAt, &txHash, &messages, &memo, &fee, &txResultBz)
			if err != nil {
				return err
			}
			msgsAny, err := indexer.UnmarshalMsgsBz(cs.encodingConfig, messages)
			if err != nil {
				return err
			}
			if err := cs.encodingConfig.Codec.Unmarshal(txResultBz, &txResult); err != nil {
				return err
			}

			var feeProto txtypes.Fee
			if err := cs.encodingConfig.Codec.UnmarshalJSON([]byte(fee), &feeProto); err != nil {
				return err
			}

			txBody := txtypes.TxBody{
				Messages: msgsAny,
				Memo:     memo,
			}
			cosmosTx := txtypes.Tx{Body: &txBody, AuthInfo: &txtypes.AuthInfo{Fee: &feeProto}}
			txResponse := cosmostypes.TxResponse{Height: height, TxHash: txHash, Codespace: txResult.Result.Codespace, Code: txResult.Result.Code, Info: txResult.Result.Info, RawLog: txResult.Result.Log, GasWanted: txResult.Result.GasWanted, GasUsed: txResult.Result.GasUsed, Events: txResult.Result.Events}
			txResponses = append(txResponses, &txtypes.GetTxResponse{Tx: &cosmosTx, TxResponse: &txResponse})
		}
		return nil
	}); err != nil {
		return nil, 0, err
	}
	return txResponses, count, nil
}

func (cs *TxEventSink) EmitModuleEvents(req *abci.RequestFinalizeBlock, res *abci.ResponseFinalizeBlock) error {
	return nil
}

func (cs *TxEventSink) ModuleName() string {
	return "tx"
}

func (cs *TxEventSink) EventSink() *psql.EventSink {
	return cs.es
}

func (cs *TxEventSink) EncodingConfig() params.EncodingConfig {
	return cs.encodingConfig
}

func CreateHeightRangeWhereConditions(heightInfo kv.HeightInfo) (whereConditions string, vals []interface{}, argsCount int) {
	// args count is used to increment parameterized arguments
	argsCount = 1
	// prioritize range conditions
	if isHeightRangeNotEmpty(heightInfo.HeightRange()) {
		value := heightInfo.HeightRange()
		ops, values := detectQueryRangeBound(value)
		whereConditions += "WHERE"
		for i, operator := range ops {
			if i == len(ops)-1 {
				whereConditions += fmt.Sprintf(" height %s $%d", operator, argsCount)
			} else {
				whereConditions += fmt.Sprintf(" height %s $%d AND", operator, argsCount)
			}

			argsCount++
		}
		vals = values
		return whereConditions, vals, argsCount
	}
	// if there's no range, and has eq condition -> handle it
	if heightInfo.Height() != 0 {
		return fmt.Sprintf("WHERE height = $%d", argsCount), []interface{}{heightInfo.Height()}, argsCount
	}
	return "", nil, 0
}

func isHeightRangeNotEmpty(heightRange cometbftindexer.QueryRange) bool {
	return heightRange.LowerBound != nil || heightRange.UpperBound != nil
}

func (cs *TxEventSink) createCursorPaginationCondition(whereCondition string) (string, error) {
	if whereCondition != "" {
		return whereCondition, nil
	}
	// if the whereCondition is empty -> we create the pagination cursor based on the latest height
	var height int64
	if err := psql.RunInTransaction(cs.es.DB(), func(dbtx *sql.Tx) error {
		// Find the block associated with this transaction. The block header
		// must have been indexed prior to the transactions belonging to it.
		if err := dbtx.QueryRow(`
SELECT height FROM ` + psql.TableBlocks + ` order by height desc limit 1;
`).Scan(&height); err != nil {
			return fmt.Errorf("finding block height: %w", err)
		}
		return nil
	}); err != nil {
		return "", err
	}
	return fmt.Sprintf("WHERE height <= %d", height), nil
}

func CreateNonHeightConditionFilterTable(conditions []syntax.Condition, ranges cometbftindexer.QueryRanges, rangeIndexes []int, argsCount int) (filterTableClause string, vals []interface{}) {
	// TODO: add filter conditions to handle non-height filters
	return "", []interface{}{}
}

func detectQueryRangeBound(value cometbftindexer.QueryRange) (ops []string, vals []interface{}) {
	if value.LowerBound != nil {
		operator := ">"
		if value.IncludeLowerBound {
			operator = ">="
		}
		ops = append(ops, operator)
		val, _ := value.LowerBound.(*big.Float).Int64()
		vals = append(vals, val)
	}
	if value.UpperBound != nil {
		operator := "<"
		if value.IncludeUpperBound {
			operator = "<="
		}
		ops = append(ops, operator)
		upper, _ := value.UpperBound.(*big.Float).Int64()
		vals = append(vals, upper)
	}
	return ops, vals
}
