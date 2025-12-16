package testutil

import (
	"iml-daemon/models"

	"github.com/stretchr/testify/mock"
)

type MockIMLClient struct {
	mock.Mock
}
func (m *MockIMLClient) GetApplication(id string) (*models.Application, error) {
	args := m.Called(id)
	return args.Get(0).(*models.Application), args.Error(1)
}
func (m *MockIMLClient) GetNetworkFunction(id string) (*models.VirtualNetworkFunction, error) {
	args := m.Called(id)
	return args.Get(0).(*models.VirtualNetworkFunction), args.Error(1)
}