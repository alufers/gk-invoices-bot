package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
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
}

var config Config

func loadConfig() error {
	filePath := os.Getenv("GK_INVOICES_CONFIG_FILE")
	if filePath == "" {
		filePath = "config.json"
	}
	configFile, err := os.Open(filePath)
	if err != nil {
		defaultConfig := Config{
			TelegramToken:      os.Getenv("GK_INVOICES_BOT_TOKEN"),
			StorageDir:         os.Getenv("GK_INVOICES_STORAGE_DIR"),
			NagInterval:        "15m0s",
			EmailCheckInterval: "10m0s",
		}
		defaultConfigFile, _ := os.Create(filePath)
		enc := json.NewEncoder(defaultConfigFile)
		enc.SetIndent("", "  ")
		enc.Encode(defaultConfig)
		defaultConfigFile.Close()
		log.Printf("created default config file %v", filePath)
		return err
	}
	defer configFile.Close()
	byteValue, _ := ioutil.ReadAll(configFile)
	json.Unmarshal(byteValue, &config)
	return nil
}
