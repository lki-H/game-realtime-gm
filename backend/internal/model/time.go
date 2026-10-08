package model

import "time"

func LegacyDatetime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), time.FixedZone("Asia/Shanghai", 8*60*60))
}
