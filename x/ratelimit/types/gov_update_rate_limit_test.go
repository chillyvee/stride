package types_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/Stride-Labs/stride/v10/app/apptesting"
	"github.com/Stride-Labs/stride/v10/x/ratelimit/types"
)

func TestGovUpdateRateLimit(t *testing.T) {
	apptesting.SetupConfig()

	validTitle := "UpdateRateLimit"
	validDescription := "Updating a rate limit"
	validDenom := "denom"
	validChannelId := "channel-0"
	validMaxPercentSend := sdkmath.NewInt(10)
	validMaxPercentRecv := sdkmath.NewInt(10)
	validDurationHours := uint64(60)

	tests := []struct {
		name     string
		proposal types.UpdateRateLimitProposal
		err      string
	}{
		{
			name: "successful proposal",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
		},
		{
			name: "invalid title",
			proposal: types.UpdateRateLimitProposal{
				Title:          "",
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "title cannot be blank",
		},
		{
			name: "invalid description",
			proposal: types.UpdateRateLimitProposal{
				Title:          validDescription,
				Description:    "",
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "description cannot be blank",
		},
		{
			name: "invalid denom",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          "",
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "invalid denom",
		},
		{
			name: "invalid channel-id",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      "channel-",
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "invalid channel-id",
		},
		{
			name: "invalid send percent (lt 0)",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: sdkmath.NewInt(-1),
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "percent must be between 0 and 100",
		},
		{
			name: "invalid send percent (gt 100)",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: sdkmath.NewInt(101),
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  validDurationHours,
			},
			err: "percent must be between 0 and 100",
		},
		{
			name: "invalid receive percent (lt 0)",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: sdkmath.NewInt(-1),
				DurationHours:  validDurationHours,
			},
			err: "percent must be between 0 and 100",
		},
		{
			name: "invalid receive percent (gt 100)",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: sdkmath.NewInt(101),
				DurationHours:  validDurationHours,
			},
			err: "percent must be between 0 and 100",
		},
		{
			name: "invalid send and receive percent",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: sdkmath.ZeroInt(),
				MaxPercentRecv: sdkmath.ZeroInt(),
				DurationHours:  validDurationHours,
			},
			err: "either the max send or max receive threshold must be greater than 0",
		},
		{
			name: "invalid duration",
			proposal: types.UpdateRateLimitProposal{
				Title:          validTitle,
				Description:    validDescription,
				Denom:          validDenom,
				ChannelId:      validChannelId,
				MaxPercentSend: validMaxPercentSend,
				MaxPercentRecv: validMaxPercentRecv,
				DurationHours:  0,
			},
			err: "duration can not be zero",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.err == "" {
				require.NoError(t, test.proposal.ValidateBasic(), "test: %v", test.name)
				require.Equal(t, test.proposal.Denom, validDenom, "denom")
				require.Equal(t, test.proposal.ChannelId, validChannelId, "channelId")
				require.Equal(t, test.proposal.MaxPercentSend, validMaxPercentSend, "maxPercentSend")
				require.Equal(t, test.proposal.MaxPercentRecv, validMaxPercentRecv, "maxPercentRecv")
				require.Equal(t, test.proposal.DurationHours, validDurationHours, "durationHours")
			} else {
				require.ErrorContains(t, test.proposal.ValidateBasic(), test.err, "test: %v", test.name)
			}
		})
	}
}
