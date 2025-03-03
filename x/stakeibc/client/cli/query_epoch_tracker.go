package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/spf13/cobra"

	"github.com/Stride-Labs/stride/v10/x/stakeibc/types"
)

func CmdListEpochTracker() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-epoch-tracker",
		Short: "list all epoch-tracker",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)

			queryClient := types.NewQueryClient(clientCtx)

			params := &types.QueryAllEpochTrackerRequest{}

			res, err := queryClient.EpochTrackerAll(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}

func CmdShowEpochTracker() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show-epoch-tracker [epoch-identifier]",
		Short: "shows a epoch-tracker",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			clientCtx := client.GetClientContextFromCmd(cmd)

			queryClient := types.NewQueryClient(clientCtx)

			argEpochIdentifier := args[0]

			params := &types.QueryGetEpochTrackerRequest{
				EpochIdentifier: argEpochIdentifier,
			}

			res, err := queryClient.EpochTracker(context.Background(), params)
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}

	flags.AddQueryFlagsToCmd(cmd)

	return cmd
}
