package logger

import (
	"os"
)

type Rotater interface {
	needRotate() bool
	Rotate(filePath string, file *os.File) error
}
