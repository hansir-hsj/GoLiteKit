package db

import (
	"fmt"
	"sync"

	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// DriverMySQL is the default driver name.
const DriverMySQL = "mysql"

// Driver describes a database backend selectable through [db] driver.
type Driver struct {
	// Name is the database/sql driver name passed to sql.Open.
	Name string
	// Dialector wraps an opened connection pool for gorm.
	Dialector func(conn gorm.ConnPool) gorm.Dialector
}

var (
	driversMu sync.RWMutex
	drivers   = map[string]Driver{
		DriverMySQL: {
			Name: "mysql",
			Dialector: func(conn gorm.ConnPool) gorm.Dialector {
				return mysqlDriver.New(mysqlDriver.Config{Conn: conn})
			},
		},
	}
)

// RegisterDriver makes a driver available under the given config name.
// It panics if the name is empty, the driver is incomplete or the name is taken.
func RegisterDriver(name string, d Driver) {
	if name == "" || d.Name == "" || d.Dialector == nil {
		panic("db: invalid driver registration")
	}
	driversMu.Lock()
	defer driversMu.Unlock()
	if _, dup := drivers[name]; dup {
		panic("db: driver already registered: " + name)
	}
	drivers[name] = d
}

func lookupDriver(name string) (Driver, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	d, ok := drivers[name]
	if !ok {
		return Driver{}, fmt.Errorf("unknown database driver %q (is its package imported?)", name)
	}
	return d, nil
}
