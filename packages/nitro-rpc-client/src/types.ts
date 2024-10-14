/**
 * JSON RPC Types
 */
export type JsonRpcRequest<MethodName extends RequestMethod, RequestPayload> = {
  id: number; // in the json-rpc spec this is optional, but we require it for all our requests
  jsonrpc: "2.0";
  method: MethodName;
  params: { authtoken: string; payload: RequestPayload };
};
export type JsonRpcResponse<ResultType> = {
  id: number;
  jsonrpc: "2.0";
  result: ResultType;
};

export type JsonRpcNotification<NotificationName, NotificationPayload> = {
  jsonrpc: "2.0";
  method: NotificationName;
  params: { payload: NotificationPayload };
};

export type JsonRpcError<Code, Message, Data = undefined> = {
  id: number;
  jsonrpc: "2.0";
  error: Data extends undefined
    ? { code: Code; message: Message }
    : { code: Code; message: Message; data: Data };
};

/**
 * Objective payloads and responses
 */
export type DirectFundPayload = {
  CounterParty: string;
  ChallengeDuration: number;
  Outcome: Outcome;
  Nonce: number;
  AppDefinition: string;
  AppData: string;
};
export type VirtualFundPayload = {
  Intermediaries: string[];
  CounterParty: string;
  ChallengeDuration: number;
  Outcome: Outcome;
  Nonce: number;
  AppDefinition: string;
};
export type SwapFundPayload = {
  Intermediaries: string[];
  CounterParty: string;
  ChallengeDuration: number;
  Outcome: Outcome;
  Nonce: number;
  AppDefinition: string;
};
export type PaymentPayload = {
  // todo: this should be a bigint
  Amount: number;
  Channel: string;
};

export type SwapInitiatePayload = {
  SwapAssetsData: SwapAssetsData;
  Channel: string;
};

export enum ConfirmSwapAction {
  accepted = 1,
  rejected,
}

export type ConfirmSwapResult = {
  SwapId: string;
  Action: keyof typeof ConfirmSwapAction;
};

export type ConfirmSwapPayload = {
  SwapId: string;
  Action: ConfirmSwapAction;
};

export enum CounterChallengeAction {
  checkpoint,
  challenge,
}

export type CounterChallengePayload = {
  ChannelId: string;
  Action: CounterChallengeAction;
  StringifiedL2SignedState?: string;
};

export type CounterChallengeResult = {
  ChannelId: string;
  Action: keyof typeof CounterChallengeAction;
};

export interface Balance {
  AssetAddress: string;
  Me: string;
  Them: string;
  MyBalance: string | bigint;
  TheirBalance: string | bigint;
}

export interface SwapChannelInfo {
  ID: string;
  Status: string;
  Balances: Balance[];
}

export type Voucher = {
  ChannelId: string;
  // todo: this should be a bigint
  Amount: number;

  Signature: string;
};
type GetChannelRequest = {
  Id: string;
};

type GetByLedgerRequest = {
  LedgerId: string;
};
export type DefundObjectiveRequest = {
  ChannelId: string;
};
export type DirectDefundObjectiveRequest = DefundObjectiveRequest & {
  IsChallenge: boolean;
};
export type MirrorBridgedDefundObjectiveRequest =
  DirectDefundObjectiveRequest & {
    StringifiedL2SignedState: string;
  };
export type ObjectiveResponse = {
  Id: string;
  ChannelId: string;
};
export type ReceiveVoucherResult = {
  Total: bigint;
  Delta: bigint;
};
export type GetPendingSwap = {
  Id: string;
};

/**
 * RPC Requests
 */
export type GetAuthTokenRequest = JsonRpcRequest<
  "get_auth_token",
  Record<string, never>
>;
export type GetAddressRequest = JsonRpcRequest<
  "get_address",
  Record<string, never>
>;
export type DirectFundRequest = JsonRpcRequest<
  "create_ledger_channel",
  DirectFundPayload
>;
export type RetryObjectiveTxRequest = JsonRpcRequest<
  "retry_objective_tx",
  {
    ObjectiveId: string;
  }
>;
export type RetryTxRequest = JsonRpcRequest<
  "retry_tx",
  {
    TxHash: string;
  }
>;
export type PaymentRequest = JsonRpcRequest<"pay", PaymentPayload>;

export type SwapRequest = JsonRpcRequest<"swap_initiate", SwapInitiatePayload>;

export type ConfirmSwapRequest = JsonRpcRequest<
  "confirm_swap",
  ConfirmSwapPayload
>;

export type CounterChallengeRequest = JsonRpcRequest<
  "counter_challenge",
  CounterChallengePayload
>;

export type VirtualFundRequest = JsonRpcRequest<
  "create_payment_channel",
  VirtualFundPayload
>;
export type SwapFundRequest = JsonRpcRequest<
  "create_swap_channel",
  SwapFundPayload
>;
export type GetLedgerChannelRequest = JsonRpcRequest<
  "get_ledger_channel",
  GetChannelRequest
>;
export type GetAllLedgerChannelsRequest = JsonRpcRequest<
  "get_all_ledger_channels",
  Record<string, never>
>;
export type GetSignedStateRequest = JsonRpcRequest<
  "get_signed_state",
  GetChannelRequest
>;
export type GetPaymentChannelRequest = JsonRpcRequest<
  "get_payment_channel",
  GetChannelRequest
>;
export type GetSwapChannelRequest = JsonRpcRequest<
  "get_swap_channel",
  GetChannelRequest
>;
export type GetVoucherRequest = JsonRpcRequest<
  "get_voucher",
  GetChannelRequest
>;
export type GetPendingSwapRequest = JsonRpcRequest<
  "get_pending_swap",
  GetPendingSwap
>;
export type GetPaymentChannelsByLedgerRequest = JsonRpcRequest<
  "get_payment_channels_by_ledger",
  GetByLedgerRequest
>;

export type GetObjectiveRequest = JsonRpcRequest<
  "get_objective",
  {
    ObjectiveId: string;
    L2: boolean;
  }
>;

export type GetPendingBridgeTxsRequest = JsonRpcRequest<
  "get_pending_bridge_txs",
  {
    ChannelId: string;
  }
>;

export type GetL2ObjectiveFromL1Request = JsonRpcRequest<
  "get_l2_objective_from_l1",
  {
    L1ObjectiveId: string;
  }
>;

export type VersionRequest = JsonRpcRequest<"version", Record<string, never>>;
export type DirectDefundRequest = JsonRpcRequest<
  "close_ledger_channel",
  DirectDefundObjectiveRequest
>;
export type BridgedDefundRequest = JsonRpcRequest<
  "close_bridge_channel",
  DefundObjectiveRequest
>;
export type MirrorBridgedDefundRequest = JsonRpcRequest<
  "mirror_bridged_defund",
  MirrorBridgedDefundObjectiveRequest
>;
export type VirtualDefundRequest = JsonRpcRequest<
  "close_payment_channel",
  DefundObjectiveRequest
>;

export type SwapDefundRequest = JsonRpcRequest<
  "close_swap_channel",
  DefundObjectiveRequest
>;

export type CreateVoucherRequest = JsonRpcRequest<
  "create_voucher",
  PaymentPayload
>;

export type ReceiveVoucherRequest = JsonRpcRequest<"receive_voucher", Voucher>;

export type GetNodeInfoRequest = JsonRpcRequest<
  "get_node_info",
  Record<string, never>
>;

/**
 * RPC Responses
 */
export type GetAuthTokenResponse = JsonRpcResponse<string>;
export type GetPaymentChannelResponse = JsonRpcResponse<PaymentChannelInfo>;
export type GetSwapChannelResponse = JsonRpcResponse<SwapChannelInfo>;
export type GetVoucherResponse = JsonRpcResponse<Voucher>;
export type GetPendingSwapResponse = JsonRpcResponse<string>;
export type PaymentResponse = JsonRpcResponse<PaymentPayload>;
export type SwapResponse = JsonRpcResponse<SwapInitiatePayload>;
export type CounterChallengeResponse = JsonRpcResponse<CounterChallengeResult>;
export type ConfirmSwapResponse = JsonRpcResponse<ConfirmSwapResult>;
export type GetLedgerChannelResponse = JsonRpcResponse<LedgerChannelInfo>;
export type VirtualFundResponse = JsonRpcResponse<ObjectiveResponse>;
export type SwapFundResponse = JsonRpcResponse<ObjectiveResponse>;
export type VersionResponse = JsonRpcResponse<string>;
export type GetNodeInfoResponse = JsonRpcResponse<GetNodeInfo>;
export type GetAddressResponse = JsonRpcResponse<string>;
export type DirectFundResponse = JsonRpcResponse<ObjectiveResponse>;
export type RetryObjectiveTxResponse = JsonRpcResponse<string>;
export type RetryTxResponse = JsonRpcResponse<string>;
export type DirectDefundResponse = JsonRpcResponse<string>;
export type MirrorBridgedDefundResponse = JsonRpcResponse<string>;
export type BridgedDefundResponse = JsonRpcResponse<string>;
export type VirtualDefundResponse = JsonRpcResponse<string>;
export type SwapDefundResponse = JsonRpcResponse<string>;
export type GetAllLedgerChannelsResponse = JsonRpcResponse<LedgerChannelInfo[]>;
export type GetSignedStateResponse = JsonRpcResponse<string>;
export type GetPaymentChannelsByLedgerResponse = JsonRpcResponse<
  PaymentChannelInfo[]
>;
export type GetObjectiveResponse = JsonRpcResponse<string>;
export type GetL2ObjectiveFromL1Response = JsonRpcResponse<string>;
export type GetPendingBridgeTxsResponse = JsonRpcResponse<string>;
export type CreateVoucherResponse = JsonRpcResponse<Voucher>;
export type ReceiveVoucherResponse = JsonRpcResponse<ReceiveVoucherResult>;
/**
 * RPC Request/Response map
 * This is a map of all the RPC methods to their request and response types
 */
export type RPCRequestAndResponses = {
  get_auth_token: [GetAuthTokenRequest, GetAuthTokenResponse];
  create_ledger_channel: [DirectFundRequest, DirectFundResponse];
  retry_objective_tx: [RetryObjectiveTxRequest, RetryObjectiveTxResponse];
  retry_tx: [RetryTxRequest, RetryTxResponse];
  close_ledger_channel: [DirectDefundRequest, DirectDefundResponse];
  close_bridge_channel: [BridgedDefundRequest, BridgedDefundResponse];
  mirror_bridged_defund: [
    MirrorBridgedDefundRequest,
    MirrorBridgedDefundResponse
  ];
  version: [VersionRequest, VersionResponse];
  get_node_info: [GetNodeInfoRequest, GetNodeInfoResponse];
  create_payment_channel: [VirtualFundRequest, VirtualFundResponse];
  create_swap_channel: [SwapFundRequest, SwapFundResponse];
  get_address: [GetAddressRequest, GetAddressResponse];
  get_ledger_channel: [GetLedgerChannelRequest, GetLedgerChannelResponse];
  get_payment_channel: [GetPaymentChannelRequest, GetPaymentChannelResponse];
  get_swap_channel: [GetSwapChannelRequest, GetSwapChannelResponse];
  get_voucher: [GetVoucherRequest, GetVoucherResponse];
  get_pending_swap: [GetPendingSwapRequest, GetPendingSwapResponse];
  pay: [PaymentRequest, PaymentResponse];
  swap_initiate: [SwapRequest, SwapResponse];
  confirm_swap: [ConfirmSwapRequest, ConfirmSwapResponse];
  counter_challenge: [CounterChallengeRequest, CounterChallengeResponse];
  close_payment_channel: [VirtualDefundRequest, VirtualDefundResponse];
  close_swap_channel: [SwapDefundRequest, SwapDefundResponse];
  get_all_ledger_channels: [
    GetAllLedgerChannelsRequest,
    GetAllLedgerChannelsResponse
  ];
  get_all_l2_channels: [
    GetAllLedgerChannelsRequest,
    GetAllLedgerChannelsResponse
  ];
  get_signed_state: [GetSignedStateRequest, GetSignedStateResponse];
  get_payment_channels_by_ledger: [
    GetPaymentChannelsByLedgerRequest,
    GetPaymentChannelsByLedgerResponse
  ];
  get_objective: [GetObjectiveRequest, GetObjectiveResponse];
  get_l2_objective_from_l1: [
    GetL2ObjectiveFromL1Request,
    GetL2ObjectiveFromL1Response
  ];
  get_pending_bridge_txs: [
    GetPendingBridgeTxsRequest,
    GetPendingBridgeTxsResponse
  ];
  create_voucher: [CreateVoucherRequest, CreateVoucherResponse];
  receive_voucher: [ReceiveVoucherRequest, ReceiveVoucherResponse];
};

export type RequestMethod = keyof RPCRequestAndResponses;

export type RPCRequest =
  RPCRequestAndResponses[keyof RPCRequestAndResponses][0];
export type RPCResponse =
  RPCRequestAndResponses[keyof RPCRequestAndResponses][1];

/**
 * RPC Notifications
 */
export type RPCNotification =
  | ObjectiveCompleteNotification
  | PaymentChannelUpdatedNotification
  | SwapUpdatedNotification
  | LedgerChannelUpdatedNotification;
export type NotificationMethod = RPCNotification["method"];
export type NotificationParams = RPCNotification["params"];
export type PaymentChannelUpdatedNotification = JsonRpcNotification<
  "payment_channel_updated",
  PaymentChannelInfo
>;

export type SwapUpdatedNotification = JsonRpcNotification<
  "swap_updated",
  SwapInfo
>;

export type LedgerChannelUpdatedNotification = JsonRpcNotification<
  "ledger_channel_updated",
  LedgerChannelInfo
>;

export type ObjectiveCompleteNotification = JsonRpcNotification<
  "objective_completed",
  string
>;

/**
 * Outcome related types
 */
export type LedgerChannelInfo = {
  ID: string;
  Status: ChannelStatus;
  Balances: LedgerChannelBalance[];
  ChannelMode: keyof typeof ChannelMode;
};

export type LedgerChannelState = {
  Participants: string[];
  ChannelNonce: number;
  AppDefinition: string;
  ChallengeDuration: number;
  AppData: string;
  Outcome: Outcome;
  TurnNum: number;
  IsFinal: boolean;
};

export type Signature = {
  r: string;
  s: string;
  v: number;
};

export type SignedState = {
  state: LedgerChannelState;
  sigs: { [key: number]: Signature };
};

export type LedgerChannelBalance = {
  AssetAddress: string;
  Them: string;
  Me: string;
  TheirBalance: bigint;
  MyBalance: bigint;
};

export type PaymentChannelBalance = {
  AssetAddress: string;
  Payee: string;
  Payer: string;
  PaidSoFar: bigint;
  RemainingFunds: bigint;
};

export type SwapChannelBalance = {
  AssetAddress: string;
  Me: string;
  Them: string;
  MyBalance: bigint;
  TheirBalance: bigint;
};

export type PaymentChannelInfo = {
  ID: string;
  Status: ChannelStatus;
  Balance: PaymentChannelBalance;
};

export type SwapInfo = {
  Id: string;
  Status: SwapStatus;
};

export type Outcome = SingleAssetOutcome[];

export type SingleAssetOutcome = {
  Asset: string;
  AssetMetadata: AssetMetadata;
  Allocations: Allocation[];
};

export type Allocation = {
  Destination: string;
  Amount: number;
  AllocationType: number;
  Metadata: null;
};
export type AssetMetadata = {
  AssetType: number;
  Metadata: null;
};

export type ChannelStatus = "Proposed" | "Open" | "Closing" | "Complete";

export enum SwapStatus {
  PendingConfirmation,
  Accepted,
  Rejected,
}

export enum ChannelMode {
  Open,
  Challenge,
  Finalized,
}

export type GetNodeInfo = {
  SCAddress: string;
  MessageServicePeerId: string;
};

export type AssetData = {
  assetAddress: string;
  alphaAmount: number;
  betaAmount: number;
};

export type SwapAssetsData = {
  TokenIn: string;
  TokenOut: string;
  AmountIn: number;
  AmountOut: number;
};
