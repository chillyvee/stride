package keeper

import (
	"fmt"
	"strings"

	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	proto "github.com/cosmos/gogoproto/proto"
	"github.com/spf13/cast"

	"github.com/Stride-Labs/stride/v10/utils"
	icacallbackstypes "github.com/Stride-Labs/stride/v10/x/icacallbacks/types"

	recordstypes "github.com/Stride-Labs/stride/v10/x/records/types"
	"github.com/Stride-Labs/stride/v10/x/stakeibc/types"

	bankTypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingTypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	epochstypes "github.com/Stride-Labs/stride/v10/x/epochs/types"
	icqtypes "github.com/Stride-Labs/stride/v10/x/interchainquery/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	icatypes "github.com/cosmos/ibc-go/v7/modules/apps/27-interchain-accounts/types"
)

func (k Keeper) DelegateOnHost(ctx sdk.Context, hostZone types.HostZone, amt sdk.Coin, depositRecord recordstypes.DepositRecord) error {
	// the relevant ICA is the delegate account
	owner := types.FormatICAAccountOwner(hostZone.ChainId, types.ICAAccountType_DELEGATION)
	portID, err := icatypes.NewControllerPortID(owner)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "%s has no associated portId", owner)
	}
	connectionId, err := k.GetConnectionId(ctx, portID)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidChainID, "%s has no associated connection", portID)
	}

	// Fetch the relevant ICA
	delegationAccount := hostZone.DelegationAccount
	if delegationAccount == nil || delegationAccount.Address == "" {
		k.Logger(ctx).Error(fmt.Sprintf("Zone %s is missing a delegation address!", hostZone.ChainId))
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "Invalid delegation account")
	}

	// Construct the transaction
	targetDelegatedAmts, err := k.GetTargetValAmtsForHostZone(ctx, hostZone, amt.Amount)
	if err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("Error getting target delegation amounts for host zone %s", hostZone.ChainId))
		return err
	}

	var splitDelegations []*types.SplitDelegation
	var msgs []proto.Message
	for _, validator := range hostZone.Validators {
		relativeAmount := sdk.NewCoin(amt.Denom, targetDelegatedAmts[validator.Address])
		if relativeAmount.Amount.IsPositive() {
			msgs = append(msgs, &stakingTypes.MsgDelegate{
				DelegatorAddress: delegationAccount.Address,
				ValidatorAddress: validator.Address,
				Amount:           relativeAmount,
			})
		}
		splitDelegations = append(splitDelegations, &types.SplitDelegation{
			Validator: validator.Address,
			Amount:    relativeAmount.Amount,
		})
	}
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "Preparing MsgDelegates from the delegation account to each validator"))

	// add callback data
	delegateCallback := types.DelegateCallback{
		HostZoneId:       hostZone.ChainId,
		DepositRecordId:  depositRecord.Id,
		SplitDelegations: splitDelegations,
	}
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "Marshalling DelegateCallback args: %+v", delegateCallback))
	marshalledCallbackArgs, err := k.MarshalDelegateCallbackArgs(ctx, delegateCallback)
	if err != nil {
		return err
	}

	// Send the transaction through SubmitTx
	_, err = k.SubmitTxsStrideEpoch(ctx, connectionId, msgs, *delegationAccount, ICACallbackID_Delegate, marshalledCallbackArgs)
	if err != nil {
		return errorsmod.Wrapf(err, "Failed to SubmitTxs for connectionId %s on %s. Messages: %s", connectionId, hostZone.ChainId, msgs)
	}
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "ICA MsgDelegates Successfully Sent"))

	// update the record state to DELEGATION_IN_PROGRESS
	depositRecord.Status = recordstypes.DepositRecord_DELEGATION_IN_PROGRESS
	k.RecordsKeeper.SetDepositRecord(ctx, depositRecord)

	return nil
}

func (k Keeper) SetWithdrawalAddressOnHost(ctx sdk.Context, hostZone types.HostZone) error {
	// The relevant ICA is the delegate account
	owner := types.FormatICAAccountOwner(hostZone.ChainId, types.ICAAccountType_DELEGATION)
	portID, err := icatypes.NewControllerPortID(owner)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "%s has no associated portId", owner)
	}
	connectionId, err := k.GetConnectionId(ctx, portID)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidChainID, "%s has no associated connection", portID)
	}

	// Fetch the relevant ICA
	delegationAccount := hostZone.DelegationAccount
	if delegationAccount == nil || delegationAccount.Address == "" {
		k.Logger(ctx).Error(fmt.Sprintf("Zone %s is missing a delegation address!", hostZone.ChainId))
		return nil
	}
	withdrawalAccount := hostZone.WithdrawalAccount
	if withdrawalAccount == nil || withdrawalAccount.Address == "" {
		k.Logger(ctx).Error(fmt.Sprintf("Zone %s is missing a withdrawal address!", hostZone.ChainId))
		return nil
	}
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "Withdrawal Address: %s, Delegator Address: %s",
		withdrawalAccount.Address, delegationAccount.Address))

	// Construct the ICA message
	msgs := []proto.Message{
		&distributiontypes.MsgSetWithdrawAddress{
			DelegatorAddress: delegationAccount.Address,
			WithdrawAddress:  withdrawalAccount.Address,
		},
	}
	_, err = k.SubmitTxsStrideEpoch(ctx, connectionId, msgs, *delegationAccount, "", nil)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "Failed to SubmitTxs for %s, %s, %s", connectionId, hostZone.ChainId, msgs)
	}

	return nil
}

// Submits an ICQ for the withdrawal account balance
func (k Keeper) UpdateWithdrawalBalance(ctx sdk.Context, hostZone types.HostZone) error {
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "Submitting ICQ for withdrawal account balance"))

	// Get the withdrawal account address from the host zone
	withdrawalAccount := hostZone.WithdrawalAccount
	if withdrawalAccount == nil || withdrawalAccount.Address == "" {
		return errorsmod.Wrapf(types.ErrICAAccountNotFound, "no withdrawal account found for %s", hostZone.ChainId)
	}

	// Encode the withdrawal account address for the query request
	// The query request consists of the withdrawal account address and denom
	_, withdrawalAddressBz, err := bech32.DecodeAndConvert(withdrawalAccount.Address)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid withdrawal account address, could not decode (%s)", err.Error())
	}
	queryData := append(bankTypes.CreateAccountBalancesPrefix(withdrawalAddressBz), []byte(hostZone.HostDenom)...)

	// The query should timeout at the end of the ICA buffer window
	ttl, err := k.GetICATimeoutNanos(ctx, epochstypes.STRIDE_EPOCH)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"Failed to get ICA timeout nanos for epochType %s using param, error: %s", epochstypes.STRIDE_EPOCH, err.Error())
	}

	// Submit the ICQ for the withdrawal account balance
	if err := k.InterchainQueryKeeper.MakeRequest(
		ctx,
		types.ModuleName,
		ICQCallbackID_WithdrawalBalance,
		hostZone.ChainId,
		hostZone.ConnectionId,
		icqtypes.BANK_STORE_QUERY_WITH_PROOF,
		queryData,
		ttl,
	); err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("Error querying for withdrawal balance, error: %s", err.Error()))
		return err
	}

	return nil
}

// helper to get time at which next epoch begins, in unix nano units
func (k Keeper) GetStartTimeNextEpoch(ctx sdk.Context, epochType string) (uint64, error) {
	epochTracker, found := k.GetEpochTracker(ctx, epochType)
	if !found {
		k.Logger(ctx).Error(fmt.Sprintf("Failed to get epoch tracker for %s", epochType))
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "Failed to get epoch tracker for %s", epochType)
	}
	return epochTracker.NextEpochStartTime, nil
}

func (k Keeper) SubmitTxsDayEpoch(
	ctx sdk.Context,
	connectionId string,
	msgs []proto.Message,
	account types.ICAAccount,
	callbackId string,
	callbackArgs []byte,
) (uint64, error) {
	sequence, err := k.SubmitTxsEpoch(ctx, connectionId, msgs, account, epochstypes.DAY_EPOCH, callbackId, callbackArgs)
	if err != nil {
		return 0, err
	}
	return sequence, nil
}

func (k Keeper) SubmitTxsStrideEpoch(
	ctx sdk.Context,
	connectionId string,
	msgs []proto.Message,
	account types.ICAAccount,
	callbackId string,
	callbackArgs []byte,
) (uint64, error) {
	sequence, err := k.SubmitTxsEpoch(ctx, connectionId, msgs, account, epochstypes.STRIDE_EPOCH, callbackId, callbackArgs)
	if err != nil {
		return 0, err
	}
	return sequence, nil
}

func (k Keeper) SubmitTxsEpoch(
	ctx sdk.Context,
	connectionId string,
	msgs []proto.Message,
	account types.ICAAccount,
	epochType string,
	callbackId string,
	callbackArgs []byte,
) (uint64, error) {
	timeoutNanosUint64, err := k.GetICATimeoutNanos(ctx, epochType)
	if err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("Failed to get ICA timeout nanos for epochType %s using param, error: %s", epochType, err.Error()))
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "Failed to convert timeoutNanos to uint64, error: %s", err.Error())
	}
	sequence, err := k.SubmitTxs(ctx, connectionId, msgs, account, timeoutNanosUint64, callbackId, callbackArgs)
	if err != nil {
		return 0, err
	}
	return sequence, nil
}

// SubmitTxs submits an ICA transaction containing multiple messages
func (k Keeper) SubmitTxs(
	ctx sdk.Context,
	connectionId string,
	msgs []proto.Message,
	account types.ICAAccount,
	timeoutTimestamp uint64,
	callbackId string,
	callbackArgs []byte,
) (uint64, error) {
	chainId, err := k.GetChainID(ctx, connectionId)
	if err != nil {
		return 0, err
	}
	owner := types.FormatICAAccountOwner(chainId, account.Target)
	portID, err := icatypes.NewControllerPortID(owner)
	if err != nil {
		return 0, err
	}

	k.Logger(ctx).Info(utils.LogWithHostZone(chainId, "  Submitting ICA Tx on %s, %s with TTL: %d", portID, connectionId, timeoutTimestamp))
	protoMsgs := []proto.Message{}
	for _, msg := range msgs {
		k.Logger(ctx).Info(utils.LogWithHostZone(chainId, "    Msg: %+v", msg))
		protoMsgs = append(protoMsgs, msg)
	}

	channelID, found := k.ICAControllerKeeper.GetActiveChannelID(ctx, connectionId, portID)
	if !found {
		return 0, errorsmod.Wrapf(icatypes.ErrActiveChannelNotFound, "failed to retrieve active channel for port %s", portID)
	}

	data, err := icatypes.SerializeCosmosTx(k.cdc, protoMsgs)
	if err != nil {
		return 0, err
	}

	packetData := icatypes.InterchainAccountPacketData{
		Type: icatypes.EXECUTE_TX,
		Data: data,
	}

	// TODO: SendTx is deprecated and MsgServer should used going forward
	// IMPORTANT: When updating to MsgServer, the timestamp passed to NewMsgSendTx is the relative timeout offset,
	// not the unix time (e.g. "30 minutes in nanoseconds" instead "current unix nano + 30 minutes in nanoseconds")
	sequence, err := k.ICAControllerKeeper.SendTx(ctx, nil, connectionId, portID, packetData, timeoutTimestamp) // nolint:staticcheck
	if err != nil {
		return 0, err
	}

	// Store the callback data
	if callbackId != "" && callbackArgs != nil {
		callback := icacallbackstypes.CallbackData{
			CallbackKey:  icacallbackstypes.PacketID(portID, channelID, sequence),
			PortId:       portID,
			ChannelId:    channelID,
			Sequence:     sequence,
			CallbackId:   callbackId,
			CallbackArgs: callbackArgs,
		}
		k.Logger(ctx).Info(utils.LogWithHostZone(chainId, "Storing callback data: %+v", callback))
		k.ICACallbacksKeeper.SetCallbackData(ctx, callback)
	}

	return sequence, nil
}

func (k Keeper) GetLightClientHeightSafely(ctx sdk.Context, connectionID string) (uint64, error) {
	// get light client's latest height
	conn, found := k.IBCKeeper.ConnectionKeeper.GetConnection(ctx, connectionID)
	if !found {
		errMsg := fmt.Sprintf("invalid connection id, %s not found", connectionID)
		k.Logger(ctx).Error(errMsg)
		return 0, fmt.Errorf(errMsg)
	}
	clientState, found := k.IBCKeeper.ClientKeeper.GetClientState(ctx, conn.ClientId)
	if !found {
		errMsg := fmt.Sprintf("client id %s not found for connection %s", conn.ClientId, connectionID)
		k.Logger(ctx).Error(errMsg)
		return 0, fmt.Errorf(errMsg)
	} else {
		latestHeightHostZone, err := cast.ToUint64E(clientState.GetLatestHeight().GetRevisionHeight())
		if err != nil {
			errMsg := fmt.Sprintf("error casting latest height to int64: %s", err.Error())
			k.Logger(ctx).Error(errMsg)
			return 0, fmt.Errorf(errMsg)
		}
		return latestHeightHostZone, nil
	}
}

func (k Keeper) GetLightClientTimeSafely(ctx sdk.Context, connectionID string) (uint64, error) {
	// get light client's latest height
	conn, found := k.IBCKeeper.ConnectionKeeper.GetConnection(ctx, connectionID)
	if !found {
		errMsg := fmt.Sprintf("invalid connection id, %s not found", connectionID)
		k.Logger(ctx).Error(errMsg)
		return 0, fmt.Errorf(errMsg)
	}
	// TODO(TEST-112) make sure to update host LCs here!
	latestConsensusClientState, found := k.IBCKeeper.ClientKeeper.GetLatestClientConsensusState(ctx, conn.ClientId)
	if !found {
		errMsg := fmt.Sprintf("client id %s not found for connection %s", conn.ClientId, connectionID)
		k.Logger(ctx).Error(errMsg)
		return 0, fmt.Errorf(errMsg)
	} else {
		latestTime := latestConsensusClientState.GetTimestamp()
		return latestTime, nil
	}
}

// Submits an ICQ to get a validator's exchange rate
func (k Keeper) QueryValidatorExchangeRate(ctx sdk.Context, msg *types.MsgUpdateValidatorSharesExchRate) (*types.MsgUpdateValidatorSharesExchRateResponse, error) {
	k.Logger(ctx).Info(utils.LogWithHostZone(msg.ChainId, "Submitting ICQ for validator exchange rate to %s", msg.Valoper))

	// Ensure ICQ can be issued now! else fail the callback
	withinBufferWindow, err := k.IsWithinBufferWindow(ctx)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unable to determine if ICQ callback is inside buffer window, err: %s", err.Error())
	} else if !withinBufferWindow {
		return nil, errorsmod.Wrapf(types.ErrOutsideIcqWindow, "outside the buffer time during which ICQs are allowed (%s)", msg.ChainId)
	}

	// Confirm the host zone exists
	hostZone, found := k.GetHostZone(ctx, msg.ChainId)
	if !found {
		return nil, errorsmod.Wrapf(types.ErrInvalidHostZone, "Host zone not found (%s)", msg.ChainId)
	}

	// check that the validator address matches the bech32 prefix of the hz
	if !strings.Contains(msg.Valoper, hostZone.Bech32Prefix) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "validator operator address must match the host zone bech32 prefix")
	}

	// Encode the validator address to form the query request
	_, validatorAddressBz, err := bech32.DecodeAndConvert(msg.Valoper)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid validator operator address, could not decode (%s)", err.Error())
	}
	queryData := stakingtypes.GetValidatorKey(validatorAddressBz)

	// The query should timeout at the start of the next epoch
	ttl, err := k.GetStartTimeNextEpoch(ctx, epochstypes.STRIDE_EPOCH)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "could not get start time for next epoch: %s", err.Error())
	}

	// Submit validator exchange rate ICQ
	if err := k.InterchainQueryKeeper.MakeRequest(
		ctx,
		types.ModuleName,
		ICQCallbackID_Validator,
		hostZone.ChainId,
		hostZone.ConnectionId,
		icqtypes.STAKING_STORE_QUERY_WITH_PROOF,
		queryData,
		ttl,
	); err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("Error submitting ICQ for validator exchange rate, error %s", err.Error()))
		return nil, err
	}
	return &types.MsgUpdateValidatorSharesExchRateResponse{}, nil
}

// Submits an ICQ to get a validator's delegations
// This is called after the validator's exchange rate is determined
func (k Keeper) QueryDelegationsIcq(ctx sdk.Context, hostZone types.HostZone, valoper string) error {
	k.Logger(ctx).Info(utils.LogWithHostZone(hostZone.ChainId, "Submitting ICQ for delegations to %s", valoper))

	// Ensure ICQ can be issued now! else fail the callback
	valid, err := k.IsWithinBufferWindow(ctx)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unable to determine if ICQ callback is inside buffer window, err: %s", err.Error())
	} else if !valid {
		return errorsmod.Wrapf(types.ErrOutsideIcqWindow, "outside the buffer time during which ICQs are allowed (%s)", hostZone.HostDenom)
	}

	// Get the validator and delegator encoded addresses to form the query request
	delegationAccount := hostZone.DelegationAccount
	if delegationAccount == nil || delegationAccount.Address == "" {
		return errorsmod.Wrapf(types.ErrICAAccountNotFound, "no delegation address found for %s", hostZone.ChainId)
	}
	_, validatorAddressBz, err := bech32.DecodeAndConvert(valoper)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid validator address, could not decode (%s)", err.Error())
	}
	_, delegatorAddressBz, err := bech32.DecodeAndConvert(delegationAccount.Address)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid delegator address, could not decode (%s)", err.Error())
	}
	queryData := stakingtypes.GetDelegationKey(delegatorAddressBz, validatorAddressBz)

	// The query should timeout at the start of the next epoch
	ttl, err := k.GetStartTimeNextEpoch(ctx, epochstypes.STRIDE_EPOCH)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "could not get start time for next epoch: %s", err.Error())
	}

	// Submit delegator shares ICQ
	if err := k.InterchainQueryKeeper.MakeRequest(
		ctx,
		types.ModuleName,
		ICQCallbackID_Delegation,
		hostZone.ChainId,
		hostZone.ConnectionId,
		icqtypes.STAKING_STORE_QUERY_WITH_PROOF,
		queryData,
		ttl,
	); err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("Error submitting ICQ for delegation, error : %s", err.Error()))
		return err
	}

	return nil
}
