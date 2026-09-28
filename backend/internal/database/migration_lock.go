package database

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"time"
)

// MySQL advisory locks belong to a connection, so pin that connection for the
// entire initialization. Never GET_LOCK on an arbitrary pooled GORM session.
func lockMigration(db *gorm.DB) (func(), error) {
	if db.Dialector.Name() != "mysql" {
		return func() {}, nil
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	// Initialization also uses the pool while the advisory-lock connection is
	// pinned. Reserve room for it even in a one-connection deployment.
	maxOpen := pool.Stats().MaxOpenConnections
	if maxOpen == 1 {
		pool.SetMaxOpenConns(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pool.Conn(ctx)
	if err != nil {
		pool.SetMaxOpenConns(maxOpen)
		return nil, err
	}
	for {
		var acquired int
		// Each server wait stays below the driver's 15-second read timeout.
		// The outer context bounds the entire attempt across migration replicas.
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(CONCAT('arre-schema:', DATABASE()), 1)").Scan(&acquired); err != nil {
			conn.Close()
			pool.SetMaxOpenConns(maxOpen)
			return nil, errors.New("schema migration lock unavailable")
		}
		if acquired == 1 {
			break
		}
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(CONCAT('arre-schema:', DATABASE()))")
		_ = conn.Close()
		pool.SetMaxOpenConns(maxOpen)
	}, nil
}
