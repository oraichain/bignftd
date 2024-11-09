package tests

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/adlio/schema"
	"github.com/cosmos/gogoproto/proto"
	"github.com/ory/dockertest"
	"github.com/ory/dockertest/docker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/types"

	// Register the Postgres database driver.
	"github.com/CosmWasm/wasmd/app/params"
	indexercodec "github.com/CosmWasm/wasmd/indexer/codec"
	indexertx "github.com/CosmWasm/wasmd/indexer/x/tx"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cometbft/cometbft/state/indexer/sink/psql"
	"github.com/cometbft/cometbft/state/txindex"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	_ "github.com/lib/pq"
)

var (
	doPauseAtExit = flag.Bool("pause-at-exit", false,
		"If true, pause the test until interrupted at shutdown, to allow debugging")

	// A hook that test cases can call to obtain the shared database instance
	// used for testing the sink. This is initialized in TestMain (see below).
	testDB func() *sql.DB

	encodingConfig params.EncodingConfig
)

func init() {
	encodingConfig = indexercodec.MakeEncodingConfig()
}

const (
	user     = "admin"
	password = "root"
	port     = "5432"
	dsn      = "postgres://%s:%s@localhost:%s/%s?sslmode=disable"
	dbName   = "postgres"
	chainID  = "testing"

	viewBlockEvents = "block_events"
	viewTxEvents    = "tx_events"
)

func TestMain(m *testing.M) {
	flag.Parse()

	// Set up docker and start a container running PostgreSQL.
	pool, err := dockertest.NewPool(os.Getenv("DOCKER_URL"))
	if err != nil {
		log.Fatalf("Creating docker pool: %v", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "13",
		Env: []string{
			"POSTGRES_USER=" + user,
			"POSTGRES_PASSWORD=" + password,
			"POSTGRES_DB=" + dbName,
			"listen_addresses = '*'",
		},
		ExposedPorts: []string{port + "/tcp"},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		log.Fatalf("Starting docker pool: %v", err)
	}

	if *doPauseAtExit {
		log.Print("Pause at exit is enabled, containers will not expire")
	} else {
		const expireSeconds = 60
		_ = resource.Expire(expireSeconds)
		log.Printf("Container expiration set to %d seconds", expireSeconds)
	}

	// Connect to the database, clear any leftover data, and install the
	// indexing schema.
	conn := fmt.Sprintf(dsn, user, password, resource.GetPort(port+"/tcp"), dbName)
	var db *sql.DB

	if err := pool.Retry(func() error {
		sink, err := psql.NewEventSink(conn, chainID)
		if err != nil {
			return err
		}
		db = sink.DB() // set global for test use
		return db.Ping()
	}); err != nil {
		log.Fatalf("Connecting to database: %v", err)
	}

	if err := resetDatabase(db); err != nil {
		log.Fatalf("Flushing database: %v", err)
	}

	sm, err := readSchema()
	if err != nil {
		log.Fatalf("Reading schema: %v", err)
	}
	migrator := schema.NewMigrator()
	if err := migrator.Apply(db, sm); err != nil {
		log.Fatalf("Applying schema: %v", err)
	}

	// Set up the hook for tests to get the shared database handle.
	testDB = func() *sql.DB { return db }

	// Run the selected test cases.
	code := m.Run()

	// Clean up and shut down the database container.
	if *doPauseAtExit {
		log.Print("Testing complete, pausing for inspection. Send SIGINT to resume teardown")
		waitForInterrupt()
		log.Print("(resuming)")
	}
	log.Print("Shutting down database")
	if err := pool.Purge(resource); err != nil {
		log.Printf("WARNING: Purging pool failed: %v", err)
	}
	if err := db.Close(); err != nil {
		log.Printf("WARNING: Closing database failed: %v", err)
	}

	os.Exit(code)
}

func TestIndexing(t *testing.T) {
	t.Run("IndexBlockEvents", func(t *testing.T) {
		indexer := psql.NewEventSinkFromDB(testDB(), chainID)
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(1)))
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(2)))

		verifyBlock(t, 1)
		verifyBlock(t, 2)

		verifyNotImplemented(t, "hasBlock", func() (bool, error) { return indexer.HasBlock(1) })
		verifyNotImplemented(t, "hasBlock", func() (bool, error) { return indexer.HasBlock(2) })

		verifyNotImplemented(t, "block search", func() (bool, error) {
			v, err := indexer.SearchBlockEvents(context.Background(), nil)
			return v != nil, err
		})

		require.NoError(t, verifyTimeStamp(psql.TableBlocks))

		// Attempting to reindex the same events should gracefully succeed.
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(1)))
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(2)))
	})

	t.Run("IndexTxEvents", func(t *testing.T) {
		indexer := psql.NewEventSinkFromDB(testDB(), chainID)

		txResult := txResultWithEvents([]abci.Event{
			psql.MakeIndexedEvent("account.number", "1"),
			psql.MakeIndexedEvent("account.owner", "Ivan"),
			psql.MakeIndexedEvent("account.owner", "Yulieta"),

			{Type: "", Attributes: []abci.EventAttribute{
				{
					Key:   "not_allowed",
					Value: "Vlad",
					Index: true,
				},
			}},
		}, 1, 0)
		require.NoError(t, indexer.IndexTxEvents([]*abci.TxResult{txResult}))

		txr, err := loadTxResult(types.Tx(txResult.Tx).Hash())
		require.NoError(t, err)
		assert.Equal(t, txResult, txr)

		require.NoError(t, verifyTimeStamp(psql.TableTxResults))
		require.NoError(t, verifyTimeStamp(viewTxEvents))

		verifyNotImplemented(t, "getTxByHash", func() (bool, error) {
			txr, err := indexer.GetTxByHash(types.Tx(txResult.Tx).Hash())
			return txr != nil, err
		})
		verifyNotImplemented(t, "tx search", func() (bool, error) {
			txr, err := indexer.SearchTxEvents(context.Background(), nil)
			return txr != nil, err
		})

		// try to insert the duplicate tx events.
		err = indexer.IndexTxEvents([]*abci.TxResult{txResult})
		require.NoError(t, err)
	})

	t.Run("IndexCosmWasmTxs", func(t *testing.T) {
		indexer := psql.NewEventSinkFromDB(testDB(), chainID)
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(1)))
		require.NoError(t, indexer.IndexBlockEvents(newTestBlockEvents(2)))

		txResult := wasmTxResultWithEvents([]abci.Event{
			psql.MakeIndexedEvent("account.number", "1"),
			psql.MakeIndexedEvent("account.owner", "Ivan"),
			psql.MakeIndexedEvent("account.owner", "Yulieta"),
			psql.MakeIndexedEvent("wasm.data", "Wasm data"),

			{Type: "", Attributes: []abci.EventAttribute{
				{
					Key:   "not_allowed",
					Value: "Vlad",
					Index: true,
				},
			}},
		}, 1, 0)
		nonWasmTxResult := txResultWithEvents([]abci.Event{
			psql.MakeIndexedEvent("account.number", "2"),
			psql.MakeIndexedEvent("account.owner", "Duc"),
			psql.MakeIndexedEvent("account.owner", "Pham"),

			{Type: "", Attributes: []abci.EventAttribute{
				{
					Key:   "not_allowed",
					Value: "Vlad",
					Index: true,
				},
			}},
		}, 1, 1)

		nonWasmTxResultNextHeight := txResultWithEvents([]abci.Event{
			psql.MakeIndexedEvent("account.number", "2"),
			psql.MakeIndexedEvent("account.owner", "Duc"),
			psql.MakeIndexedEvent("account.owner", "Pham"),

			{Type: "", Attributes: []abci.EventAttribute{
				{
					Key:   "not_allowed",
					Value: "Vlad",
					Index: true,
				},
			}},
		}, 2, 0)

		abciTxResults := []*abci.TxResult{txResult, nonWasmTxResult, nonWasmTxResultNextHeight}

		require.NoError(t, indexer.IndexTxEvents(abciTxResults))

		// try indexing tx requests
		time := time.Now()
		firstBlockTxs := [][]byte{abciTxResults[0].Tx, abciTxResults[1].Tx}
		secBlockTxs := [][]byte{abciTxResults[2].Tx}
		firstExecTxResults := []*abci.ExecTxResult{&abciTxResults[0].Result, &abciTxResults[1].Result}
		secExecTxResults := []*abci.ExecTxResult{&abciTxResults[2].Result}

		customTxEventSink := indexertx.NewTxEventSinkIndexer(indexer, encodingConfig)
		err := customTxEventSink.InsertModuleEvents(&abci.RequestFinalizeBlock{Height: 1, Txs: firstBlockTxs, Time: time}, &abci.ResponseFinalizeBlock{Events: []abci.Event{}, TxResults: firstExecTxResults})
		require.NoError(t, err)
		err = customTxEventSink.InsertModuleEvents(&abci.RequestFinalizeBlock{Height: 2, Txs: secBlockTxs, Time: time}, &abci.ResponseFinalizeBlock{Events: []abci.Event{}, TxResults: secExecTxResults})
		require.NoError(t, err)

		_, count, err := customTxEventSink.SearchTxs(query.MustCompile("tx.height >= 1 AND tx.height <= 2 AND john.doe < 1 AND john.doe ='10'"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(3), count)

		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("tx.height >= 1 AND tx.height < 2 AND wasm.foobar = 'x'"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(2), count)

		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("tx.height >= 1 AND tx.height > 1"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(3), count)

		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("tx.height < 1"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(0), count)

		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("tx.height = 2 AND hello.world > 1"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)

		// no height clause, with limit 10 -> got all txs
		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("hello.world > 1"), 10)
		require.NoError(t, err)
		require.Equal(t, uint64(3), count)

		// limit 1 with no query clause -> only 1 txs
		_, count, err = customTxEventSink.SearchTxs(query.MustCompile("hello.world > 1"), 1)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count)
	})

	t.Run("IndexerService", func(t *testing.T) {
		indexer := psql.NewEventSinkFromDB(testDB(), chainID)

		// event bus
		eventBus := types.NewEventBus()
		err := eventBus.Start()
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := eventBus.Stop(); err != nil {
				t.Error(err)
			}
		})

		service := txindex.NewIndexerService(indexer.TxIndexer(), indexer.BlockIndexer(), eventBus, true)
		service.SetLogger(tmlog.TestingLogger())
		err = service.Start()
		require.NoError(t, err)
		t.Cleanup(func() {
			if err := service.Stop(); err != nil {
				t.Error(err)
			}
		})

		// publish block with txs
		err = eventBus.PublishEventNewBlockEvents(types.EventDataNewBlockEvents{
			Height: 1,
			NumTxs: 2,
		})
		require.NoError(t, err)
		txResult1 := &abci.TxResult{
			Height: 1,
			Index:  uint32(0),
			Tx:     types.Tx("foo"),
			Result: abci.ExecTxResult{Code: 0},
		}
		err = eventBus.PublishEventTx(types.EventDataTx{TxResult: *txResult1})
		require.NoError(t, err)
		txResult2 := &abci.TxResult{
			Height: 1,
			Index:  uint32(1),
			Tx:     types.Tx("bar"),
			Result: abci.ExecTxResult{Code: 1},
		}
		err = eventBus.PublishEventTx(types.EventDataTx{TxResult: *txResult2})
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		require.True(t, service.IsRunning())
	})
}

func TestStop(t *testing.T) {
	indexer := psql.NewEventSinkFromDB(testDB(), chainID)
	require.NoError(t, indexer.Stop())
}

// newTestBlock constructs a fresh copy of a new block event containing
// known test values to exercise the indexer.
func newTestBlockEvents(height int64) types.EventDataNewBlockEvents {
	return types.EventDataNewBlockEvents{
		Height: height,
		Events: []abci.Event{
			psql.MakeIndexedEvent("begin_event.proposer", "FCAA001"),
			psql.MakeIndexedEvent("thingy.whatzit", "O.O"),
			psql.MakeIndexedEvent("end_event.foo", "100"),
			psql.MakeIndexedEvent("thingy.whatzit", "-.O"),
		},
	}
}

// readSchema loads the indexing database schema file
func readSchema() ([]*schema.Migration, error) {
	filename := filepath.Join("../", "dbschema", "schema.sql")
	contents, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read sql file from '%s': %w", filename, err)
	}

	return []*schema.Migration{{
		ID:     time.Now().Local().String() + " db schema",
		Script: string(contents),
	}}, nil
}

// resetDB drops all the data from the test database.
func resetDatabase(db *sql.DB) error {
	_, err := db.Exec(`DROP TABLE IF EXISTS blocks,tx_results,events,attributes CASCADE;`)
	if err != nil {
		return fmt.Errorf("dropping tables: %v", err)
	}
	_, err = db.Exec(`DROP VIEW IF EXISTS event_attributes,block_events,tx_events CASCADE;`)
	if err != nil {
		return fmt.Errorf("dropping views: %v", err)
	}
	return nil
}

// txResultWithEvents constructs a fresh transaction result with fixed values
// for testing, that includes the specified events.
func txResultWithEvents(events []abci.Event, height int64, txIndex uint32) *abci.TxResult {

	txBuilder := encodingConfig.TxConfig.NewTxBuilder()
	grant := "orai1wsg0l9c6tr5uzjrhwhqch9tt4e77h0w28wvp3u"
	instantiateMsg := banktypes.MsgSend{
		FromAddress: grant,
		ToAddress:   grant,
		Amount:      sdk.NewCoins(sdk.NewCoin("orai", math.NewInt(100))),
	}

	if err := txBuilder.SetMsgs(&instantiateMsg); err != nil {
		panic(err)
	}
	txBuilder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("orai", math.NewInt(1))))
	tx := txBuilder.GetTx()
	txBz, err := encodingConfig.TxConfig.TxEncoder()(tx)
	if err != nil {
		panic(err)
	}

	return &abci.TxResult{
		Height: height,
		Index:  txIndex,
		Tx:     txBz,
		Result: abci.ExecTxResult{
			Data:   []byte{0},
			Code:   abci.CodeTypeOK,
			Log:    "",
			Events: events,
		},
	}
}

// txResultWithEvents constructs a fresh transaction result with fixed values
// for testing, that includes the specified events.
func wasmTxResultWithEvents(events []abci.Event, height int64, txIndex uint32) *abci.TxResult {

	txBuilder := encodingConfig.TxConfig.NewTxBuilder()
	grant := "orai1wsg0l9c6tr5uzjrhwhqch9tt4e77h0w28wvp3u"
	instantiateMsg := wasmtypes.MsgInstantiateContract{
		Sender: grant,
		CodeID: 0,
		Label:  "label",
		Funds:  sdk.NewCoins(sdk.NewCoin("orai", math.NewInt(100))),
		Msg:    []byte(wasmtypes.RawContractMessage{}),
		Admin:  grant,
	}

	if err := txBuilder.SetMsgs(&instantiateMsg); err != nil {
		panic(err)
	}
	txBuilder.SetMemo("hello world")
	txBuilder.SetFeeGranter(sdk.MustAccAddressFromBech32(grant))
	txBuilder.SetFeePayer(sdk.MustAccAddressFromBech32(grant))
	txBuilder.SetGasLimit(10000000)
	txBuilder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("orai", math.NewInt(1))))
	tx := txBuilder.GetTx()
	txBz, err := encodingConfig.TxConfig.TxEncoder()(tx)
	if err != nil {
		panic(err)
	}

	return &abci.TxResult{
		Height: height,
		Index:  txIndex,
		Tx:     txBz,
		Result: abci.ExecTxResult{
			Data:   []byte{0},
			Code:   abci.CodeTypeOK,
			Log:    "",
			Events: events,
		},
	}
}

func loadTxResult(hash []byte) (*abci.TxResult, error) {
	hashString := fmt.Sprintf("%X", hash)
	var resultData []byte
	if err := testDB().QueryRow(`
SELECT tx_result FROM `+psql.TableTxResults+` WHERE tx_hash = $1;
`, hashString).Scan(&resultData); err != nil {
		return nil, fmt.Errorf("lookup transaction for hash %q failed: %v", hashString, err)
	}

	txr := new(abci.TxResult)
	if err := proto.Unmarshal(resultData, txr); err != nil {
		return nil, fmt.Errorf("unmarshaling txr: %v", err)
	}

	return txr, nil
}

func verifyTimeStamp(tableName string) error {
	return testDB().QueryRow(fmt.Sprintf(`
SELECT DISTINCT %[1]s.created_at
  FROM %[1]s
  WHERE %[1]s.created_at >= $1;
`, tableName), time.Now().Add(-2*time.Second)).Err()
}

func verifyBlock(t *testing.T, height int64) {
	// Check that the blocks table contains an entry for this height.
	if err := testDB().QueryRow(`
SELECT height FROM `+psql.TableBlocks+` WHERE height = $1;
`, height).Err(); err == sql.ErrNoRows {
		t.Errorf("No block found for height=%d", height)
	} else if err != nil {
		t.Fatalf("Database query failed: %v", err)
	}

	// Verify the presence of begin_block and end_block events.
	if err := testDB().QueryRow(`
SELECT type, height, chain_id FROM `+viewBlockEvents+`
  WHERE height = $1 AND type = $2 AND chain_id = $3;
`, height, psql.EventTypeFinalizeBlock, chainID).Err(); err == sql.ErrNoRows {
		t.Errorf("No %q event found for height=%d", psql.EventTypeFinalizeBlock, height)
	} else if err != nil {
		t.Fatalf("Database query failed: %v", err)
	}
}

// verifyNotImplemented calls f and verifies that it returns both a
// false-valued flag and a non-nil error whose string matching the expected
// "not supported" message with label prefixed.
func verifyNotImplemented(t *testing.T, label string, f func() (bool, error)) {
	t.Helper()
	t.Logf("Verifying that %q reports it is not implemented", label)

	want := label + " is not supported via the postgres event sink"
	ok, err := f()
	assert.False(t, ok)
	require.NotNil(t, err)
	assert.Equal(t, want, err.Error())
}

// waitForInterrupt blocks until a SIGINT is received by the process.
func waitForInterrupt() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
