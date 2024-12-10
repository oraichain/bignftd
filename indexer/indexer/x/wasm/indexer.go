package wasm

import (
	"github.com/CosmWasm/wasmd/app/params"
	"github.com/CosmWasm/wasmd/indexer/indexer/sink/psql"
	indexerType "github.com/CosmWasm/wasmd/indexer/indexer/types"
	"github.com/CosmWasm/wasmd/streaming/redpanda"
	"github.com/CosmWasm/wasmd/x/wasm/types"
	abci "github.com/cometbft/cometbft/abci/types"
)

// EventSink is an indexer backend providing the tx/block index services.  This
// implementation stores records in a PostgreSQL database using the schema
// defined in state/indexer/sink/psql/schema.sql.
type WasmEventSink struct {
	es             *psql.EventSink
	encodingConfig params.EncodingConfig
	ri             *redpanda.RedpandaInfo
}

var _ indexerType.ModuleEventSinkIndexer = (*WasmEventSink)(nil)

func NewWasmEventSinkIndexer(
	es *psql.EventSink,
	encodingConfig params.EncodingConfig,
	ri *redpanda.RedpandaInfo,
) *WasmEventSink {
	return &WasmEventSink{es: es, encodingConfig: encodingConfig, ri: ri}
}

func (cs *WasmEventSink) InsertModuleEvents(req *abci.RequestFinalizeBlock, res *abci.ResponseFinalizeBlock) error {
	return nil
}

func (cs *WasmEventSink) EmitModuleEvents(req *abci.RequestFinalizeBlock, res *abci.ResponseFinalizeBlock) error {
	return nil
}

func (cs *WasmEventSink) ModuleName() string {
	return types.ModuleName
}

func (cs *WasmEventSink) EventSink() *psql.EventSink {
	return cs.es
}

func (cs *WasmEventSink) EncodingConfig() params.EncodingConfig {
	return cs.encodingConfig
}
