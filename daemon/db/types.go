package db

import (
	"iml-daemon/models"

	"github.com/google/uuid"
)

type Registry interface {
	FindActiveAppByLocalID(localID uuid.UUID) (*models.Application, error)
	FindActiveAppByGlobalID(globalID string) (*models.Application, error)
	FindLocalAppGroupByGlobalID(globalAppID string) (*models.AppGroup, error)
	FindAppGroupByID(id uuid.UUID) (*models.AppGroup, error)
	FindAppInstanceByContainerID(containerID string) (*models.AppInstance, error)
	FindActiveNetworkFunctionByLocalID(localID uuid.UUID) (*models.VirtualNetworkFunction, error)
	FindActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error)
	FindCompleteActiveNetworkFunctionByGlobalID(globalID string) (*models.VirtualNetworkFunction, error)
	FindVnfInstanceByContainerID(containerID string) (*models.SimpleVnfInstance, error)
	FindVnfMultiplexedInstanceByContainerID(containerID string) (*models.MultiplexedVnfInstance, error)
	FindVnfGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.SimpleVnfGroup, error)
	FindSubfunctionPreloadedVnfMultiplexedGroupByID(id uuid.UUID) (*models.MultiplexedVnfGroup, error)
	FindVnfMultiplexedGroupByIDWithPrepopulatedInstanceList(id uuid.UUID) (*models.MultiplexedVnfGroup, error)
	FindLocalSimpleVnfGroupByVnfID(globalVnfID string) (*models.SimpleVnfGroup, error)
	FindLocalMultiplexedGroupByVnfID(globalVnfID string) (*models.MultiplexedVnfGroup, error)
	FindAllNetworkServiceChains() ([]models.ServiceChain, error)
	FindActiveNetworkServiceChainByGlobalID(globalChainID string) (*models.ServiceChain, error)
	FindActiveNodeByGlobalID(globalNodeID string) (*models.Worker, error)
	FindRemoteAppGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteAppGroup, error)
	FindRemoteVnfGroupByNodeAndExternalID(nodeID string, groupID string) (*models.RemoteVnfGroup, error)
	SaveApp(app *models.Application) error
	SaveAppGroup(group *models.AppGroup) error
	SaveAppInstance(instance *models.AppInstance) error
	SaveVnf(function *models.VirtualNetworkFunction) error
	SaveSubfunctions(subfunctions []models.Subfunction) error
	SaveVnfGroup(group *models.SimpleVnfGroup) error
	SaveVnfMultiplexedGroup(group *models.MultiplexedVnfGroup) error
	SaveVnfInstance(instance *models.SimpleVnfInstance) error
	SaveVnfMultiplexedInstance(instance *models.MultiplexedVnfInstance) error
	SaveNetworkServiceChain(chain *models.ServiceChain) error
	RemoveAppInstance(id uuid.UUID) error
	RemoveAppGroup(id uuid.UUID) error
	RemoveVnfInstance(id uuid.UUID) error
	RemoveVnfMultiplexedInstance(id uuid.UUID) error
	RemoveVnfGroup(id uuid.UUID) error
	RemoveVnfMultiplexedGroup(id uuid.UUID) error
	SaveRemoteAppGroup(group *models.RemoteAppGroup) error
	SaveRemoteVnfGroup(group *models.RemoteVnfGroup) error
	SaveNode(node *models.Worker) error
	MarkAppAsDeleted(globalID string) error
	MarkVnfAsDeleted(globalID string) error
	MarkServiceChainAsDeleted(globalID string) error
	MarkNodeAsDeleted(globalID string) error
	RemoveRemoteAppInstancesByIP(ips []string, groupID uuid.UUID) error
	RemoveRemoteAppGroupsByGlobalAppID(globalAppID string) error
	RemoveRemoteVnfGroupsByGlobalVnfID(globalVnfID string) error
}
