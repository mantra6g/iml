package testutil

import (
	"iml-daemon/services/router/dataplane"
	"net"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockDataplaneManager struct {
	mock.Mock
}

func (mgr *MockDataplaneManager) Close() error {
	args := mgr.Called()
	return args.Error(0)
}
func (mgr *MockDataplaneManager) AddVNFSubnet(vnfGroupID uuid.UUID, sidAmount int) (dataplane.VNFSubnet, error) {
	args := mgr.Called(vnfGroupID, sidAmount)
	return args.Get(0).(dataplane.VNFSubnet), args.Error(1)
}
func (mgr *MockDataplaneManager) AddApplicationSubnet(appGroupID uuid.UUID) (dataplane.AppSubnet, error) {
	args := mgr.Called(appGroupID)
	return args.Get(0).(dataplane.AppSubnet), args.Error(1)
}
func (mgr *MockDataplaneManager) RemoveApplicationSubnet(appGroupID uuid.UUID) {}
func (mgr *MockDataplaneManager) RemoveVNFSubnet(vnfGroupID uuid.UUID)         {}
func (mgr *MockDataplaneManager) AddApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) (*net.IPNet, string, error) {
	args := mgr.Called(appGroupID, appInstanceID)
	return args.Get(0).(*net.IPNet), args.String(1), args.Error(2)
}
func (mgr *MockDataplaneManager) RemoveApplicationInstance(appGroupID uuid.UUID, appInstanceID uuid.UUID) error {
	args := mgr.Called(appGroupID, appInstanceID)
	return args.Error(0)
}
func (mgr *MockDataplaneManager) AddVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) (*net.IPNet, string, error) {
	args := mgr.Called(vnfGroupID, vnfInstanceID)
	return args.Get(0).(*net.IPNet), args.String(1), args.Error(2)
}
func (mgr *MockDataplaneManager) RemoveVNFInstance(vnfGroupID uuid.UUID, vnfInstanceID uuid.UUID) error {
	args := mgr.Called(vnfGroupID, vnfInstanceID)
	return args.Error(0)
}
func (mgr *MockDataplaneManager) AddRoute(srcAppGroup uuid.UUID, dstNet net.IPNet, sids []net.IP) error {
	args := mgr.Called(srcAppGroup, dstNet, sids)
	return args.Error(0)
}
func (mgr *MockDataplaneManager) RemoveRoute(srcAppGroup uuid.UUID, dstNet string) error {
	args := mgr.Called(srcAppGroup, dstNet)
	return args.Error(0)
}
