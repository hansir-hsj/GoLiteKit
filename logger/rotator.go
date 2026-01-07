package logger

type Rotator interface {
	NeedRotate() bool
	Rotate() error
	NewFilePath() string
}
