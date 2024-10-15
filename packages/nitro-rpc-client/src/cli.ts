#!/usr/bin/env ts-node
/* eslint-disable @typescript-eslint/no-empty-function */
/* eslint-disable @typescript-eslint/no-shadow */

import { readFileSync, writeFileSync } from "fs";

import yargs from "yargs/yargs";
import { hideBin } from "yargs/helpers";

import { NitroRpcClient } from "./rpc-client";
import {
  compactJson,
  prettyJson,
  getRPCUrl,
  logOutChannelUpdates,
  parseAssetData as parseAssetsData,
  parseSwapAssetsData,
} from "./utils";
import { AssetData, ConfirmSwapAction, CounterChallengeAction } from "./types";
import { ZERO_ETHEREUM_ADDRESS } from "./constants";

yargs(hideBin(process.argv))
  .scriptName("nitro-rpc-client")
  .option({
    p: { alias: "port", default: 4005, type: "number" },
    n: {
      alias: "printnotifications",
      default: false,
      type: "boolean",
      description: "Whether channel notifications are printed to the console",
    },
    h: {
      alias: "host",
      default: "127.0.0.1",
      type: "string",
      description: "Custom hostname",
    },
    s: {
      alias: "isSecure",
      default: true,
      type: "boolean",
      description: "Is it a secured connection",
    },
  })
  .command(
    "version",
    "Get the version of the Nitro RPC server",
    async () => {},
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const version = await rpcClient.GetVersion();
      console.log(version);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-node-info",
    "Get the information of the nitro node",
    async () => {},
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const nodeInfo = await rpcClient.GetNodeInfo();
      console.log(prettyJson(nodeInfo));
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "address",
    "Get the address of the Nitro RPC server",
    async () => {},
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const address = await rpcClient.GetAddress();
      console.log(address);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  // TODO: Update method to get info for all the outcomes
  .command(
    "get-all-ledger-channels",
    "Get all ledger channels",
    async () => {},
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const ledgers = await rpcClient.GetAllLedgerChannels();
      console.log(prettyJson(ledgers));
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-all-l2-channels",
    "Get all L2 channels",
    async () => {},
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const ledgers = await rpcClient.GetAllL2Channels();
      console.log(prettyJson(ledgers));
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-signed-state <channelId> <jsonFilePath>",
    "Get latest signed state",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The channel ID of the ledger channel",
          type: "string",
          demandOption: true,
        })
        .positional("jsonFilePath", {
          describe: "Path to JSON file for saving signed state",
          type: "string",
          demandOption: true,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;
      const channelId = yargs.channelId;
      const jsonFilePath = yargs.jsonFilePath;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const stringifiedSignedState = await rpcClient.GetSignedState(channelId);

      console.log(stringifiedSignedState);

      writeFileSync(jsonFilePath, stringifiedSignedState, "utf8");

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-payment-channels-by-ledger <ledgerId>",
    "Gets any payment channels funded by the given ledger",
    (yargsBuilder) => {
      return yargsBuilder.positional("ledgerId", {
        describe: "The id of the ledger channel to defund",
        type: "string",
        demandOption: true,
      });
    },

    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const paymentChans = await rpcClient.GetPaymentChannelsByLedger(
        yargs.ledgerId
      );
      for (const p of paymentChans) {
        console.log(`${compactJson(p)}`);
      }
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-objective <objectiveId>",
    "Get current status of objective with given objective ID",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("objectiveId", {
          describe: "ID of the objective",
          type: "string",
          demandOption: true,
        })
        .option("l2", {
          describe: "Whether the passed objective is on L2",
          type: "boolean",
          default: false,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const objectiveId = yargs.objectiveId;
      const l2 = yargs.l2;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const objectiveInfo = await rpcClient.GetObjective(objectiveId, l2);
      prettyJson(objectiveInfo);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-l2-objective-from-l1 <l1ObjectiveId>",
    "Get current status of objective with given objective ID",
    (yargsBuilder) => {
      return yargsBuilder.positional("l1ObjectiveId", {
        describe: "ID of the objective",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const l1ObjectiveId = yargs.l1ObjectiveId;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const objectiveInfo = await rpcClient.GetL2ObjectiveFromL1(l1ObjectiveId);
      prettyJson(objectiveInfo);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-pending-bridge-txs <channelId>",
    "Get current status of objective with given objective ID",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "Channel ID to get events for",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const channelId = yargs.channelId;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const pendingBridgeTxs = await rpcClient.GetPendingBridgeTxs(channelId);
      console.log(pendingBridgeTxs);

      await rpcClient.Close();
      process.exit(0);
    }
  )

  .command(
    "direct-fund <counterparty>",
    "Creates a directly funded ledger channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("counterparty", {
          describe: "The counterparty's address",
          type: "string",
          demandOption: true,
        })
        .option("assetAddress", {
          describe: "Address of the token to be used",
          type: "string",
          default: ZERO_ETHEREUM_ADDRESS,
        })
        .option("alphaAmount", {
          describe: "The amount to be funded by alpha node",
          type: "number",
          default: 1_000_000,
        })
        .option("betaAmount", {
          describe: "The amount to be funded by beta node",
          type: "number",
          default: 1_000_000,
        })
        .option("asset", {
          describe:
            "Asset data in the format '0xAddress:alphaAmount,betaAmount'",
          type: "array",
          string: true,
        })
        .option("challengeDuration", {
          describe:
            "The duration (in seconds) of the challenge-response window",
          type: "number",
          default: 10,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      let assetsData: AssetData[] = [];

      if (!yargs.asset) {
        assetsData = [
          {
            assetAddress: yargs.assetAddress,
            alphaAmount: yargs.alphaAmount,
            betaAmount: yargs.betaAmount,
          },
        ];
      } else {
        assetsData = parseAssetsData(yargs.asset);
      }

      const dfObjective = await rpcClient.CreateLedgerChannel(
        yargs.counterparty,
        assetsData,
        yargs.challengeDuration
      );
      const { Id, ChannelId } = dfObjective;

      console.log(`Objective started ${Id}`);
      await rpcClient.WaitForLedgerChannelStatus(ChannelId, "Open");
      console.log(`Channel Open ${ChannelId}`);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "direct-defund <channelId>",
    "Defunds a directly funded ledger channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The id of the ledger channel to defund",
          type: "string",
          demandOption: true,
        })
        .option("isChallenge", {
          describe: "To initiate challenge transaction",
          type: "boolean",
          default: false,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const id = await rpcClient.CloseLedgerChannel(
        yargs.channelId,
        yargs.isChallenge
      );
      console.log(`Objective started ${id}`);
      // Not using WaitForLedgerChannelStatus method with complete status, as the ledger channel status will be open if a challenge is cleared using checkpoint
      await rpcClient.WaitForObjectiveToComplete(
        `DirectDefunding-${yargs.channelId}`
      );
      console.log(`Objective Complete ${yargs.channelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "bridged-defund <channelId>",
    "Defunds a mirror ledger channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The id of mirror ledger channel to defund",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const id = await rpcClient.CloseBridgeChannel(yargs.channelId);
      console.log(`Objective started ${id}`);
      await rpcClient.WaitForObjectiveToComplete(
        `bridgeddefunding-${yargs.channelId}`
      );
      console.log(`Objective Complete ${yargs.channelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "mirror-bridged-defund <channelId> <l2SignedStateFilePath>",
    "Defunds a mirror ledger channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The id of ledger channel to call challenge on",
          type: "string",
          demandOption: true,
        })
        .positional("l2SignedStateFilePath", {
          describe: "Path to JSON file containing L2 signed state",
          type: "string",
          demandOption: true,
        })
        .option("isChallenge", {
          describe: "To initiate challenge transaction",
          type: "boolean",
          default: false,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;
      const isChallenge = yargs.isChallenge;
      const l2SignedStateFilePath = yargs.l2SignedStateFilePath;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const stringifiedL2SignedState = readFileSync(
        l2SignedStateFilePath,
        "utf8"
      );

      const id = await rpcClient.MirrorBridgedDefund(
        yargs.channelId,
        stringifiedL2SignedState,
        isChallenge
      );

      console.log(`Objective started ${id}`);

      await rpcClient.WaitForObjectiveToComplete(
        `mirrorbridgeddefunding-${yargs.channelId}`
      );

      console.log(`Objective Complete ${yargs.channelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "swap-fund <counterparty> [intermediaries...]",
    "Creates a swap channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("counterparty", {
          describe: "The counterparty's address",
          type: "string",
          demandOption: true,
        })
        .array("intermediaries")
        .option("asset", {
          describe:
            "Asset data in the format '0xAddress:alphaAmount,betaAmount'",
          type: "array",
          string: true,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      // Parse all intermediary args to strings
      const intermediaries =
        yargs.intermediaries?.map((intermediary) => {
          if (typeof intermediary === "string") {
            return intermediary;
          }
          return intermediary.toString(16);
        }) ?? [];

      if (!yargs.asset) {
        throw new Error("assets are required");
      }

      const assets = yargs.asset.map((a) => {
        if (typeof a === "string") {
          return a;
        }
        throw new Error("Incorrect asset type");
      });

      const assetsData = parseAssetsData(assets);

      const sfObjective = await rpcClient.CreateSwapChannel(
        yargs.counterparty,
        intermediaries,
        assetsData
      );

      const { Id, ChannelId } = sfObjective;
      console.log(`Objective started ${Id}`);

      await rpcClient.WaitForObjectiveToComplete(Id);
      console.log(`Channel open ${ChannelId}`);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "swap-defund <channelId>",
    "Defunds a swap channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The id of the swap channel to defund",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      if (yargs.n) logOutChannelUpdates(rpcClient);

      const id = await rpcClient.CloseSwapChannel(yargs.channelId);

      console.log(`Objective started ${id}`);

      await rpcClient.WaitForObjectiveToComplete(id);
      console.log(`Objective complete ${id}`);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "virtual-fund <counterparty> [intermediaries...]",
    "Creates a virtually funded payment channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("counterparty", {
          describe: "The counterparty's address",
          type: "string",
          demandOption: true,
        })
        .array("intermediaries")
        .option("amount", {
          describe: "The amount to fund the channel with",
          type: "number",
          default: 1000,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      // Parse all intermediary args to strings
      const intermediaries =
        yargs.intermediaries?.map((intermediary) => {
          if (typeof intermediary === "string") {
            return intermediary;
          }
          return intermediary.toString(16);
        }) ?? [];

      const vfObjective = await rpcClient.CreatePaymentChannel(
        yargs.counterparty,
        intermediaries,
        yargs.amount
      );

      const { ChannelId, Id } = vfObjective;
      console.log(`Objective started ${Id}`);
      await rpcClient.WaitForPaymentChannelStatus(ChannelId, "Open");
      console.log(`Channel Open ${ChannelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "virtual-defund <channelId>",
    "Defunds a virtually funded payment channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The id of the payment channel to defund",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      if (yargs.n) logOutChannelUpdates(rpcClient);

      const id = await rpcClient.ClosePaymentChannel(yargs.channelId);

      console.log(`Objective started ${id}`);
      await rpcClient.WaitForPaymentChannelStatus(yargs.channelId, "Complete");
      console.log(`Channel complete ${yargs.channelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-ledger-channel <channelId>",
    "Gets information about a ledger channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The channel ID of the ledger channel",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      const ledgerInfo = await rpcClient.GetLedgerChannel(yargs.channelId);
      console.log(ledgerInfo);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-payment-channel <channelId>",
    "Gets information about a payment channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The channel ID of the payment channel",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const paymentChannelInfo = await rpcClient.GetPaymentChannel(
        yargs.channelId
      );
      console.log(paymentChannelInfo);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-swap-channel <channelId>",
    "Gets information about a swap channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The channel ID of the payment channel",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      const swapChannelInfo = await rpcClient.GetSwapChannel(yargs.channelId);
      console.log(swapChannelInfo);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "pay <channelId> <amount>",
    "Sends a payment on the given channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The channel ID of the payment channel",
          type: "string",
          demandOption: true,
        })
        .positional("amount", {
          describe: "The amount to pay",
          type: "number",
          demandOption: true,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const paymentChannelInfo = await rpcClient.Pay(
        yargs.channelId,
        yargs.amount
      );
      console.log(paymentChannelInfo);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-pending-swap <channelId>",
    "Gets the pending swap awaiting confirmation for this swap channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The channel ID of the payment channel",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      // TODO: Generate JSON schema for get current swap
      const pendingSwap = await rpcClient.GetPendingSwap(yargs.channelId);
      console.log(JSON.parse(pendingSwap));
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "swap-initiate <channelId>",
    "Swaps assets on a given swap channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The channel ID of the payment channel",
          type: "string",
          demandOption: true,
        })
        .option("AssetIn", {
          describe: "Input asset details in the format of '0xAddress:amount'",
          type: "string",
          demandOption: true,
        })
        .option("AssetOut", {
          describe: "Output asset details in the format of '0xAddress:amount'",
          type: "string",
          demandOption: true,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const swapAssetsData = parseSwapAssetsData(yargs.AssetIn, yargs.AssetOut);

      const swapInfo = await rpcClient.SwapAssets(
        yargs.channelId,
        swapAssetsData
      );

      console.log(swapInfo);
      const confirmedSwap = await rpcClient.WaitForSwapStatus(yargs.channelId);
      console.log("Response:\n", confirmedSwap);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  // TODO: Separate out methods to accept and reject swaps
  .command(
    "swap-accept <SwapId>",
    "Accept the received swap",
    (yargsBuilder) => {
      return yargsBuilder.positional("SwapId", {
        describe: "The Id of swap",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const response = await rpcClient.ConfirmSwap(
        yargs.SwapId,
        ConfirmSwapAction.accepted
      );

      console.log(`Confirming Swap with ${response.Action}`);
      await rpcClient.WaitForObjectiveToComplete(`Swap-${yargs.SwapId}`);
      console.log(`Objective complete Swap-${yargs.SwapId}`);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "swap-reject <SwapId>",
    "Reject the received swap",
    (yargsBuilder) => {
      return yargsBuilder.positional("SwapId", {
        describe: "The Id of swap",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const response = await rpcClient.ConfirmSwap(
        yargs.SwapId,
        ConfirmSwapAction.rejected
      );

      console.log(`Confirming Swap with ${response.Action}`);
      await rpcClient.WaitForObjectiveToComplete(`Swap-${yargs.SwapId}`);
      console.log(`Objective complete Swap-${yargs.SwapId}`);

      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "retry-objective-tx <objectiveId>",
    "Retries transaction for given objective",
    (yargsBuilder) => {
      return yargsBuilder.positional("objectiveId", {
        describe: "The id of the objective to send transaction for",
        type: "string",
        demandOption: true,
      });
    },

    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      const id = await rpcClient.RetryObjectiveTx(yargs.objectiveId);

      console.log(`Transaction retried for objective ${id}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "retry-tx <txHash>",
    "Retries transaction with given transaction hash",
    (yargsBuilder) => {
      return yargsBuilder.positional("txHash", {
        describe: "Hash of transaction to retry",
        type: "string",
        demandOption: true,
      });
    },

    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      const txHash = await rpcClient.RetryTx(yargs.txHash);

      console.log(`Transaction with hash ${txHash} retried`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "get-voucher <channelId>",
    "Get largest voucher paid/received on the payment channel",
    (yargsBuilder) => {
      return yargsBuilder.positional("channelId", {
        describe: "The channel ID of the payment channel",
        type: "string",
        demandOption: true,
      });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );

      const voucher = await rpcClient.GetVoucher(yargs.channelId);
      console.log(voucher);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "counter-challenge <channelId> <action>",
    "Counter challenge the registered challenge",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The channel ID of the payment channel",
          type: "string",
          demandOption: true,
        })
        .positional("action", {
          describe: "The action to take",
          type: "string",
          choices: ["checkpoint", "challenge"],
          demandOption: true,
        })
        .option("l2SignedStateFilePath", {
          describe: "Path to JSON file containing L2 signed state",
          type: "string",
          demandOption: false,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;
      const l2SignedStateFilePath = yargs.l2SignedStateFilePath;
      let stringifiedL2SignedState: string | undefined;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      if (l2SignedStateFilePath) {
        stringifiedL2SignedState = readFileSync(l2SignedStateFilePath, "utf8");
      }

      const response = await rpcClient.CounterChallenge(
        yargs.channelId,
        CounterChallengeAction[
          yargs.action as keyof typeof CounterChallengeAction
        ],
        stringifiedL2SignedState
      );
      console.log(
        `Sending ${response.Action} transaction for channel ${response.ChannelId}`
      );

      const waitForDirectDefund = rpcClient.WaitForObjectiveToComplete(
        `DirectDefunding-${yargs.channelId}`
      );
      const waitForMirrorDefund = rpcClient.WaitForObjectiveToComplete(
        `mirrorbridgeddefunding-${yargs.channelId}`
      );
      await Promise.race([waitForDirectDefund, waitForMirrorDefund]);
      console.log(`Objective Complete ${response.ChannelId}`);
      await rpcClient.Close();
      process.exit(0);
    }
  )
  .command(
    "create-voucher <channelId> <amount>",
    "Create a payment on the given channel",
    (yargsBuilder) => {
      return yargsBuilder
        .positional("channelId", {
          describe: "The channel ID of the payment channel",
          type: "string",
          demandOption: true,
        })
        .positional("amount", {
          describe: "The amount to pay",
          type: "number",
          demandOption: true,
        });
    },
    async (yargs) => {
      const rpcPort = yargs.p;
      const rpcHost = yargs.h;
      const isSecure = yargs.s;

      const rpcClient = await NitroRpcClient.CreateHttpNitroClient(
        getRPCUrl(rpcHost, rpcPort),
        isSecure
      );
      if (yargs.n) logOutChannelUpdates(rpcClient);

      const voucher = await rpcClient.CreateVoucher(
        yargs.channelId,
        yargs.amount
      );
      console.log(voucher);
      await rpcClient.Close();
      process.exit(0);
    }
  )

  .demandCommand(1, "You need at least one command before moving on")
  .parserConfiguration({ "parse-numbers": false })
  .strict()
  .parse();
