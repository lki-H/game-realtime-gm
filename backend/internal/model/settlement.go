package model

import "time"

type MissionRecord struct {
	ID                  int64     `json:"id"`
	MissionInstanceID   string    `json:"mission_instance_id"`
	MissionID           string    `json:"mission_id"`
	SquadID             string    `json:"squad_id"`
	SubmittedByPlayerID int64     `json:"submitted_by_player_id"`
	Nonce               string    `json:"nonce"`
	Status              string    `json:"status"`
	CompletionSeconds   int64     `json:"completion_seconds"`
	Score               int64     `json:"score"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type RewardRecord struct {
	ID                int64     `json:"id"`
	MissionRecordID   int64     `json:"mission_record_id"`
	MissionInstanceID string    `json:"mission_instance_id"`
	PlayerID          int64     `json:"player_id"`
	RewardType        string    `json:"reward_type"`
	Amount            int64     `json:"amount"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

type SettlementResult struct {
	Record  MissionRecord  `json:"record"`
	Rewards []RewardRecord `json:"rewards"`
}
