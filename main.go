package main

import (
	"crypto/tls"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/cmd/utils"
	"github.com/statechannels/go-nitro/internal/logging"
	nodeUtils "github.com/statechannels/go-nitro/internal/node"
	"github.com/statechannels/go-nitro/internal/rpc"
	"github.com/statechannels/go-nitro/node"
	"github.com/statechannels/go-nitro/node/engine/chainservice"
	p2pms "github.com/statechannels/go-nitro/node/engine/messageservice/p2p-message-service"
	"github.com/statechannels/go-nitro/node/engine/store"
	"github.com/statechannels/go-nitro/paymentsmanager"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
)

func main() {
	const (
		CONFIG = "config"

		// Connectivity
		CONNECTIVITY_CATEGORY = "Connectivity:"
		USE_NATS              = "usenats"
		CHAIN_URL             = "chainurl"
		CHAIN_START_BLOCK     = "chainstartblock"
		CHAIN_AUTH_TOKEN      = "chainauthtoken"
		NA_ADDRESS            = "naaddress"
		VPA_ADDRESS           = "vpaaddress"
		CA_ADDRESS            = "caaddress"
		BRIDGE_ADDRESS        = "bridgeaddress"
		PUBLIC_IP             = "publicip"
		MSG_PORT              = "msgport"
		WS_MSG_PORT           = "wsmsgport"
		RPC_PORT              = "rpcport"
		GUI_PORT              = "guiport"
		BOOT_PEERS            = "bootpeers"
		L2                    = "l2"
		EXT_MULTIADDR         = "extMultiAddr"

		// Keys
		KEYS_CATEGORY = "Keys:"
		PK            = "pk"
		CHAIN_PK      = "chainpk"

		// Storage
		STORAGE_CATEGORY     = "Storage:"
		USE_DURABLE_STORE    = "usedurablestore"
		DURABLE_STORE_FOLDER = "durablestorefolder"

		// TLS
		TLS_CATEGORY      = "TLS:"
		TLS_CERT_FILEPATH = "tlscertfilepath"
		TLS_KEY_FILEPATH  = "tlskeyfilepath"
	)
	var pkString, chainUrl, chainAuthToken, naAddress, vpaAddress, caAddress, bridgeAddress, chainPk, durableStoreFolder, bootPeers, publicIp, extMultiAddr string
	var msgPort, wsMsgPort, rpcPort, guiPort int
	var chainStartBlock uint64
	var useNats, useDurableStore, l2 bool

	var tlsCertFilepath, tlsKeyFilepath string

	// urfave default precedence for flag value sources (highest to lowest):
	// 1. Command line flag value
	// 2. Environment variable (if specified)
	// 3. Configuration file (if specified)
	// 4. Default defined on the flag

	flags := []cli.Flag{
		&cli.StringFlag{
			Name:    CONFIG,
			Usage:   "Load config options from `config.toml`",
			EnvVars: []string{"NITRO_CONFIG_PATH"},
		},
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        USE_NATS,
			Usage:       "Specifies whether to use NATS or http/ws for the rpc server.",
			Value:       false,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &useNats,
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        L2,
			Usage:       "Specifies whether to initialize node on L2 or L1.",
			Value:       false,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &l2,
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:        USE_DURABLE_STORE,
			Usage:       "Specifies whether to use a durable store or an in-memory store.",
			Category:    STORAGE_CATEGORY,
			Value:       false,
			Destination: &useDurableStore,
		}),

		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        PK,
			Usage:       "Specifies the private key used by the nitro node.",
			Category:    KEYS_CATEGORY,
			Destination: &pkString,
			EnvVars:     []string{"SC_PK"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        CHAIN_URL,
			Usage:       "Specifies the url of a RPC endpoint for the chain.",
			Value:       "ws://127.0.0.1:8545",
			DefaultText: "hardhat / anvil default",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &chainUrl,
			EnvVars:     []string{"CHAIN_URL"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        CHAIN_AUTH_TOKEN,
			Usage:       "The bearer token used for auth when making requests to the chain's RPC endpoint.",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &chainAuthToken,
			EnvVars:     []string{"CHAIN_AUTH_TOKEN"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        CHAIN_PK,
			Usage:       "Specifies the private key to use when interacting with the chain.",
			Category:    KEYS_CATEGORY,
			Destination: &chainPk,
			EnvVars:     []string{"CHAIN_PK"},
		}),
		altsrc.NewUint64Flag(&cli.Uint64Flag{
			Name:        CHAIN_START_BLOCK,
			Usage:       "Specifies the block number to start looking for nitro adjudicator events.",
			Value:       0,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &chainStartBlock,
			EnvVars:     []string{"CHAIN_START_BLOCK"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        NA_ADDRESS,
			Usage:       "Specifies the address of the nitro adjudicator contract.",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &naAddress,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        VPA_ADDRESS,
			Usage:       "Specifies the address of the virtual payment app.",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &vpaAddress,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        CA_ADDRESS,
			Usage:       "Specifies the address of the consensus app.",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &caAddress,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        BRIDGE_ADDRESS,
			Usage:       "Specifies the address of the bridge contract.",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &bridgeAddress,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        PUBLIC_IP,
			Usage:       "Specifies the public ip used for the message service.",
			Value:       "127.0.0.1",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &publicIp,
			EnvVars:     []string{"NITRO_PUBLIC_IP"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        EXT_MULTIADDR,
			Usage:       "Additional external multiaddr to advertise",
			Value:       "",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &extMultiAddr,
		}),
		altsrc.NewIntFlag(&cli.IntFlag{
			Name:        MSG_PORT,
			Usage:       "Specifies the tcp port for the message service.",
			Value:       3005,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &msgPort,
		}),
		altsrc.NewIntFlag(&cli.IntFlag{
			Name:        WS_MSG_PORT,
			Usage:       "Specifies the websocket port for the message service.",
			Value:       6005,
			Category:    "Connectivity:",
			Destination: &wsMsgPort,
		}),
		altsrc.NewIntFlag(&cli.IntFlag{
			Name:        RPC_PORT,
			Usage:       "Specifies the tcp port for the rpc server.",
			Value:       4005,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &rpcPort,
		}),
		altsrc.NewIntFlag(&cli.IntFlag{
			Name:        GUI_PORT,
			Usage:       "Specifies the tcp port for the Nitro Connect GUI.",
			Value:       5005,
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &guiPort,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        DURABLE_STORE_FOLDER,
			Usage:       "Specifies the folder for the durable store data storage.",
			Category:    STORAGE_CATEGORY,
			Destination: &durableStoreFolder,
			Value:       "./data/nitro-store",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        BOOT_PEERS,
			Usage:       "Comma-delimited list of peer multiaddrs the messaging service will connect to when initialized.",
			Value:       "",
			Category:    CONNECTIVITY_CATEGORY,
			Destination: &bootPeers,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        TLS_CERT_FILEPATH,
			Usage:       "Filepath to the TLS certificate. If not specified, TLS will not be used with the RPC transport.",
			Value:       "./tls/statechannels.org.pem",
			Category:    TLS_CATEGORY,
			Destination: &tlsCertFilepath,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        TLS_KEY_FILEPATH,
			Usage:       "Filepath to the TLS private key. If not specified, TLS will not be used with the RPC transport.",
			Value:       "./tls/statechannels.org_key.pem",
			Category:    TLS_CATEGORY,
			Destination: &tlsKeyFilepath,
		}),
	}
	app := &cli.App{
		Name:   "go-nitro",
		Usage:  "Nitro as a service. State channel node with RPC server.",
		Flags:  flags,
		Before: altsrc.InitInputSourceWithContext(flags, altsrc.NewTomlSourceFromFlagFunc(CONFIG)),
		Action: func(cCtx *cli.Context) error {
			chainPk = utils.TrimHexPrefix(chainPk)
			pkString = utils.TrimHexPrefix(pkString)

			storeOpts := store.StoreOpts{
				PkBytes:            common.Hex2Bytes(pkString),
				UseDurableStore:    useDurableStore,
				DurableStoreFolder: durableStoreFolder,
			}

			var peerSlice []string
			if bootPeers != "" {
				peerSlice = strings.Split(bootPeers, ",")
			}

			messageOpts := p2pms.MessageOpts{
				PkBytes:      common.Hex2Bytes(pkString),
				TcpPort:      msgPort,
				WsMsgPort:    wsMsgPort,
				BootPeers:    peerSlice,
				PublicIp:     publicIp,
				ExtMultiAddr: extMultiAddr,
			}

			var node *node.Node
			var err error
			if l2 {
				chainOpts := chainservice.L2ChainOpts{
					ChainUrl:           chainUrl,
					ChainStartBlockNum: chainStartBlock,
					ChainAuthToken:     chainAuthToken,
					ChainPk:            chainPk,
					BridgeAddress:      common.HexToAddress(bridgeAddress),
					VpaAddress:         common.HexToAddress(vpaAddress),
					CaAddress:          common.HexToAddress(caAddress),
				}

				node, _, _, _, err = nodeUtils.InitializeL2Node(chainOpts, storeOpts, messageOpts)
			} else {
				chainOpts := chainservice.ChainOpts{
					ChainUrl:           chainUrl,
					ChainStartBlockNum: chainStartBlock,
					ChainAuthToken:     chainAuthToken,
					ChainPk:            chainPk,
					NaAddress:          common.HexToAddress(naAddress),
					VpaAddress:         common.HexToAddress(vpaAddress),
					CaAddress:          common.HexToAddress(caAddress),
				}

				node, _, _, _, err = nodeUtils.InitializeNode(chainOpts, storeOpts, messageOpts)
			}

			logging.SetupDefaultLogger(os.Stdout, slog.LevelDebug)

			if err != nil {
				return err
			}

			paymentsManager, err := paymentsmanager.NewPaymentsManager(node)
			if err != nil {
				return err
			}

			wg := new(sync.WaitGroup)
			defer wg.Wait()

			paymentsManager.Start(wg)
			defer func() {
				err := paymentsManager.Stop()
				if err != nil {
					panic(err)
				}
			}()

			var cert *tls.Certificate
			if tlsCertFilepath != "" && tlsKeyFilepath != "" {
				loadedCert, err := tls.LoadX509KeyPair(tlsCertFilepath, tlsKeyFilepath)
				if err != nil {
					panic(err)
				}
				cert = &loadedCert
			}

			rpcServer, err := rpc.InitializeNodeRpcServer(node, paymentsManager, rpcPort, useNats, cert)
			if err != nil {
				return err
			}

			hostNitroUI(uint(guiPort), uint(rpcPort))

			stopChan := make(chan os.Signal, 2)
			signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
			<-stopChan // wait for interrupt or terminate signal

			return rpcServer.Close()
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
