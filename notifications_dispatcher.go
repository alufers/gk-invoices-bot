package main

import (
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type MonthToNotify struct {
	Year  int
	Month int
}

func (m MonthToNotify) String() string {
	// print with leading zero
	return fmt.Sprintf("%v-%02v", m.Year, m.Month)
}

func doSendNotifications() {
	notifiedChats := []NotifiedChat{}
	if err := db.Find(&notifiedChats).Error; err != nil {
		log.Printf("error getting notified chats: %v", err)
		return
	}
	rows, err := db.Raw("SELECT strftime('%Y', created_at) as year, strftime('%m', created_at) as month, COUNT(*) as count FROM invoices GROUP BY year, month ORDER BY year DESC, month DESC").Rows()
	if err != nil {
		log.Printf("error getting months to notify: %v", err)
		return
	}
	monthsToNotify := []MonthToNotify{}
	currYear, currMonth, _ := time.Now().Date()

	for rows.Next() {
		var year, month, count int
		rows.Scan(&year, &month, &count)
		log.Printf("INV: year %v, month %v, count %v", year, month, count)
		// if is current or future month, skip
		if year > currYear || (year == currYear && month >= int(currMonth)) {
			continue
		}

		monthsToNotify = append(monthsToNotify, MonthToNotify{Year: year, Month: month})
	}
	for _, notifiedChat := range notifiedChats {
		// the months that should be notified to this chat
		monthsToNotifyToSend := []MonthToNotify{}
		for _, monthToNotify := range monthsToNotify {
			if notifiedChat.LastAcknowledgedYear < monthToNotify.Year || (notifiedChat.LastAcknowledgedYear == monthToNotify.Year && notifiedChat.LastAcknowledgedMonth < monthToNotify.Month) {
				monthsToNotifyToSend = append(monthsToNotifyToSend, monthToNotify)
			}
		}
		if len(monthsToNotifyToSend) == 0 {
			continue
		}
		// send notifications
		msg := tgbotapi.NewMessage(notifiedChat.TelegramChatID, "❗ You have invoices to send to accounting for the following months:")
		keyboard := tgbotapi.NewInlineKeyboardMarkup()
		for _, monthToNotify := range monthsToNotifyToSend {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(monthToNotify.String(), "/invoices "+monthToNotify.String())))
		}
		msg.ReplyMarkup = keyboard
		if _, err := bot.Send(msg); err != nil {
			log.Printf("error sending notification to chat %v: %v", notifiedChat.TelegramChatID, err)
		}

	}
}

func notifyAllChats(contents string) {
	notifiedChats := []NotifiedChat{}
	if err := db.Find(&notifiedChats).Error; err != nil {
		log.Printf("error getting notified chats: %v", err)
		return
	}
	for _, notifiedChat := range notifiedChats {
		msg := tgbotapi.NewMessage(notifiedChat.TelegramChatID, contents)
		msg.ParseMode = "HTML"
		if _, err := bot.Send(msg); err != nil {
			log.Printf("error sending notification to chat %v: %v", notifiedChat.TelegramChatID, err)
		}
	}
}
func runNotificationsLoop() {
	dur, err := time.ParseDuration(config.NagInterval)
	if err != nil || dur < time.Second {
		log.Fatalf("invalid nag interval: %v", err)
	}
	for {
		log.Printf("running notifications loop")
		doSendNotifications()
		time.Sleep(dur)
	}
}
