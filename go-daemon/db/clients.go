package db

import (
	"fmt"
	"iml-daemon/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// InMemoryRegistry is the main entrypoint to accessing the database.
// It provides methods to find applications, app instances, VNFs, and VNF instances.
// It uses GORM for database operations.
type InMemoryRegistry struct {
	dbHandle *gorm.DB
}

func InitializeInMemoryRegistry() (Registry, error) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	// // Auto-migrate the RouteStage model first to ensure its "position" field is created
	// err = db.AutoMigrate(&models.RouteStage{})
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to auto migrate database: %w", err)
	// }

	// Migrate the schema
	err = db.AutoMigrate(
		&models.Application{},
		&models.AppInstance{},
		&models.VirtualNetworkFunction{},
		&models.Subfunction{},
		&models.SimpleVnfGroup{},
		&models.SimpleVnfInstance{},
		&models.MultiplexedVnfGroup{},
		&models.MultiplexedVnfInstance{},
		&models.SidAssignment{},
		&models.ServiceChain{},
		&models.ServiceChainVnfs{},
		// &models.Route{},
		&models.Worker{},
		&models.RemoteAppGroup{},
		&models.RemoteAppInstance{},
		&models.RemoteVnfGroup{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}

	// // Setup many-to-many relationship for Route and Segment
	// err = db.SetupJoinTable(&models.Route{}, "Stages", &models.RouteStage{})
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to setup join table for Route and RouteStage: %w", err)
	// }

	return &InMemoryRegistry{
		dbHandle: db,
	}, nil
}
