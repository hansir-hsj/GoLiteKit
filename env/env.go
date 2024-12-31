package env

import (
	"github/hsj/golite/config"
	"os"
)

var defaultEnv = &Env{}

type Env struct {
	AppName string `toml:"appName"`
	RunMode string `toml:"runMode"`
	Addr    string `toml:"addr"`
	RootDir string
	ConfDir string
	LogDir  string
}

func Init(path string) error {
	curPath, err := os.Getwd()
	if err != nil {
		return err
	}
	defaultEnv.RootDir = curPath
	defaultEnv.ConfDir = curPath + "/conf"
	defaultEnv.LogDir = curPath + "/log"
	return config.Parse(path, defaultEnv)
}

func GetAppName() string {
	return defaultEnv.AppName
}

func GetRunMode() string {
	return defaultEnv.RunMode
}

func GetAddr() string {
	return defaultEnv.Addr
}

func GetRootDir() string {
	return defaultEnv.RootDir
}

func GetConfDir() string {
	return defaultEnv.ConfDir
}

func GetLogDir() string {
	return defaultEnv.LogDir
}
