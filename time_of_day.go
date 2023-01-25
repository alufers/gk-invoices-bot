package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type TimeOfDay struct {
	Hour   int
	Minute int
}

func (t TimeOfDay) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// json unmarshalling
func (t *TimeOfDay) UnmarshalJSON(b []byte) error {
	var dat any
	if err := json.Unmarshal(b, &dat); err != nil {
		return err
	}
	if _, ok := dat.(string); !ok {
		return fmt.Errorf("invalid time of day format type: %s", string(b))
	}
	parts := strings.Split(dat.(string), ":")

	if len(parts) != 2 {
		return fmt.Errorf("invalid time of day format: %s", string(b))
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return err
	}
	if hour < 0 || hour > 23 {
		return fmt.Errorf("invalid hour: %d", hour)
	}
	if minute < 0 || minute > 59 {
		return fmt.Errorf("invalid minute: %d", minute)
	}
	t.Hour = hour
	t.Minute = minute
	return nil
}

// json marshalling
func (t TimeOfDay) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", t.String())), nil
}

func (t TimeOfDay) IsTimeAfter(time time.Time) bool {
	return time.Hour() > t.Hour || (time.Hour() == t.Hour && time.Minute() > t.Minute)
}
func (t TimeOfDay) IsTimeBefore(time time.Time) bool {
	return time.Hour() < t.Hour || (time.Hour() == t.Hour && time.Minute() < t.Minute)
}
