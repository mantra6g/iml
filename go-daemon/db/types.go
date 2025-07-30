package db

import (
	"gorm.io/gorm"
)

// Registry is the main entrypoint to accessing the database.
// It provides methods to find applications, app instances, VNFs, and VNF instances.
// It uses GORM for database operations.
type Registry struct {
	dbHandle *gorm.DB
}
