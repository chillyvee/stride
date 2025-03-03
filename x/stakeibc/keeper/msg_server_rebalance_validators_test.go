package keeper_test

import (
	sdkmath "cosmossdk.io/math"
	_ "github.com/stretchr/testify/suite"

	sdk "github.com/cosmos/cosmos-sdk/types"
	icatypes "github.com/cosmos/ibc-go/v7/modules/apps/27-interchain-accounts/types"
	ibctesting "github.com/cosmos/ibc-go/v7/testing"

	epochtypes "github.com/Stride-Labs/stride/v10/x/epochs/types"
	icacallbackstypes "github.com/Stride-Labs/stride/v10/x/icacallbacks/types"
	stakeibctypes "github.com/Stride-Labs/stride/v10/x/stakeibc/types"
)

type RebalanceValidatorsTestCase struct {
	hostZone          stakeibctypes.HostZone
	initialValidators []*stakeibctypes.Validator
	validMsgs         []stakeibctypes.MsgRebalanceValidators
	delegationChannel string
}

func (s *KeeperTestSuite) SetupRebalanceValidators() RebalanceValidatorsTestCase {
	// Setup IBC
	delegationIcaOwner := "GAIA.DELEGATION"
	delegationChannelId := s.CreateICAChannel(delegationIcaOwner)
	delegationAddr := s.IcaAddresses[delegationIcaOwner]

	// setup epochs
	epochNumber := uint64(1)
	epochTracker := stakeibctypes.EpochTracker{
		EpochIdentifier:    epochtypes.STRIDE_EPOCH,
		EpochNumber:        epochNumber,
		NextEpochStartTime: uint64(s.Coordinator.CurrentTime.UnixNano() + 30_000_000_000), // dictates timeouts
	}
	s.App.StakeibcKeeper.SetEpochTracker(s.Ctx, epochTracker)

	// define validators for host zone
	initialValidators := []*stakeibctypes.Validator{
		{
			Name:          "val1",
			Address:       "stride_VAL1",
			Weight:        100,
			DelegationAmt: sdkmath.NewInt(100),
		},
		{
			Name:          "val2",
			Address:       "stride_VAL2",
			Weight:        500,
			DelegationAmt: sdkmath.NewInt(500),
		},
		{
			Name:          "val3",
			Address:       "stride_VAL3",
			Weight:        200,
			DelegationAmt: sdkmath.NewInt(200),
		},
		{
			Name:          "val4",
			Address:       "stride_VAL4",
			Weight:        400,
			DelegationAmt: sdkmath.NewInt(400),
		},
		{
			Name:          "val5",
			Address:       "stride_VAL5",
			Weight:        400,
			DelegationAmt: sdkmath.NewInt(400),
		},
	}

	// setup host zone
	hostZone := stakeibctypes.HostZone{
		ChainId:      "GAIA",
		Validators:   initialValidators,
		StakedBal:    sdkmath.NewInt(1000),
		ConnectionId: ibctesting.FirstConnectionID,
		DelegationAccount: &stakeibctypes.ICAAccount{
			Address: delegationAddr,
			Target:  stakeibctypes.ICAAccountType_DELEGATION,
		},
		HostDenom: "uatom",
	}
	s.App.StakeibcKeeper.SetHostZone(s.Ctx, hostZone)

	// base valid messages
	validMsgs := []stakeibctypes.MsgRebalanceValidators{
		{
			Creator:      "stride_ADDRESS",
			HostZone:     "GAIA",
			NumRebalance: 1,
		},
		{
			Creator:      "stride_ADDRESS",
			HostZone:     "GAIA",
			NumRebalance: 2,
		},
	}

	return RebalanceValidatorsTestCase{
		hostZone:          hostZone,
		initialValidators: initialValidators,
		validMsgs:         validMsgs,
		delegationChannel: delegationChannelId,
	}
}

func (s *KeeperTestSuite) TestRebalanceValidators_Successful() {
	tc := s.SetupRebalanceValidators()

	hz, found := s.App.StakeibcKeeper.GetHostZone(s.Ctx, "GAIA")
	s.Require().True(found, "host zone should exist")
	validators := hz.GetValidators()
	s.Require().Equal(5, len(validators), "host zone should have 5 validators")
	// modify weight to 25
	validators[0].Weight = 250
	validators[2].Weight = 100
	s.App.StakeibcKeeper.SetHostZone(s.Ctx, hz)

	// get sequence ID for callbacks
	portId := icatypes.ControllerPortPrefix + "GAIA.DELEGATION"
	startSequence, found := s.App.IBCKeeper.ChannelKeeper.GetNextSequenceSend(s.Ctx, portId, tc.delegationChannel)
	s.Require().True(found, "sequence number not found before rebalance")

	// Rebalance one validator
	badMsg_rightWeights := stakeibctypes.MsgRebalanceValidators{
		Creator:      "stride_ADDRESS",
		HostZone:     "GAIA",
		NumRebalance: 2,
	}
	_, err := s.GetMsgServer().RebalanceValidators(sdk.WrapSDKContext(s.Ctx), &badMsg_rightWeights)
	s.Require().NoError(err, "rebalancing with 2 validators should succeed")

	// get stored callback data
	callbackKey := icacallbackstypes.PacketID(portId, tc.delegationChannel, startSequence)
	callbackData, found := s.App.StakeibcKeeper.ICACallbacksKeeper.GetCallbackData(s.Ctx, callbackKey)
	s.Require().True(found, "callback should exist")
	s.Require().Equal("rebalance", callbackData.CallbackId, "callback key should be rebalance")
	callbackArgs, err := s.App.StakeibcKeeper.UnmarshalRebalanceCallbackArgs(s.Ctx, callbackData.CallbackArgs)
	s.Require().NoError(err, "unmarshalling callback args error for callback key (%s)", callbackKey)
	s.Require().Equal("GAIA", callbackArgs.HostZoneId, "callback host zone id should be GAIA")

	// verify callback rebalance is what we want
	s.Require().Equal(2, len(callbackArgs.Rebalancings), "callback should have 2 rebalancing")
	firstRebal := callbackArgs.Rebalancings[0]
	s.Require().Equal(sdkmath.NewInt(104), firstRebal.Amt, "first rebalance should rebalance 104 ATOM")
	s.Require().Equal("stride_VAL1", firstRebal.DstValidator, "first rebalance moves to val1")
	s.Require().Equal("stride_VAL3", firstRebal.SrcValidator, "first rebalance takes from val3")
	secondRebal := callbackArgs.Rebalancings[1]
	s.Require().Equal(sdkmath.NewInt(13), secondRebal.Amt, "second rebalance should rebalance 13 ATOM")
	s.Require().Equal("stride_VAL1", secondRebal.DstValidator, "second rebalance moves to val1")
	s.Require().Equal("stride_VAL4", secondRebal.SrcValidator, "second rebalance takes from val4")
}

func (s *KeeperTestSuite) TestRebalanceValidators_InvalidNoValidators() {
	s.SetupRebalanceValidators()

	hz, found := s.App.StakeibcKeeper.GetHostZone(s.Ctx, "GAIA")
	s.Require().True(found, "host zone should exist")
	hz.Validators = []*stakeibctypes.Validator{}
	s.App.StakeibcKeeper.SetHostZone(s.Ctx, hz)

	// Rebalance with all weights properly set should fail
	badMsg_noValidators := stakeibctypes.MsgRebalanceValidators{
		Creator:      "stride_ADDRESS",
		HostZone:     "GAIA",
		NumRebalance: 2,
	}
	_, err := s.GetMsgServer().RebalanceValidators(sdk.WrapSDKContext(s.Ctx), &badMsg_noValidators)
	expectedErrMsg := "no non-zero validator weights"
	s.Require().EqualError(err, expectedErrMsg, "rebalancing with no validators should fail")
}

func (s *KeeperTestSuite) TestRebalanceValidators_InvalidAllValidatorsNoWeight() {
	s.SetupRebalanceValidators()

	hz, found := s.App.StakeibcKeeper.GetHostZone(s.Ctx, "GAIA")
	s.Require().True(found, "host zone should exist")
	validators := hz.GetValidators()
	for _, v := range validators {
		v.Weight = 0
	}
	s.App.StakeibcKeeper.SetHostZone(s.Ctx, hz)

	// Rebalance with all weights properly set should fail
	badMsg_noValidators := stakeibctypes.MsgRebalanceValidators{
		Creator:      "stride_ADDRESS",
		HostZone:     "GAIA",
		NumRebalance: 2,
	}
	_, err := s.GetMsgServer().RebalanceValidators(sdk.WrapSDKContext(s.Ctx), &badMsg_noValidators)
	expectedErrMsg := "no non-zero validator weights"
	s.Require().EqualError(err, expectedErrMsg, "rebalancing with no validators should fail")
}
