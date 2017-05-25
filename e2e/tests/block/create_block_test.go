package block

import (
	"github.com/rook/rook/e2e/framework/clients"
	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/tests"
	"github.com/rook/rook/pkg/model"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"strings"
	"testing"
)

var (
	default_pool   = "rbd"
	pool1          = "rook_test_pool"
	blockImageName = "testImage"
)

func TestBlockCreate(t *testing.T) {
	suite.Run(t, new(BlockImageCreateSuite))
}

type BlockImageCreateSuite struct {
	suite.Suite
	testClient      *clients.TestClient
	rc              contracts.RestAPIOperator
	init_blockCount int
}

func (s *BlockImageCreateSuite) SetupSuite() {

	var err error

	s.testClient, err = clients.CreateTestClient(tests.Platform)
	require.Nil(s.T(), err)

	s.rc = s.testClient.GetRestAPIClient()
	initialBlocks, err := s.rc.GetBlockImages()
	require.Nil(s.T(), err)
	s.init_blockCount = len(initialBlocks)
}

//Test Creating Block image on default pool(rbd)
func (s *BlockImageCreateSuite) TestCreatingNewBlockImageOnDefaultPool() {

	s.T().Log("Test Creating new block image  for default pool")
	newImage := model.BlockImage{Name: blockImageName, Size: 123, PoolName: default_pool}
	cbi, err := s.rc.CreateBlockImage(newImage)
	require.Nil(s.T(), err)
	require.Contains(s.T(), cbi, "succeeded created image")
	b, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), s.init_blockCount+1, len(b), "Make sure new block image is created")
	s.T().Log("Test Passed")

}

//Test Creating Block image on custom pool
func (s *BlockImageCreateSuite) TestCreatingNewBlockImageOnCustomPool() {

	s.T().Log("Test Creating new block image for custom pool")
	newPool := model.Pool{Name: pool1}
	_, err := s.rc.CreatePool(newPool)
	require.Nil(s.T(), err)

	newImage := model.BlockImage{Name: blockImageName, Size: 123, PoolName: newPool.Name}
	cbi, err := s.rc.CreateBlockImage(newImage)
	require.Nil(s.T(), err)
	require.Contains(s.T(), cbi, "succeeded created image")
	b, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), s.init_blockCount+1, len(b), "Make sure new block image is created")

}

//Test Creating Block image twice on same pool
func (s *BlockImageCreateSuite) TestRecreatingBlockImageForSamePool() {

	s.T().Log("Test Case when Block Image is created with Name that is already used by another block on same pool")
	// create new block image
	newImage := model.BlockImage{Name: blockImageName, Size: 123, PoolName: default_pool}
	cbi, err := s.rc.CreateBlockImage(newImage)
	require.Nil(s.T(), err)
	require.Contains(s.T(), cbi, "succeeded created image")
	b, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), s.init_blockCount+1, len(b), "Make sure new block image is created")

	//create same block again on same pool
	newImage2 := model.BlockImage{Name: blockImageName, Size: 2897, PoolName: default_pool}
	cbi2, err := s.rc.CreateBlockImage(newImage2)
	require.Error(s.T(), err, "Make sure dupe block is not created")
	require.NotContains(s.T(), cbi2, "succeeded created image")
	b2, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), len(b), len(b2), "Make sure new block image is not created")

}

//Test Creating Block image twice on different pool
func (s *BlockImageCreateSuite) TestRecreatingBlockImageForDifferentPool() {

	s.T().Log("Test Case when Block Image is created with Name that is already used by another block on different pool")
	// create new block image
	newImage := model.BlockImage{Name: blockImageName, Size: 123, PoolName: default_pool}
	cbi, err := s.rc.CreateBlockImage(newImage)
	require.Nil(s.T(), err)
	require.Contains(s.T(), cbi, "succeeded created image")
	b, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), s.init_blockCount+1, len(b), "Make sure new block image is created")

	newPool := model.Pool{Name: pool1}
	_, err = s.rc.CreatePool(newPool)
	require.Nil(s.T(), err)

	//create same block again on different pool
	newImage2 := model.BlockImage{Name: blockImageName, Size: 2897, PoolName: newPool.Name}
	cbi2, err := s.rc.CreateBlockImage(newImage2)
	require.Nil(s.T(), err)
	require.Contains(s.T(), cbi2, "succeeded created image")
	b2, _ := s.rc.GetBlockImages()
	require.Equal(s.T(), len(b)+1, len(b2), "Make sure new block image is created")

}

// Delete all Block images that have the word Test in their name
func (s *BlockImageCreateSuite) TearDownTest() {

	blocks, _ := s.rc.GetBlockImages()

	for _, b := range blocks {
		if strings.Contains(b.Name, "test") {
			s.rc.DeleteBlockImage(b)
		}
	}
}
