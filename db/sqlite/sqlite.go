// Package sqlite registers a pure-Go (no cgo) SQLite driver for package db.
// Import it for side effects and set driver = "sqlite" in db.toml; dsn is the
// database file, e.g. "file:data/app.db?_pragma=journal_mode(WAL)".
package sqlite

import (
	"github.com/hansir-hsj/GoLiteKit/db"

	glebarez "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func init() {
	db.RegisterDriver("sqlite", db.Driver{
		Name: glebarez.DriverName,
		Dialector: func(conn gorm.ConnPool) gorm.Dialector {
			return &glebarez.Dialector{Conn: conn}
		},
	})
}
