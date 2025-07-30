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
		&models.NetworkServiceChain{},
		&models.ChainElement{},
		&models.RouteSegment{},
		&models.Route{},
	)

	// Setup many-to-many relationship for Route and Segment
	db.SetupJoinTable(&models.Route{}, "RouteSegments", &models.RouteSegment{})

	return &Registry{
		dbHandle: db,
	}, nil
}
