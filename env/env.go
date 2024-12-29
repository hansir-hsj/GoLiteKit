package env

import "github/hsj/golite/config"

var Default = &Env{}

type Env struct {
	AppName string
	RunMode string
	RootDir string
	ConfDir string
	LogDir  string
}

func (e *Env) Init(path string) error {
	cnf := config.NewAppConfig()
	return cnf.Parse(path, Default)
}

func (e *Env) GetAppName() string {
	return Default.AppName
}

func (e *Env) GetRunMode() string {
	return Default.RunMode
}

func (e *Env) GetRootDir() string {
	return Default.RootDir
}

func (e *Env) GetConfDir() string {
	return Default.ConfDir
}

func (e *Env) GetLogDir() string {
	return Default.LogDir
}
