package testutil

import (
	"iml-daemon/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockRepo struct {
	mock.Mock
}

func (r *MockRepo) FindActiveAppByLocalID(localID uuid.UUID) (*models.Application, error) {
	args := r.Called(localID)
	return args.Get(0).(*models.Application), args.Error(1)
}
func (r *MockRepo) FindActiveAppByGlobalID(globalID string) (*models.Application, error) {
	args := r.Called(globalID)
	return args.Get(0).(*models.Application), args.Error(1)
}
func (r *MockRepo) FindLocalAppGroupByGlobalID(globalAppID string) (*models.AppGroup, error) {
	args := r.Called(globalAppID)
	return args.Get(0).(*models.AppGroup), args.Error(1)
}
func (r *MockRepo) FindAppGroupByID(id uuid.UUID) (*models.AppGroup, error) {
	args := r.Called(id)
	return args.Get(0).(*models.AppGroup), args.Error(1)
}
func (r *MockRepo) FindAppInstanceByContainerID(containerID string) (*models.AppInstance, error) {
	args := r.Called(containerID)
	return args.Get(0).(*models.AppInstance), args.Error(1)
}
func (r *MockRepo) FindActiveNetworkFunctionByLocalID(localID uuid.UUID) (*models.VirtualNetworkFunction, error) {
	args := r.Called(localID)
	return args.Get(0).(*models.VirtualNetworkFunction), args.Error(1)
}
func (r *MockRepo) FindActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	args := r.Called(globalID)
	return args.Get(0).(*models.VirtualNetworkFunction), args.Error(1)
}
func (r *MockRepo) FindCompleteActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error) {
	args := r.Called(globalID)
	return args.Get(0).(*models.VirtualNetworkFunction), args.Error(1)
}
func (r *MockRepo) FindVnfInstanceByContainerID(containerID string) (*models.SimpleVnfInstance, error) {
	args := r.Called(containerID)
	return args.Get(0).(*models.SimpleVnfInstance), args.Error(1)
}
func (r *MockRepo) FindVnfMultiplexedInstanceByContainerID(containerID string) (*models.MultiplexedVnfInstance, error) {
	args := r.Called(containerID)
	return args.Get(0).(*models.MultiplexedVnfInstance), args.Error(1)
}
func (r *MockRepo) FindVnfGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.SimpleVnfGroup, error) {
	args := r.Called(id)
	return args.Get(0).(*models.SimpleVnfGroup), args.Error(1)
}
func (r *MockRepo) FindSubfunctionPreloadedVnfMultiplexedGroupByID(id uuid.UUID) (*models.MultiplexedVnfGroup, error) {
	args := r.Called(id)
	return args.Get(0).(*models.MultiplexedVnfGroup), args.Error(1)
}
func (r *MockRepo) FindVnfMultiplexedGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.MultiplexedVnfGroup, error) {
	args := r.Called(id)
	return args.Get(0).(*models.MultiplexedVnfGroup), args.Error(1)
}
func (r *MockRepo) FindLocalSimpleVnfGroupByVnfID(globalVnfID string) (*models.SimpleVnfGroup, error) {
	args := r.Called(globalVnfID)
	return args.Get(0).(*models.SimpleVnfGroup), args.Error(1)
}
func (r *MockRepo) FindLocalMultiplexedGroupByVnfID(globalVnfID string) (*models.MultiplexedVnfGroup, error) {
	args := r.Called(globalVnfID)
	return args.Get(0).(*models.MultiplexedVnfGroup), args.Error(1)
}
func (r *MockRepo) FindAllNetworkServiceChains() ([]models.ServiceChain, error) {
	args := r.Called()
	return args.Get(0).([]models.ServiceChain), args.Error(1)
}
func (r *MockRepo) FindActiveNetworkServiceChainByGlobalID(globalChainID string) (*models.ServiceChain, error) {
	args := r.Called(globalChainID)
	return args.Get(0).(*models.ServiceChain), args.Error(1)
}
func (r *MockRepo) FindActiveNodeByGlobalID(globalNodeID string) (*models.Worker, error) {
	args := r.Called(globalNodeID)
	return args.Get(0).(*models.Worker), args.Error(1)
}
func (r *MockRepo) FindRemoteAppGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteAppGroup, error) {
	args := r.Called(nodeID, groupID)
	return args.Get(0).(*models.RemoteAppGroup), args.Error(1)
}
func (r *MockRepo) FindRemoteVnfGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteVnfGroup, error) {
	args := r.Called(nodeID, groupID)
	return args.Get(0).(*models.RemoteVnfGroup), args.Error(1)
}
func (r *MockRepo) SaveApp(app *models.Application) error {
	args := r.Called(app)
	return args.Error(0)
}
func (r *MockRepo) SaveAppGroup(group *models.AppGroup) error {
	args := r.Called(group)
	return args.Error(0)
}
func (r *MockRepo) SaveAppInstance(instance *models.AppInstance) error {
	args := r.Called(instance)
	return args.Error(0)
}
func (r *MockRepo) SaveVnf(function *models.VirtualNetworkFunction) error {
	args := r.Called(function)
	return args.Error(0)
}
func (r *MockRepo) SaveSubfunctions(subfunctions []models.Subfunction) error {
	args := r.Called(subfunctions)
	return args.Error(0)
}
func (r *MockRepo) SaveVnfGroup(group *models.SimpleVnfGroup) error {
	args := r.Called(group)
	return args.Error(0)
}
func (r *MockRepo) SaveVnfMultiplexedGroup(group *models.MultiplexedVnfGroup) error {
	args := r.Called(group)
	return args.Error(0)
}
func (r *MockRepo) SaveVnfInstance(instance *models.SimpleVnfInstance) error {
	args := r.Called(instance)
	return args.Error(0)
}
func (r *MockRepo) SaveVnfMultiplexedInstance(instance *models.MultiplexedVnfInstance) error {
	args := r.Called(instance)
	return args.Error(0)
}
func (r *MockRepo) SaveNetworkServiceChain(chain *models.ServiceChain) error {
	args := r.Called(chain)
	return args.Error(0)
}
func (r *MockRepo) RemoveAppInstance(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) RemoveAppGroup(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) RemoveVnfInstance(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) RemoveVnfMultiplexedInstance(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) RemoveVnfGroup(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) RemoveVnfMultiplexedGroup(id uuid.UUID) error {
	args := r.Called(id)
	return args.Error(0)
}
func (r *MockRepo) SaveRemoteAppGroup(group *models.RemoteAppGroup) error {
	args := r.Called(group)
	return args.Error(0)
}
func (r *MockRepo) SaveRemoteVnfGroup(group *models.RemoteVnfGroup) error {
	args := r.Called(group)
	return args.Error(0)
}
func (r *MockRepo) SaveNode(node *models.Worker) error {
	args := r.Called(node)
	return args.Error(0)
}
func (r *MockRepo) MarkAppAsDeleted(globalID string) error {
	args := r.Called(globalID)
	return args.Error(0)
}
func (r *MockRepo) MarkVnfAsDeleted(globalID string) error {
	args := r.Called(globalID)
	return args.Error(0)
}
func (r *MockRepo) MarkServiceChainAsDeleted(globalID string) error {
	args := r.Called(globalID)
	return args.Error(0)
}
func (r *MockRepo) MarkNodeAsDeleted(globalID string) error {
	args := r.Called(globalID)
	return args.Error(0)
}
func (r *MockRepo) RemoveRemoteAppInstancesByIP(ips []string, groupID uuid.UUID) error {
	args := r.Called(ips, groupID)
	return args.Error(0)
}
func (r *MockRepo) RemoveRemoteAppGroupsByGlobalAppID(globalAppID string) error {
	args := r.Called(globalAppID)
	return args.Error(0)
}
func (r *MockRepo) RemoveRemoteVnfGroupsByGlobalVnfID(globalVnfID string) error {
	args := r.Called(globalVnfID)
	return args.Error(0)
}
