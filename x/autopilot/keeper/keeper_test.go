package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/Stride-Labs/stride/v10/app/apptesting"
	"github.com/Stride-Labs/stride/v10/x/autopilot/types"
)

type KeeperTestSuite struct {
	apptesting.AppTestHelper
	QueryClient types.QueryClient
}

func (s *KeeperTestSuite) SetupTest() {
	s.Setup()
	s.QueryClient = types.NewQueryClient(s.QueryHelper)
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}
