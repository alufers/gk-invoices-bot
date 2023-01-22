package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"gorm.io/driver/sqlite" // Sqlite driver based on GGO

	"gorm.io/gorm"
)

var bot *tgbotapi.BotAPI
var db *gorm.DB

func sendError(chatID int64, err error) {
	log.Printf("sending error error: %v", err)
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Error: %v", err))
	bot.Send(msg)
}

func checkAuthorization(update tgbotapi.Update, command, args string, chatID int64) *AuthorizedUser {
	var from *tgbotapi.User
	if update.Message != nil {
		from = update.Message.From
	} else if update.CallbackQuery != nil {
		from = update.CallbackQuery.From
	}
	if command == "authorize" {
		if args != config.TelegramToken {
			log.Printf("wrong token")
			msg := tgbotapi.NewMessage(chatID, "Please provide a valid bot token as an argument to authorize.")
			bot.Send(msg)
			return nil
		}
		user := &AuthorizedUser{
			TelegramID: from.ID,
			UserName:   from.UserName,
		}
		if err := db.Create(&user).Error; err != nil {
			sendError(chatID, err)
			return nil
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("User %v authorized", from.ID))
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)
		return nil
	}
	// check if user is authorized
	user := &AuthorizedUser{}
	if err := db.Where("telegram_id = ?", from.ID).First(&user).Error; err != nil {
		log.Printf("user %v is not authorized", from.ID)
		sendError(chatID, fmt.Errorf("user %v is not authorized", from.ID))
		return nil
	}
	return user
}

func HandleMessage(update tgbotapi.Update) {
	if update.Message != nil {
		log.Printf("[%s (%v)] %s", update.Message.From.UserName, update.Message.From.ID, update.Message.Text)
	} else if update.CallbackQuery != nil {
		log.Printf("[%s (%v)] %s", update.CallbackQuery.From.UserName, update.CallbackQuery.From.ID, update.CallbackQuery.Data)
	}
	var command string
	var args string
	var chatID int64
	var messageID int

	if update.Message != nil {
		command = update.Message.Command()
		args = update.Message.CommandArguments()
		chatID = update.Message.Chat.ID
		messageID = update.Message.MessageID
	} else if update.CallbackQuery != nil {
		split := strings.SplitN(update.CallbackQuery.Data, " ", 2)
		command = split[0]
		command = strings.TrimPrefix(command, "/")
		if len(split) > 1 {
			args = split[1]
		}
		chatID = update.CallbackQuery.Message.Chat.ID
		messageID = update.CallbackQuery.Message.MessageID
	}
	authorizedUser := checkAuthorization(update, command, args, chatID)
	if authorizedUser == nil {
		return
	}
	log.Printf("command: %v, args: %v", command, args)
	switch command {
	case "invoices":
		if args == "" {
			months, err := db.Raw("SELECT strftime('%Y-%m', created_at) as month, COUNT(*) as count FROM invoices  GROUP BY month ORDER BY month DESC").Rows()
			if err != nil {
				sendError(chatID, err)
				return
			}
			keyboard := tgbotapi.NewInlineKeyboardMarkup()
			for months.Next() {
				var month string
				var count int
				months.Scan(&month, &count)

				countStr := fmt.Sprintf("%d invoice", count)
				if count > 1 {
					countStr += "s"
				}
				keyboard.InlineKeyboard = append(
					keyboard.InlineKeyboard,
					tgbotapi.NewInlineKeyboardRow(
						tgbotapi.NewInlineKeyboardButtonData(month+" ("+countStr+")", "/invoices "+month),
					))
			}

			msg := tgbotapi.NewMessage(chatID, "Provide a year and a month (YYYY-MM) as an argument to get invoices for a specific month.")
			msg.ReplyToMessageID = messageID
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
			return
		}

		month := args
		month = strings.TrimSpace(month)
		var invoices []Invoice
		if err := db.Where("strftime('%Y-%m', created_at) = ?", month).Find(&invoices).Error; err != nil {
			sendError(chatID, err)
			return
		}
		if len(invoices) == 0 {
			msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("No invoices found for %v", month))
			msg.ReplyToMessageID = messageID
			bot.Send(msg)
			return
		}
		invoicesStr := ""
		for _, invoice := range invoices {
			date := invoice.CreatedAt.Format("2006-01-02")
			invoicesStr += fmt.Sprintf("%v: %v\n", date, invoice.FileName)

		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Invoices for %v:\n %v", month, invoicesStr))
		msg.ReplyToMessageID = messageID
		bot.Send(msg)
		msg = tgbotapi.NewMessage(chatID, "Generating ZIP file...")
		msg.ReplyToMessageID = messageID
		progressMsg, err := bot.Send(msg)
		if err != nil {
			sendError(chatID, err)
			return
		}
		buf := new(bytes.Buffer)
		w := zip.NewWriter(buf)
		for _, invoice := range invoices {
			month := invoice.CreatedAt.Format("2006-01")
			f, err := w.Create("GK_faktury_" + month + "/" + invoice.FileName)
			if err != nil {
				sendError(chatID, err)
			}
			if _, err := f.Write(invoice.Contents); err != nil {
				sendError(chatID, err)
			}
		}
		if err := w.Close(); err != nil {
			sendError(chatID, err)
		}
		zipFile := tgbotapi.FileBytes{
			Name:  "GK_faktury_" + month + ".zip",
			Bytes: buf.Bytes(),
		}
		zipSha256 := fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
		parsedYear, err := strconv.Atoi(month[:4])
		parsedMonth, err := strconv.Atoi(month[5:])
		db.Where("sha256 = ?", zipSha256).FirstOrCreate(&GeneratedZip{
			Sha256:   zipSha256,
			FileName: zipFile.Name,
			Year:     parsedYear,
			Month:    parsedMonth,
		})
		mediaInput := tgbotapi.NewInputMediaDocument(zipFile)
		mg := tgbotapi.NewMediaGroup(chatID, []any{mediaInput})
		if _, err := bot.SendMediaGroup(mg); err != nil {
			sendError(chatID, err)
		}

		//delete progressMsg
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, progressMsg.MessageID)
		bot.Send(deleteMsg)

		return

	case "authorized":
		var users []AuthorizedUser
		if err := db.Find(&users).Error; err != nil {
			sendError(chatID, err)
			return
		}
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Authorized users: %#v", users))
		msg.ReplyToMessageID = messageID
		bot.Send(msg)
	case "notifications":
		if args == "" {
			msg := tgbotapi.NewMessage(chatID, "Choose whether you want to receive notifications to send invoices on this chat.")
			msg.ReplyToMessageID = messageID
			msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData("Yes", "/notifications yes"),
					tgbotapi.NewInlineKeyboardButtonData("No", "/notifications no"),
				),
			)
			bot.Send(msg)
			return
		}
		args = strings.TrimSpace(args)
		if args == "yes" {
			if err := db.Where("telegram_chat_id = ?", chatID).FirstOrCreate(&NotifiedChat{TelegramChatID: chatID}).Error; err != nil {
				sendError(chatID, err)
				return
			}
			msg := tgbotapi.NewMessage(chatID, "You will now receive notifications to send invoices on this chat.")
			msg.ReplyToMessageID = messageID
			bot.Send(msg)
			return
		} else if args == "no" {
			if err := db.Where("telegram_chat_id = ?", chatID).Delete(&NotifiedChat{}).Error; err != nil {
				sendError(chatID, err)
				return
			}
			msg := tgbotapi.NewMessage(chatID, "You will no longer receive notifications to send invoices on this chat.")
			msg.ReplyToMessageID = messageID
			bot.Send(msg)
			return
		} else {
			sendError(chatID, fmt.Errorf("invalid argument: %v (expected yes or no)", args))
			return
		}
	case "checkemail":
		msg := tgbotapi.NewMessage(chatID, "Checking email...")
		msg.ReplyToMessageID = messageID
		progressMsg, err := bot.Send(msg)
		err = doCheckEmail()
		// delete progressMsg
		bot.Send(tgbotapi.NewDeleteMessage(chatID, progressMsg.MessageID))
		if err != nil {
			sendError(chatID, err)
			return
		}
		msg = tgbotapi.NewMessage(chatID, "Email checked.")
		msg.ReplyToMessageID = messageID
		bot.Send(msg)
	}

	// handle invoice upload
	if update.Message != nil && update.Message.Document != nil {
		log.Printf("got document: %#v", update.Message.Document)
		file, err := bot.GetFileDirectURL(update.Message.Document.FileID)
		if err != nil {
			sendError(chatID, err)
			return
		}
		log.Printf("got file url: %v", file)
		resp, err := http.Get(file)
		if err != nil {
			sendError(chatID, err)
			return
		}
		defer resp.Body.Close()

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			sendError(chatID, err)
			return
		}

		if err := processIncomingInvoice(update.Message.Document.FileName, data); err != nil {
			sendError(chatID, err)
			return
		}
		msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Invoice %v saved", update.Message.Document.FileName))
		msg.ReplyToMessageID = update.Message.MessageID
		bot.Send(msg)

	}

}

func main() {
	if err := loadConfig(); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if config.StorageDir == "" {
		log.Fatalf("GK_INVOICES_STORAGE_DIR is not set")
	}
	os.MkdirAll(config.StorageDir, 0755)
	var err error
	db, err = gorm.Open(sqlite.Open(filepath.Join(config.StorageDir, "gk-invoices.db")), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Migrate the schema
	err = db.AutoMigrate(&AuthorizedUser{}, &Invoice{}, &NotifiedChat{}, &GeneratedZip{})
	if err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	if config.TelegramToken == "" {
		log.Fatalf("GK_INVOICES_BOT_TOKEN is not set")
	}

	bot, err = tgbotapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)
	go runNotificationsLoop()
	go runEmailCheckerLoop()
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)
	// set my commands
	commands := []tgbotapi.BotCommand{
		{
			Command:     "start",
			Description: "Start the bot",
		},
		{
			Command:     "invoices",
			Description: "Get invoices list for a given month",
		},
		{
			Command:     "authorize",
			Description: "Authorize yourself to use the bot, provide a bot token as an argument.",
		},
		{
			Command:     "authorized",
			Description: "Get a list of authorized users",
		},
		{
			Command:     "notifications",
			Description: "Enable or disable notifications after the end of each month.",
		},
		{
			Command:     "checkemail",
			Description: "Check email for invoices and send them to the bot.",
		},
	}
	cmdsConfig := tgbotapi.NewSetMyCommands(commands...)
	_, err = bot.Request(cmdsConfig)
	if err != nil {
		log.Fatalf("failed to set commands: %v", err)
	}

	for update := range updates {

		HandleMessage(update)

	}

}
