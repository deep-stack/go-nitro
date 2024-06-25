// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package Bridge

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// BridgeMetaData contains all meta data concerning the Bridge contract.
var BridgeMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"l2ChannelId\",\"type\":\"bytes32\"}],\"name\":\"getL2ToL1\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"channelId\",\"type\":\"bytes32\"}],\"name\":\"getMirroredChannelStatus\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"l2Tol1\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"l1ChannelId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"l2ChannelId\",\"type\":\"bytes32\"}],\"name\":\"setL2ToL1\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"statusOf\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"channelId\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"stateHash\",\"type\":\"bytes32\"},{\"internalType\":\"bytes32\",\"name\":\"outcomeHash\",\"type\":\"bytes32\"}],\"name\":\"updateMirroredChannelStatus\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x6080806040523461002857600280546001600160a01b03191633179055610452908161002e8239f35b600080fdfe6080604081815260048036101561001557600080fd5b600092833560e01c908163486f14d1146103d45750806377027728146103935780638a7ca6641461034e5780638da5cb5b146102f7578063c7df14e2146102b3578063d3a50807146100b75763e34e51801461007057600080fd5b346100b35760207ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126100b357602092829135815280845220549051908152f35b8280fd5b50346100b35760607ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126100b3578035906024356044359073ffffffffffffffffffffffffffffffffffffffff918260025416330361023057848752866020528587205493865190608082019167ffffffffffffffff92818110848211176102045784916060918b528860d01c815265ffffffffffff8960a01c166020820152868b820152015287519260208401948552888401528783526060830191838310908311176101d85750865251902016918452836020527fffffffffffff000000000000000000000000000000000000000000000000000079ffffffffffff00000000000000000000000000000000000000008216911617179082205580f35b8860416024927f4e487b7100000000000000000000000000000000000000000000000000000000835252fd5b60248b6041857f4e487b7100000000000000000000000000000000000000000000000000000000835252fd5b60848460208851917f08c379a0000000000000000000000000000000000000000000000000000000008352820152603d60248201527f4f776e65727368697020417373657274696f6e3a2043616c6c6572206f66207460448201527f68652066756e6374696f6e206973206e6f7420746865206f776e65722e0000006064820152fd5b50346100b35760207ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126100b357602092829135815280845220549051908152f35b50503461034a57817ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc36011261034a5760209073ffffffffffffffffffffffffffffffffffffffff600254169051908152f35b5080fd5b50346100b35760207ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126100b35760209282913581526001845220549051908152f35b50346100b357817ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc3601126100b35760243583526001602052359082205580f35b92919050346104185760207ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffc36011261041857926020933581526001845220548152f35b8380fdfea2646970667358221220991462e400674131dfe749d3ba2b1a83f03321997540f77c02aa78d7a4c189cf64736f6c63430008110033",
}

// BridgeABI is the input ABI used to generate the binding from.
// Deprecated: Use BridgeMetaData.ABI instead.
var BridgeABI = BridgeMetaData.ABI

// BridgeBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use BridgeMetaData.Bin instead.
var BridgeBin = BridgeMetaData.Bin

// DeployBridge deploys a new Ethereum contract, binding an instance of Bridge to it.
func DeployBridge(auth *bind.TransactOpts, backend bind.ContractBackend) (common.Address, *types.Transaction, *Bridge, error) {
	parsed, err := BridgeMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(BridgeBin), backend)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &Bridge{BridgeCaller: BridgeCaller{contract: contract}, BridgeTransactor: BridgeTransactor{contract: contract}, BridgeFilterer: BridgeFilterer{contract: contract}}, nil
}

// Bridge is an auto generated Go binding around an Ethereum contract.
type Bridge struct {
	BridgeCaller     // Read-only binding to the contract
	BridgeTransactor // Write-only binding to the contract
	BridgeFilterer   // Log filterer for contract events
}

// BridgeCaller is an auto generated read-only Go binding around an Ethereum contract.
type BridgeCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeTransactor is an auto generated write-only Go binding around an Ethereum contract.
type BridgeTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type BridgeFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// BridgeSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type BridgeSession struct {
	Contract     *Bridge           // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// BridgeCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type BridgeCallerSession struct {
	Contract *BridgeCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// BridgeTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type BridgeTransactorSession struct {
	Contract     *BridgeTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// BridgeRaw is an auto generated low-level Go binding around an Ethereum contract.
type BridgeRaw struct {
	Contract *Bridge // Generic contract binding to access the raw methods on
}

// BridgeCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type BridgeCallerRaw struct {
	Contract *BridgeCaller // Generic read-only contract binding to access the raw methods on
}

// BridgeTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type BridgeTransactorRaw struct {
	Contract *BridgeTransactor // Generic write-only contract binding to access the raw methods on
}

// NewBridge creates a new instance of Bridge, bound to a specific deployed contract.
func NewBridge(address common.Address, backend bind.ContractBackend) (*Bridge, error) {
	contract, err := bindBridge(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &Bridge{BridgeCaller: BridgeCaller{contract: contract}, BridgeTransactor: BridgeTransactor{contract: contract}, BridgeFilterer: BridgeFilterer{contract: contract}}, nil
}

// NewBridgeCaller creates a new read-only instance of Bridge, bound to a specific deployed contract.
func NewBridgeCaller(address common.Address, caller bind.ContractCaller) (*BridgeCaller, error) {
	contract, err := bindBridge(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &BridgeCaller{contract: contract}, nil
}

// NewBridgeTransactor creates a new write-only instance of Bridge, bound to a specific deployed contract.
func NewBridgeTransactor(address common.Address, transactor bind.ContractTransactor) (*BridgeTransactor, error) {
	contract, err := bindBridge(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &BridgeTransactor{contract: contract}, nil
}

// NewBridgeFilterer creates a new log filterer instance of Bridge, bound to a specific deployed contract.
func NewBridgeFilterer(address common.Address, filterer bind.ContractFilterer) (*BridgeFilterer, error) {
	contract, err := bindBridge(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &BridgeFilterer{contract: contract}, nil
}

// bindBridge binds a generic wrapper to an already deployed contract.
func bindBridge(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := BridgeMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Bridge *BridgeRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Bridge.Contract.BridgeCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Bridge *BridgeRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Bridge.Contract.BridgeTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Bridge *BridgeRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Bridge.Contract.BridgeTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_Bridge *BridgeCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _Bridge.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_Bridge *BridgeTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _Bridge.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_Bridge *BridgeTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _Bridge.Contract.contract.Transact(opts, method, params...)
}

// GetL2ToL1 is a free data retrieval call binding the contract method 0x8a7ca664.
//
// Solidity: function getL2ToL1(bytes32 l2ChannelId) view returns(bytes32)
func (_Bridge *BridgeCaller) GetL2ToL1(opts *bind.CallOpts, l2ChannelId [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Bridge.contract.Call(opts, &out, "getL2ToL1", l2ChannelId)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetL2ToL1 is a free data retrieval call binding the contract method 0x8a7ca664.
//
// Solidity: function getL2ToL1(bytes32 l2ChannelId) view returns(bytes32)
func (_Bridge *BridgeSession) GetL2ToL1(l2ChannelId [32]byte) ([32]byte, error) {
	return _Bridge.Contract.GetL2ToL1(&_Bridge.CallOpts, l2ChannelId)
}

// GetL2ToL1 is a free data retrieval call binding the contract method 0x8a7ca664.
//
// Solidity: function getL2ToL1(bytes32 l2ChannelId) view returns(bytes32)
func (_Bridge *BridgeCallerSession) GetL2ToL1(l2ChannelId [32]byte) ([32]byte, error) {
	return _Bridge.Contract.GetL2ToL1(&_Bridge.CallOpts, l2ChannelId)
}

// GetMirroredChannelStatus is a free data retrieval call binding the contract method 0xe34e5180.
//
// Solidity: function getMirroredChannelStatus(bytes32 channelId) view returns(bytes32)
func (_Bridge *BridgeCaller) GetMirroredChannelStatus(opts *bind.CallOpts, channelId [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Bridge.contract.Call(opts, &out, "getMirroredChannelStatus", channelId)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetMirroredChannelStatus is a free data retrieval call binding the contract method 0xe34e5180.
//
// Solidity: function getMirroredChannelStatus(bytes32 channelId) view returns(bytes32)
func (_Bridge *BridgeSession) GetMirroredChannelStatus(channelId [32]byte) ([32]byte, error) {
	return _Bridge.Contract.GetMirroredChannelStatus(&_Bridge.CallOpts, channelId)
}

// GetMirroredChannelStatus is a free data retrieval call binding the contract method 0xe34e5180.
//
// Solidity: function getMirroredChannelStatus(bytes32 channelId) view returns(bytes32)
func (_Bridge *BridgeCallerSession) GetMirroredChannelStatus(channelId [32]byte) ([32]byte, error) {
	return _Bridge.Contract.GetMirroredChannelStatus(&_Bridge.CallOpts, channelId)
}

// L2Tol1 is a free data retrieval call binding the contract method 0x486f14d1.
//
// Solidity: function l2Tol1(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeCaller) L2Tol1(opts *bind.CallOpts, arg0 [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Bridge.contract.Call(opts, &out, "l2Tol1", arg0)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// L2Tol1 is a free data retrieval call binding the contract method 0x486f14d1.
//
// Solidity: function l2Tol1(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeSession) L2Tol1(arg0 [32]byte) ([32]byte, error) {
	return _Bridge.Contract.L2Tol1(&_Bridge.CallOpts, arg0)
}

// L2Tol1 is a free data retrieval call binding the contract method 0x486f14d1.
//
// Solidity: function l2Tol1(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeCallerSession) L2Tol1(arg0 [32]byte) ([32]byte, error) {
	return _Bridge.Contract.L2Tol1(&_Bridge.CallOpts, arg0)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Bridge *BridgeCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _Bridge.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Bridge *BridgeSession) Owner() (common.Address, error) {
	return _Bridge.Contract.Owner(&_Bridge.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_Bridge *BridgeCallerSession) Owner() (common.Address, error) {
	return _Bridge.Contract.Owner(&_Bridge.CallOpts)
}

// StatusOf is a free data retrieval call binding the contract method 0xc7df14e2.
//
// Solidity: function statusOf(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeCaller) StatusOf(opts *bind.CallOpts, arg0 [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _Bridge.contract.Call(opts, &out, "statusOf", arg0)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// StatusOf is a free data retrieval call binding the contract method 0xc7df14e2.
//
// Solidity: function statusOf(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeSession) StatusOf(arg0 [32]byte) ([32]byte, error) {
	return _Bridge.Contract.StatusOf(&_Bridge.CallOpts, arg0)
}

// StatusOf is a free data retrieval call binding the contract method 0xc7df14e2.
//
// Solidity: function statusOf(bytes32 ) view returns(bytes32)
func (_Bridge *BridgeCallerSession) StatusOf(arg0 [32]byte) ([32]byte, error) {
	return _Bridge.Contract.StatusOf(&_Bridge.CallOpts, arg0)
}

// SetL2ToL1 is a paid mutator transaction binding the contract method 0x77027728.
//
// Solidity: function setL2ToL1(bytes32 l1ChannelId, bytes32 l2ChannelId) returns()
func (_Bridge *BridgeTransactor) SetL2ToL1(opts *bind.TransactOpts, l1ChannelId [32]byte, l2ChannelId [32]byte) (*types.Transaction, error) {
	return _Bridge.contract.Transact(opts, "setL2ToL1", l1ChannelId, l2ChannelId)
}

// SetL2ToL1 is a paid mutator transaction binding the contract method 0x77027728.
//
// Solidity: function setL2ToL1(bytes32 l1ChannelId, bytes32 l2ChannelId) returns()
func (_Bridge *BridgeSession) SetL2ToL1(l1ChannelId [32]byte, l2ChannelId [32]byte) (*types.Transaction, error) {
	return _Bridge.Contract.SetL2ToL1(&_Bridge.TransactOpts, l1ChannelId, l2ChannelId)
}

// SetL2ToL1 is a paid mutator transaction binding the contract method 0x77027728.
//
// Solidity: function setL2ToL1(bytes32 l1ChannelId, bytes32 l2ChannelId) returns()
func (_Bridge *BridgeTransactorSession) SetL2ToL1(l1ChannelId [32]byte, l2ChannelId [32]byte) (*types.Transaction, error) {
	return _Bridge.Contract.SetL2ToL1(&_Bridge.TransactOpts, l1ChannelId, l2ChannelId)
}

// UpdateMirroredChannelStatus is a paid mutator transaction binding the contract method 0xd3a50807.
//
// Solidity: function updateMirroredChannelStatus(bytes32 channelId, bytes32 stateHash, bytes32 outcomeHash) returns()
func (_Bridge *BridgeTransactor) UpdateMirroredChannelStatus(opts *bind.TransactOpts, channelId [32]byte, stateHash [32]byte, outcomeHash [32]byte) (*types.Transaction, error) {
	return _Bridge.contract.Transact(opts, "updateMirroredChannelStatus", channelId, stateHash, outcomeHash)
}

// UpdateMirroredChannelStatus is a paid mutator transaction binding the contract method 0xd3a50807.
//
// Solidity: function updateMirroredChannelStatus(bytes32 channelId, bytes32 stateHash, bytes32 outcomeHash) returns()
func (_Bridge *BridgeSession) UpdateMirroredChannelStatus(channelId [32]byte, stateHash [32]byte, outcomeHash [32]byte) (*types.Transaction, error) {
	return _Bridge.Contract.UpdateMirroredChannelStatus(&_Bridge.TransactOpts, channelId, stateHash, outcomeHash)
}

// UpdateMirroredChannelStatus is a paid mutator transaction binding the contract method 0xd3a50807.
//
// Solidity: function updateMirroredChannelStatus(bytes32 channelId, bytes32 stateHash, bytes32 outcomeHash) returns()
func (_Bridge *BridgeTransactorSession) UpdateMirroredChannelStatus(channelId [32]byte, stateHash [32]byte, outcomeHash [32]byte) (*types.Transaction, error) {
	return _Bridge.Contract.UpdateMirroredChannelStatus(&_Bridge.TransactOpts, channelId, stateHash, outcomeHash)
}
