package service

import (
	"time"
)

var manilaLocation *time.Location

func init() {
	var err error
	manilaLocation, err = time.LoadLocation("Asia/Manila")
	if err != nil {
		// Fallback to UTC+8 if timezone data is not available
		manilaLocation = time.FixedZone("Manila", 8*60*60)
	}
}

// GetManilaLocation returns the Manila timezone location
func GetManilaLocation() *time.Location {
	return manilaLocation
}

// GetTodayManilaDate returns today's date in Manila timezone as a time.Time with time set to midnight
func GetTodayManilaDate() time.Time {
	now := time.Now().In(manilaLocation)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, manilaLocation)
}

// GetYesterdayManilaDate returns yesterday's date in Manila timezone as a time.Time with time set to midnight
func GetYesterdayManilaDate() time.Time {
	today := GetTodayManilaDate()
	return today.AddDate(0, 0, -1)
}

// ConvertToManilaDate converts any time to a Manila date (midnight of that day in Manila timezone)
func ConvertToManilaDate(t time.Time) time.Time {
	manilaTime := t.In(manilaLocation)
	return time.Date(manilaTime.Year(), manilaTime.Month(), manilaTime.Day(), 0, 0, 0, 0, manilaLocation)
}

// IsSameManilaDate checks if two times represent the same calendar day in Manila timezone
func IsSameManilaDate(t1, t2 time.Time) bool {
	date1 := ConvertToManilaDate(t1)
	date2 := ConvertToManilaDate(t2)
	return date1.Equal(date2)
}

// GetManilaTimeNow returns the current time in Manila timezone
func GetManilaTimeNow() time.Time {
	return time.Now().In(manilaLocation)
}

// FormatManilaDate formats a Manila date for display (e.g., "January 23, 2025")
func FormatManilaDate(t time.Time) string {
	return t.In(manilaLocation).Format("January 23, 2025")
}
