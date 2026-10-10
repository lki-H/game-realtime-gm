package experiment

import (
	"context"
	"database/sql"
	"errors"
)

func (projection Projection) Applied(ctx context.Context, envelope Envelope) (bool, error) {
	var fingerprint, status string
	err := projection.DB.QueryRowContext(ctx, "SELECT fingerprint,status FROM r5_receipts WHERE message_id=?", envelope.MessageID).Scan(&fingerprint, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if fingerprint != envelope.Fingerprint() {
		return false, ErrConflict
	}
	return status == "applied", nil
}
