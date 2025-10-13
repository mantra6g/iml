package db

import (
	"fmt"
	"iml-daemon/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func InitializeInMemoryRegistry() (*Registry, error) {
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	// Migrate the schema
	db.AutoMigrate(
		&models.Application{},
		&models.AppInstance{},
		&models.VirtualNetworkFunction{},
		&models.VnfGroup{},
		&models.ServiceChain{},
		&models.ServiceChainVnfs{},
		&models.RouteStage{},
		&models.Route{},
		&models.Worker{},
		&models.RemoteAppGroup{},
		&models.RemoteAppInstance{},
		&models.RemoteVnfGroup{},
	)

	// Setup many-to-many relationship for Route and Segment
	db.SetupJoinTable(&models.Route{}, "RouteSegments", &models.RouteStage{})

	return &Registry{
		dbHandle: db,
	}, nil
}
