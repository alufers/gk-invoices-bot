package main

import (
	"time"

	"gorm.io/gorm"
)

type AuthorizedUser struct {
	gorm.Model
	TelegramID int64
	UserName   string
}

type NotifiedChat struct {
	gorm.Model
	TelegramChatID         int64
	UserName               string
	LastNotificationSentAt *time.Time
	LastAcknowledgedMonth  int
	LastAcknowledgedYear   int
}

type Invoice struct {
	gorm.Model
	FileName string
	Contents []byte `gorm:"type:blob"`
	Sha256   string
}

type GeneratedZip struct {
	gorm.Model
	FileName string
	Sha256   string `gorm:"unique"`
	Month    int
	Year     int
}
