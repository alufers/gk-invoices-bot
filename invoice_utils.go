package main

import (
	"crypto/sha256"
	"fmt"
)

func processIncomingInvoice(filename string, contents []byte) error {
	sha265 := fmt.Sprintf("%x", sha256.Sum256(contents))
	invoice := &Invoice{
		FileName: filename,
		Contents: contents,
		Sha256:   sha265,
	}
	if db.Where("sha256 = ?", sha265).First(&invoice).Error == nil {

		return fmt.Errorf("invoice with this sha256 already exists")
	}
	if err := db.Create(&invoice).Error; err != nil {

		return err
	}
	return nil
}
