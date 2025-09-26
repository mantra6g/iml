package vnfs

import (
	"fmt"
	"iml-daemon/db"
	"iml-daemon/models"
	"iml-daemon/services/iml"
)

type Repository struct {
	db        *db.Registry
	imlClient *iml.Client
}

func NewRepository(db *db.Registry, imlClient *iml.Client) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("database registry is required")
	}
	if imlClient == nil {
		return nil, fmt.Errorf("IML client is required")
	}

	return &Repository{
		db:        db,
		imlClient: imlClient,
	}, nil
}

func (r *Repository) GetNF(globalID string) (*models.VirtualNetworkFunction, error) {
	// First check if the NF exists in the local database
	nf, err := r.db.FindActiveNetworkFunctionByGlobalID(globalID)
	if err == nil {
		return nf, nil
	}

	// If not found locally, fetch from IML.
	// This will automatically keep the local database in sync with IML.
	nf, err = r.imlClient.PullNetworkFunction(globalID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch NF %s from IML: %v", globalID, err)
	}

	return nf, nil
}
