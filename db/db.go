package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
	"github.com/hansir-hsj/GoLiteKit/env"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// DbTimeout values are in milliseconds.
type DbTimeout struct {
	Timeout      int `toml:"timeout"`
	ReadTimeout  int `toml:"readTimeout"`
	WriteTimeout int `toml:"writeTimeout"`
}

// DbConn configures the connection pool. ConnMaxLifeTime is in milliseconds.
type DbConn struct {
	MaxOpenConns    int `toml:"maxOpenConns"`
	MaxIdleConns    int `toml:"maxIdleConns"`
	ConnMaxLifeTime int `toml:"connMaxLifeTime"`
}

type DbConfig struct {
	// Driver selects the backend, "mysql" by default. Other drivers must be
	// registered, e.g. by importing GoLiteKit/db/sqlite.
	Driver    string `toml:"driver"`
	DSN       string `toml:"dsn"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	Protocol  string `toml:"protocol"`
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	Database  string `toml:"database"`
	Charset   string `toml:"charset"`
	ParseTime bool   `toml:"parseTime"`

	// Nested as [db.Timeout] and [db.Conn], matching db.toml.
	DbTimeout `toml:"Timeout"`
	DbConn    `toml:"Conn"`
}

type Config struct {
	DbConfig `toml:"db"`
}

func parse(conf string) (*Config, error) {
	var dbConfig Config
	if err := config.Parse(conf, &dbConfig); err != nil {
		return nil, err
	}

	if dbConfig.Driver == "" {
		dbConfig.Driver = DriverMySQL
	}
	if dbConfig.Driver != DriverMySQL && dbConfig.DSN == "" {
		return nil, fmt.Errorf("dsn is required for driver %q", dbConfig.Driver)
	}

	if dbConfig.DSN == "" {
		protocol := dbConfig.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		// Only include charset when explicitly configured; an empty value causes
		// "unknown charset" errors on some MySQL server versions.
		params := map[string]string{}
		if dbConfig.Charset != "" {
			params["charset"] = dbConfig.Charset
		}

		timeout := time.Duration(dbConfig.Timeout) * time.Millisecond
		if timeout <= 0 {
			timeout = DefaultConnectTimeout // the driver default is no dial timeout
		}
		mysqlConfig := mysql.Config{
			User:                 dbConfig.Username,
			Passwd:               dbConfig.Password,
			Net:                  protocol,
			Addr:                 fmt.Sprintf("%s:%d", dbConfig.Host, dbConfig.Port),
			DBName:               dbConfig.Database,
			Timeout:              timeout,
			ReadTimeout:          time.Duration(dbConfig.ReadTimeout) * time.Millisecond,
			WriteTimeout:         time.Duration(dbConfig.WriteTimeout) * time.Millisecond,
			ParseTime:            dbConfig.ParseTime,
			AllowNativePasswords: true,
			Params:               params,
		}
		dbConfig.DSN = mysqlConfig.FormatDSN()
	}

	return &dbConfig, nil
}

// NewFromConfig opens a database from a config file. Without an argument it
// uses [HttpServer.DB] configFile of the default env, or db.toml in ConfDir.
func NewFromConfig(conf ...string) (*gorm.DB, error) {
	var dbConf string
	switch {
	case len(conf) > 0:
		dbConf = conf[0]
	case env.DBConfigFile() != "":
		dbConf = env.DBConfigFile()
	default:
		dbConf = filepath.Join(env.ConfDir(), "db.toml")
	}

	cfg, err := parse(dbConf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse db config: %w", err)
	}

	drv, err := lookupDriver(cfg.Driver)
	if err != nil {
		return nil, err
	}

	// Connect and ping with a timeout before handing the pool to gorm: gorm's
	// own startup queries (automatic ping, SELECT VERSION()) use no context
	// and would hang on an unresponsive server.
	sqlDB, err := sql.Open(drv.Name, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifeTime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifeTime) * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.pingTimeout())
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db, err := gorm.Open(drv.Dialector(sqlDB), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	return db, nil
}

// DefaultConnectTimeout bounds connecting to and pinging the database when
// no timeout is configured.
const DefaultConnectTimeout = 5 * time.Second

// pingTimeout is the configured connect timeout, or DefaultConnectTimeout.
func (c *Config) pingTimeout() time.Duration {
	if c.Timeout > 0 {
		return time.Duration(c.Timeout) * time.Millisecond
	}
	return DefaultConnectTimeout
}

// Close closes the database connection
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Ping checks if the database connection is alive
func Ping(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
