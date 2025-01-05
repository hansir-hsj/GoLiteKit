package logger

type Rotater interface {
	NeedRotate() bool
	Rotate() error
	NewFilePath(filePath string) string
}
