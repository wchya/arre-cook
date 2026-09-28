package database

import "gorm.io/gorm"

// Handle carries a request context or an existing transaction through domain
// services. The default is reserved for startup, maintenance and legacy tests.
// HTTP and agent entry points explicitly supply a context-bound handle.
func Handle(dbs ...*gorm.DB) *gorm.DB {
	if len(dbs) > 0 && dbs[0] != nil {
		return dbs[0]
	}
	return DB
}
