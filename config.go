package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type Config struct {
	TelegramToken      string `json:"telegram_token"`
	StorageDir         string `json:"storage_dir"`
	NagInterval        string `json:"nag_interval"`
	ImapAddress        string `json:"imap_address"`
	ImapUsername       string `json:"imap_username"`
	ImapPassword       string `json:"imap_password"`
	EmailCheckInterval string `json:"email_check_interval"`

	NotificationsStartTime *TimeOfDay `json:"notifications_start_time"`
	NotificationsEndTime   *TimeOfDay `json:"notifications_end_time"`
}

var config Config = Config{
	NotificationsStartTime: &TimeOfDay{},
	NotificationsEndTime:   &TimeOfDay{},
}

func loadConfig() error {
	filePath := os.Getenv("GK_INVOICES_CONFIG_FILE")
	if filePath == "" {
		filePath = "config.json"
	}
	configFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer configFile.Close()
	byteValue, err := ioutil.ReadAll(configFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		return err
	}
	return nil
}
