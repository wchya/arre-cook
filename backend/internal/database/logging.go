package database

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

// Driver messages can contain parameters too (e.g. MySQL duplicate key errors).
// Keep the error class/code and SQL template, never driver-provided values.
type SafeLogger struct{ logger.Interface }

func (l SafeLogger) LogMode(level logger.LogLevel) logger.Interface {
	return SafeLogger{l.Interface.LogMode(level)}
}
func (l SafeLogger) ParamsFilter(_ context.Context, sql string, _ ...interface{}) (string, []interface{}) {
	return sql, nil
}
func (l SafeLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		var driver *mysql.MySQLError
		if errors.As(err, &driver) {
			err = fmt.Errorf("database error code=%d", driver.Number)
		} else {
			err = fmt.Errorf("database operation failed (%T)", err)
		}
	}
	l.Interface.Trace(ctx, begin, fc, err)
}
