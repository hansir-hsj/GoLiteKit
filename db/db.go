package db

import (
	"fmt"
	"github/hsj/GoLiteKit/config"
	"github/hsj/GoLiteKit/env"
	"os"
	"path/filepath"
	"time"

	"github.com/go-sql-driver/mysql"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	defaultDB *gorm.DB
)

type Config struct {
	DSN          string `json:"dsn"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	Protocol     string `json:"protocol"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Database     string `json:"database"`
	Charset      string `json:"charset"`
	Timeout      int    `json:"timeout"`
	ReadTimeout  int    `json:"readTimeout"`
	WriteTimeout int    `json:"writeTimeout"`

	MaxOpenConns    int `json:"maxOpenConns"`
	MaxIdleConns    int `json:"maxIdleConns"`
	ConnMaxLifeTime int `json:"connMaxLifeTime"`

	gorm.Config
}

func NewDB() *gorm.DB {
	return defaultDB
}

func parse(conf string) (*Config, error) {
	var dbConfig Config
	if err := config.Parse(conf, &dbConfig); err != nil {
		return nil, err
	}

	if dbConfig.DSN == "" {
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
			Params: map[string]string{
				"charset": dbConfig.Charset,
			},
		}
		dbConfig.DSN = mysqlConfig.FormatDSN()
	}

	return &dbConfig, nil
}

func Init(conf ...string) error {
	var dbConf string
	if len(conf) > 0 {
		dbConf = conf[0]
	} else {
		dbConf = filepath.Join(env.ConfDir(), "db.toml")
	}
	config, err := parse(dbConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database connection: %v\n", err)
		return err
	}
	db, err := gorm.Open(mysqlDriver.Open(config.DSN), config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database connection: %v\n", err)
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get SQL database connection: %v\n", err)
		return err
	}

	if config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	}
	if config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	}
	if config.ConnMaxLifeTime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(config.ConnMaxLifeTime) * time.Second)
	}

	if err := sqlDB.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ping database: %v\n", err)
		return err
	}
	defaultDB = db

	return nil
}
