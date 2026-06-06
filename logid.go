package golitekit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

func generateLogID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000")))[:16]
	}
	return hex.EncodeToString(b)
}

func EnsureLogID(ctx context.Context) string {
	gcx := GetContext(ctx)
	if gcx == nil {
		return ""
	}

	gcx.dataLock.Lock()
	defer gcx.dataLock.Unlock()
	if gcx.logID == "" {
		gcx.logID = generateLogID()
	}
	return gcx.logID
}

func SetLogID(ctx context.Context, logID string) {
	if logID == "" {
		return
	}
	gcx := GetContext(ctx)
	if gcx == nil {
		return
	}

	gcx.dataLock.Lock()
	defer gcx.dataLock.Unlock()
	gcx.logID = logID
}
