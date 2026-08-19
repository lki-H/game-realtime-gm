package model

import "time"

type AdminOperationLog struct {
	ID            int64     `json:"id"`
	AdminID       int64     `json:"admin_id"`
	AdminUsername string    `json:"admin_username"`
	AdminRole     string    `json:"admin_role"`
	Action        string    `json:"action"`
	TargetType    string    `json:"target_type"`
	TargetID      *int64    `json:"target_id,omitempty"`
	Detail        string    `json:"detail"`
	IP            string    `json:"ip"`
	UserAgent     string    `json:"user_agent"`
	CreatedAt     time.Time `json:"created_at"`
}
