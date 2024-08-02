package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	"google.golang.org/grpc"

	abciserver "github.com/cometbft/cometbft/abci/server"
	tcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	tmos "github.com/cometbft/cometbft/libs/os"
	"github.com/cometbft/cometbft/node"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/rpc/client/local"
	dbm "github.com/tendermint/tm-db"

	"github.com/cosmos/cosmos-sdk/server/rosetta"
	crgserver "github.com/cosmos/cosmos-sdk/server/rosetta/lib/server"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	"github.com/cosmos/cosmos-sdk/server/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"

	ethdebug "github.com/tharsis/ethermint/rpc/ethereum/namespaces/debug"
	"github.com/tharsis/ethermint/server/config"
	srvflags "github.com/tharsis/ethermint/server/flags"
)

// DBOpener is a function to open `application.db`, potentially with customized options.
type DBOpener func(opts types.AppOptions, rootDir string, backend dbm.BackendType) (dbm.DB, error)

// StartOptions defines options that can be customized in `StartCmd`
type StartOptions struct {
	AppCreator      types.AppCreator
	DefaultNodeHome string
	DBOpener        DBOpener
}

// NewDefaultStartOptions use the default db opener provided in tm-db.
func NewDefaultStartOptions(appCreator types.AppCreator, defaultNodeHome string) StartOptions {
	return StartOptions{
		AppCreator:      appCreator,
		DefaultNodeHome: defaultNodeHome,
		DBOpener:        openDB,
	}
}

// StartCmd runs the service passed in, either stand-alone or in-process with
// Tendermint.
func StartCmd(opts StartOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the full node",
		Long: `Run the full node application with Tendermint in or out of process. By
default, the application will run with Tendermint in process.

Pruning options can be provided via the '--pruning' flag or alternatively with '--pruning-keep-recent',
'pruning-keep-every', and 'pruning-interval' together.

For '--pruning' the options are as follows:

default: the last 100 states are kept in addition to every 500th state; pruning at 10 block intervals
nothing: all historic states will be saved, nothing will be deleted (i.e. archiving node)
everything: all saved states will be deleted, storing only the current state; pruning at 10 block intervals
custom: allow pruning options to be manually specified through 'pruning-keep-recent', 'pruning-keep-every', and 'pruning-interval'

Node halting configurations exist in the form of two flags: '--halt-height' and '--halt-time'. During
the ABCI Commit phase, the node will check if the current block height is greater than or equal to
the halt-height or if the current block time is greater than or equal to the halt-time. If so, the
node will attempt to gracefully shutdown and the block will not be committed. In addition, the node
will not be able to commit subsequent blocks.

For profiling and benchmarking purposes, CPU profiling can be enabled via the '--cpu-profile' flag
which accepts a path for the resulting pprof file.
`,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)

			// Bind flags to the Context's Viper so the app construction can set
			// options accordingly.
			err := serverCtx.Viper.BindPFlags(cmd.Flags())
			if err != nil {
				return err
			}

			_, err = server.GetPruningOptionsFromFlags(serverCtx.Viper)
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			withTM, _ := cmd.Flags().GetBool(srvflags.WithTendermint)
			if !withTM {
				serverCtx.Logger.Info("starting ABCI without Tendermint")
				return startStandAlone(serverCtx, opts)
			}

			serverCtx.Logger.Info("Unlocking keyring")

			// fire unlock precess for keyring
			keyringBackend, _ := cmd.Flags().GetString(flags.FlagKeyringBackend)
			if keyringBackend == keyring.BackendFile {
				_, err = clientCtx.Keyring.List()
				if err != nil {
					return err
				}
			}

			serverCtx.Logger.Info("starting ABCI with Tendermint")

			// amino is needed here for backwards compatibility of REST routes
			err = startInProcess(serverCtx, clientCtx, opts)
			errCode, ok := err.(server.ErrorCode)
			if !ok {
				return err
			}

			serverCtx.Logger.Debug(fmt.Sprintf("received quit signal: %d", errCode.Code))
			return nil
		},
	}

	cmd.Flags().String(flags.FlagHome, opts.DefaultNodeHome, "The application home directory")
	cmd.Flags().Bool(srvflags.WithTendermint, true, "Run abci app embedded in-process with tendermint")
	cmd.Flags().String(srvflags.Address, "tcp://0.0.0.0:26658", "Listen address")
	cmd.Flags().String(srvflags.Transport, "socket", "Transport protocol: socket, grpc")
	cmd.Flags().String(srvflags.TraceStore, "", "Enable KVStore tracing to an output file")
	cmd.Flags().String(server.FlagMinGasPrices, "", "Minimum gas prices to accept for transactions; Any fee in a tx must meet this minimum (e.g. 0.01photon;0.0001stake)")
	cmd.Flags().IntSlice(server.FlagUnsafeSkipUpgrades, []int{}, "Skip a set of upgrade heights to continue the old binary")
	cmd.Flags().Uint64(server.FlagHaltHeight, 0, "Block height at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Uint64(server.FlagHaltTime, 0, "Minimum block time (in Unix seconds) at which to gracefully halt the chain and shutdown the node")
	cmd.Flags().Bool(server.FlagInterBlockCache, true, "Enable inter-block caching")
	cmd.Flags().String(srvflags.CPUProfile, "", "Enable CPU profiling and write to the provided file")
	cmd.Flags().Bool(server.FlagTrace, false, "Provide full stack traces for errors in ABCI Log")
	cmd.Flags().String(server.FlagPruning, storetypes.PruningOptionDefault, "Pruning strategy (default|nothing|everything|custom)")
	cmd.Flags().Uint64(server.FlagPruningKeepRecent, 0, "Number of recent heights to keep on disk (ignored if pruning is not 'custom')")
	cmd.Flags().Uint64(server.FlagPruningKeepEvery, 0, "Offset heights to keep on disk after 'keep-every' (ignored if pruning is not 'custom')")
	cmd.Flags().Uint64(server.FlagPruningInterval, 0, "Height interval at which pruned heights are removed from disk (ignored if pruning is not 'custom')")
	cmd.Flags().Uint(server.FlagInvCheckPeriod, 0, "Assert registered invariants every N blocks")
	cmd.Flags().Uint64(server.FlagMinRetainBlocks, 0, "Minimum block height offset during ABCI commit to prune Tendermint blocks")

	cmd.Flags().Bool(srvflags.GRPCEnable, true, "Define if the gRPC server should be enabled")
	cmd.Flags().String(srvflags.GRPCAddress, serverconfig.DefaultGRPCAddress, "the gRPC server address to listen on")
	cmd.Flags().Bool(srvflags.GRPCWebEnable, true, "Define if the gRPC-Web server should be enabled. (Note: gRPC must also be enabled.)")
	cmd.Flags().String(srvflags.GRPCWebAddress, serverconfig.DefaultGRPCWebAddress, "The gRPC-Web server address to listen on")

	cmd.Flags().Bool(srvflags.RPCEnable, false, "Defines if Cosmos-sdk REST server should be enabled")
	cmd.Flags().Bool(srvflags.EnabledUnsafeCors, false, "Defines if CORS should be enabled (unsafe - use it at your own risk)")

	cmd.Flags().Bool(srvflags.JSONRPCEnable, true, "Define if the gRPC server should be enabled")
	cmd.Flags().StringSlice(srvflags.JSONRPCAPI, config.GetDefaultAPINamespaces(), "Defines a list of JSON-RPC namespaces that should be enabled")
	cmd.Flags().String(srvflags.JSONRPCAddress, config.DefaultJSONRPCAddress, "the JSON-RPC server address to listen on")
	cmd.Flags().String(srvflags.JSONWsAddress, config.DefaultJSONRPCWsAddress, "the JSON-RPC WS server address to listen on")
	cmd.Flags().Uint64(srvflags.JSONRPCGasCap, config.DefaultGasCap, "Sets a cap on gas that can be used in eth_call/estimateGas unit is aphoton (0=infinite)")
	cmd.Flags().Float64(srvflags.JSONRPCTxFeeCap, config.DefaultTxFeeCap, "Sets a cap on transaction fee that can be sent via the RPC APIs (1 = default 1 photon)")
	cmd.Flags().Int32(srvflags.JSONRPCFilterCap, config.DefaultFilterCap, "Sets the global cap for total number of filters that can be created")
	cmd.Flags().Duration(srvflags.JSONRPCEVMTimeout, config.DefaultEVMTimeout, "Sets a timeout used for eth_call (0=infinite)")
	cmd.Flags().Duration(srvflags.JSONRPCHTTPTimeout, config.DefaultHTTPTimeout, "Sets a read/write timeout for json-rpc http server (0=infinite)")
	cmd.Flags().Duration(srvflags.JSONRPCHTTPIdleTimeout, config.DefaultHTTPIdleTimeout, "Sets a idle timeout for json-rpc http server (0=infinite)")
	cmd.Flags().Int32(srvflags.JSONRPCLogsCap, config.DefaultLogsCap, "Sets the max number of results can be returned from single `eth_getLogs` query")
	cmd.Flags().Int32(srvflags.JSONRPCBlockRangeCap, config.DefaultBlockRangeCap, "Sets the max block range allowed for `eth_getLogs` query")

	cmd.Flags().String(srvflags.EVMTracer, config.DefaultEVMTracer, "the EVM tracer type to collect execution traces from the EVM transaction execution (json|struct|access_list|markdown)")
	cmd.Flags().Uint64(srvflags.EVMMaxTxGasWanted, config.DefaultMaxTxGasWanted, "the gas wanted for each eth tx returned in ante handler in check tx mode")

	cmd.Flags().String(srvflags.TLSCertPath, "", "the cert.pem file path for the server TLS configuration")
	cmd.Flags().String(srvflags.TLSKeyPath, "", "the key.pem file path for the server TLS configuration")

	cmd.Flags().Uint64(server.FlagStateSyncSnapshotInterval, 0, "State sync snapshot interval")
	cmd.Flags().Uint32(server.FlagStateSyncSnapshotKeepRecent, 2, "State sync snapshot to keep")

	// add support for all Tendermint-specific command line options
	tcmd.AddNodeFlags(cmd)
	return cmd
}

func startStandAlone(ctx *server.Context, opts StartOptions) error {
	addr := ctx.Viper.GetString(srvflags.Address)
	transport := ctx.Viper.GetString(srvflags.Transport)
	home := ctx.Viper.GetString(flags.FlagHome)

	db, err := opts.DBOpener(ctx.Viper, home, getAppDBBackend(ctx.Viper))
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctx.Logger.With("error", err).Error("error closing db")
		}
	}()

	traceWriterFile := ctx.Viper.GetString(srvflags.TraceStore)
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return err
	}

	app := opts.AppCreator(ctx.Logger, db, traceWriter, ctx.Viper)

	svr, err := abciserver.NewServer(addr, transport, app)
	if err != nil {
		return fmt.Errorf("error creating listener: %v", err)
	}

	svr.SetLogger(ctx.Logger.With("server", "abci"))

	err = svr.Start()
	if err != nil {
		tmos.Exit(err.Error())
	}

	defer func() {
		if err = svr.Stop(); err != nil {
			tmos.Exit(err.Error())
		}
	}()

	// Wait for SIGINT or SIGTERM signal
	return server.WaitForQuitSignals()
}

// legacyAminoCdc is used for the legacy REST API
func startInProcess(ctx *server.Context, clientCtx client.Context, opts StartOptions) (err error) {
	cfg := ctx.Config
	home := cfg.RootDir
	logger := ctx.Logger
	var cpuProfileCleanup func() error

	if cpuProfile := ctx.Viper.GetString(srvflags.CPUProfile); cpuProfile != "" {
		fp, err := ethdebug.ExpandHome(cpuProfile)
		if err != nil {
			ctx.Logger.Debug("failed to get filepath for the CPU profile file", "error", err.Error())
			return err
		}
		f, err := os.Create(fp)
		if err != nil {
			return err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		cpuProfileCleanup = func() error {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				logger.Error("failed to close CPU profiler file", "error", err.Error())
				return err
			}
			return nil
		}
	}

	traceWriterFile := ctx.Viper.GetString(srvflags.TraceStore)

	db, err := opts.DBOpener(ctx.Viper, home, getAppDBBackend(ctx.Viper))
	if err != nil {
		logger.Error("failed to open DB", "error", err.Error())
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctx.Logger.With("error", err).Error("error closing db")
		}
	}()

	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		logger.Error("failed to open trace writer", "error", err.Error())
		return err
	}

	appConfig, err := config.GetConfig(ctx.Viper)
	if err != nil {
		logger.Error("failed to get config", "error", err.Error())
		return err
	}

	if err := appConfig.ValidateBasic(); err != nil {
		if strings.Contains(err.Error(), "set min gas price in app.toml or flag or env variable") {
			ctx.Logger.Error(
				"WARNING: The minimum-gas-prices config in app.toml is set to the empty string. " +
					"This defaults to 0 in the current version, but will error in the next version " +
					"(SDK v0.44). Please explicitly put the desired minimum-gas-prices in your app.toml.",
			)
		} else if strings.Contains(err.Error(), "invalid ethermint") {
			home := ctx.Viper.GetString(flags.FlagHome)
			appConfigPath := filepath.Join(home, "config/app.toml")
			ctx.Logger.Info(
				"This node does not have ethermint configurations. Applying the default values to " + appConfigPath,
			)
			appConfigTemplate, defaultAppConfig := config.AppConfig("")
			appConfig.EVM = defaultAppConfig.EVM
			appConfig.JSONRPC = defaultAppConfig.JSONRPC
			appConfig.TLS = defaultAppConfig.TLS
			// overwrite the config file with new values
			serverconfig.SetConfigTemplate(appConfigTemplate)
			serverconfig.WriteConfigFile(appConfigPath, appConfig)
		} else {
			return err
		}
	}
	app := opts.AppCreator(ctx.Logger, db, traceWriter, ctx.Viper)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		logger.Error("failed load or gen node key", "error", err.Error())
		return err
	}

	genDocProvider := node.DefaultGenesisDocProviderFunc(cfg)
	tmNode, err := node.NewNode(
		cfg,
		pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile()),
		nodeKey,
		proxy.NewLocalClientCreator(app),
		genDocProvider,
		node.DefaultDBProvider,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		ctx.Logger.With("server", "node"),
	)
	if err != nil {
		logger.Error("failed init node", "error", err.Error())
		return err
	}

	if err := tmNode.Start(); err != nil {
		logger.Error("failed start tendermint server", "error", err.Error())
		return err
	}

	// Add the tx service to the gRPC router. We only need to register this
	// service if API or gRPC or JSONRPC is enabled, and avoid doing so in the general
	// case, because it spawns a new local tendermint RPC client.
	if appConfig.API.Enable || appConfig.GRPC.Enable || appConfig.JSONRPC.Enable {
		clientCtx = clientCtx.WithClient(local.New(tmNode))

		app.RegisterTxService(clientCtx)
		app.RegisterTendermintService(clientCtx)
	}

	var apiSrv *api.Server
	if appConfig.API.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx := clientCtx.
			WithHomeDir(home).
			WithChainID(genDoc.ChainID)

		apiSrv = api.New(clientCtx, ctx.Logger.With("server", "api"))
		app.RegisterAPIRoutes(apiSrv, appConfig.API)
		errCh := make(chan error)

		go func() {
			if err := apiSrv.Start(appConfig.Config); err != nil {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err
		case <-time.After(types.ServerStartTime): // assume server started successfully
		}
	}

	var (
		grpcSrv    *grpc.Server
		grpcWebSrv *http.Server
	)
	if appConfig.GRPC.Enable {
		grpcSrv, err = servergrpc.StartGRPCServer(clientCtx, app, appConfig.GRPC.Address)
		if err != nil {
			return err
		}
		if appConfig.GRPCWeb.Enable {
			grpcWebSrv, err = servergrpc.StartGRPCWeb(grpcSrv, appConfig.Config)
			if err != nil {
				ctx.Logger.Error("failed to start grpc-web http server", "error", err)
				return err
			}
		}
	}

	var rosettaSrv crgserver.Server
	if appConfig.Rosetta.Enable {
		offlineMode := appConfig.Rosetta.Offline
		if !appConfig.GRPC.Enable { // If GRPC is not enabled rosetta cannot work in online mode, so it works in offline mode.
			offlineMode = true
		}

		conf := &rosetta.Config{
			Blockchain:    appConfig.Rosetta.Blockchain,
			Network:       appConfig.Rosetta.Network,
			TendermintRPC: ctx.Config.RPC.ListenAddress,
			GRPCEndpoint:  appConfig.GRPC.Address,
			Addr:          appConfig.Rosetta.Address,
			Retries:       appConfig.Rosetta.Retries,
			Offline:       offlineMode,
		}
		conf.WithCodec(clientCtx.InterfaceRegistry, clientCtx.Codec.(*codec.ProtoCodec))

		rosettaSrv, err = rosetta.ServerFromConfig(conf)
		if err != nil {
			return err
		}
		errCh := make(chan error)
		go func() {
			if err := rosettaSrv.Start(); err != nil {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err
		case <-time.After(types.ServerStartTime): // assume server started successfully
		}
	}

	var (
		httpSrv     *http.Server
		httpSrvDone chan struct{}
	)

	if appConfig.JSONRPC.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx := clientCtx.WithChainID(genDoc.ChainID)

		tmEndpoint := "/websocket"
		tmRPCAddr := cfg.RPC.ListenAddress
		httpSrv, httpSrvDone, err = StartJSONRPC(ctx, clientCtx, tmRPCAddr, tmEndpoint, appConfig)
		if err != nil {
			return err
		}
	}

	defer func() {
		if tmNode.IsRunning() {
			_ = tmNode.Stop()
		}

		if cpuProfileCleanup != nil {
			_ = cpuProfileCleanup()
		}

		if apiSrv != nil {
			_ = apiSrv.Close()
		}

		if grpcSrv != nil {
			grpcSrv.Stop()
			if grpcWebSrv != nil {
				if err := grpcWebSrv.Close(); err != nil {
					logger.Error("failed to close the grpcWebSrc", "error", err.Error())
				}
			}
		}

		if httpSrv != nil {
			shutdownCtx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelFn()

			if err := httpSrv.Shutdown(shutdownCtx); err != nil {
				logger.Error("HTTP server shutdown produced a warning", "error", err.Error())
			} else {
				logger.Info("HTTP server shut down, waiting 5 sec")
				select {
				case <-time.Tick(5 * time.Second):
				case <-httpSrvDone:
				}
			}
		}

		logger.Info("Bye!")
	}()

	// Wait for SIGINT or SIGTERM signal
	return server.WaitForQuitSignals()
}

func openDB(_ types.AppOptions, rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile == "" {
		return
	}

	filePath := filepath.Clean(traceWriterFile)
	return os.OpenFile(
		filePath,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o600,
	)
}

// getAppDBBackend gets the backend type to use for the application DBs.
// NOTE: getAppDBBackend is backported from newer versions of cosmos-sdk
func getAppDBBackend(opts types.AppOptions) dbm.BackendType {
	rv := cast.ToString(opts.Get("app-db-backend"))
	if len(rv) == 0 {
		rv = cast.ToString(opts.Get("db_backend"))
	}
	if len(rv) != 0 {
		return dbm.BackendType(rv)
	}

	return dbm.GoLevelDBBackend
}