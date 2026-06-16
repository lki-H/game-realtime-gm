package model

import "time"

type Player struct {
	ID              int64      `json:"id"`
	Username        string     `json:"username"`
	PasswordHash    string     `json:"-"`
	Nickname        string     `json:"nickname"`
	Status          string     `json:"status"`
	BannedReason    string     `json:"banned_reason,omitempty"`
	BannedAt        *time.Time `json:"banned_at,omitempty"`
	BannedByAdminID *int64     `json:"banned_by_admin_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
