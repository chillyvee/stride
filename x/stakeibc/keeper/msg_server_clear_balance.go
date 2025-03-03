package keeper

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	proto "github.com/cosmos/gogoproto/proto"
	ibctransfertypes "github.com/cosmos/ibc-go/v7/modules/apps/transfer/types"
	"github.com/spf13/cast"

	"github.com/Stride-Labs/stride/v10/x/stakeibc/types"
)

func (k msgServer) ClearBalance(goCtx context.Context, msg *types.MsgClearBalance) (*types.MsgClearBalanceResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	zone, found := k.GetHostZone(ctx, msg.ChainId)
	if !found {
		return nil, errorsmod.Wrapf(types.ErrInvalidHostZone, "chainId: %s", msg.ChainId)
	}
	feeAccount := zone.GetFeeAccount()
	if feeAccount == nil {
		return nil, errorsmod.Wrapf(types.ErrFeeAccountNotRegistered, "chainId: %s", msg.ChainId)
	}

	sourcePort := ibctransfertypes.PortID
	// Should this be a param?
	// I think as long as we have a timeout on this, it should be hard to attack (even if someone send a tx on a bad channel, it would be reverted relatively quickly)
	sourceChannel := msg.Channel
	coinString := cast.ToString(msg.Amount) + zone.GetHostDenom()
	tokens, err := sdk.ParseCoinNormalized(coinString)
	if err != nil {
		k.Logger(ctx).Error(fmt.Sprintf("failed to parse coin (%s)", coinString))
		return nil, errorsmod.Wrapf(err, "failed to parse coin (%s)", coinString)
	}
	sender := feeAccount.GetAddress()
	// KeyICATimeoutNanos are for our Stride ICA calls, KeyFeeTransferTimeoutNanos is for the IBC transfer
	feeTransferTimeoutNanos := k.GetParam(ctx, types.KeyFeeTransferTimeoutNanos)
	timeoutTimestamp := cast.ToUint64(ctx.BlockTime().UnixNano()) + feeTransferTimeoutNanos
	msgs := []proto.Message{
		&ibctransfertypes.MsgTransfer{
			SourcePort:       sourcePort,
			SourceChannel:    sourceChannel,
			Token:            tokens,
			Sender:           sender,
			Receiver:         types.FeeAccount,
			TimeoutTimestamp: timeoutTimestamp,
		},
	}

	connectionId := zone.GetConnectionId()

	icaTimeoutNanos := k.GetParam(ctx, types.KeyICATimeoutNanos)
	icaTimeoutNanos = cast.ToUint64(ctx.BlockTime().UnixNano()) + icaTimeoutNanos

	_, err = k.SubmitTxs(ctx, connectionId, msgs, *feeAccount, icaTimeoutNanos, "", nil)
	if err != nil {
		return nil, errorsmod.Wrapf(err, "failed to submit txs")
	}
	return &types.MsgClearBalanceResponse{}, nil
}
