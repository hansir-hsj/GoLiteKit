package db

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hansir-hsj/GoLiteKit/config"
	"github.com/hansir-hsj/GoLiteKit/env"

	"github.com/go-sql-driver/mysql"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type DbTimeout struct {
	Timeout      int `toml:"timeout"`
	ReadTimeout  int `toml:"readTimeout"`
	WriteTimeout int `toml:"writeTimeout"`
}

type DbConn struct {
	MaxOpenConns    int `toml:"maxOpenConns"`
	MaxIdleConns    int `toml:"maxIdleConns"`
	ConnMaxLifeTime int `toml:"connMaxLifeTime"`
}

type DbConfig struct {
	DSN      string `toml:"dsn"`
	Username string `toml:"username"`
	Password string `toml:"password"`
	Protocol string `toml:"protocol"`
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Database string `toml:"database"`
	Charset  string `toml:"charset"`
}

type Config struct {
	DbConfig  `toml:"db"`
	DbTimeout `toml:"Timeout"`
	DbConn    `toml:"Conn"`

	gorm.Config
}

func parse(conf string) (*Config, error) {
	var dbConfig Config
	if err := config.Parse(conf, &dbConfig); err != nil {
		return nil, err
	}

	if dbConfig.DSN == "" {
		// Only include charset when explicitly configured; an empty value causes
		// "unknown charset" errors on some MySQL server versions.
		params := map[string]string{}
		if dbConfig.Charset != "" {
			params["charset"] = dbConfig.Charset
		}

		mysqlConfig := mysql.Config{
			User:                 dbConfig.Username,
			Passwd:               dbConfig.Password,
			Net:                  dbConfig.Protocol,
			Addr:                 fmt.Sprintf("%s:%d", dbConfig.Host, dbConfig.Port),
			DBName:               dbConfig.Database,
			Timeout:              time.Duration(dbConfig.Timeout) * time.Millisecond,
			ReadTimeout:          time.Duration(dbConfig.ReadTimeout) * time.Millisecond,
			WriteTimeout:         time.Duration(dbConfig.WriteTimeout) * time.Millisecond,
			AllowNativePasswords: true,
			Params:               params,
		}
		dbConfig.DSN = mysqlConfig.FormatDSN()
	}

	return &dbConfig, nil
}

// NewFromConfig creates a new database connection from config file
func NewFromConfig(conf ...string) (*gorm.DB, error) {
	var dbConf string
	if len(conf) > 0 {
		dbConf = conf[0]
	} else {
		dbConf = filepath.Join(env.ConfDir(), "db.toml")
	}

	cfg, err := parse(dbConf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse db config: %w", err)
	}

	db, err := gorm.Open(mysqlDriver.Open(cfg.DSN), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL database connection: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifeTime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifeTime) * time.Second)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
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
