package ws

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewConnectionID(playerID int64, connectedAt time.Time) (string, error) {
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	return fmt.Sprintf("conn_%d_%d_%s", playerID, connectedAt.UnixNano(), hex.EncodeToString(randomBytes)), nil
}
