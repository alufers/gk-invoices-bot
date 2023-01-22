package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
)

var emailCheckMutex sync.Mutex

func setupEmailConn() (*client.Client, error) {
	conn, err := client.DialTLS(config.ImapAddress, nil)
	if err != nil {
		return nil, err
	}
	if err := conn.Login(config.ImapUsername, config.ImapPassword); err != nil {
		return nil, err
	}
	return conn, nil
}

type AttachmentToHandle struct {
	SenderEmail string
	Subject     string
	FileName    string
	MimeType    string
	Content     []byte
	CC          []string
	To          []string
}

func handleEmailAttachment(attachment AttachmentToHandle) error {
	log.Printf("Handling email attachment: %v, %v", attachment.MimeType, attachment.FileName)
	if attachment.MimeType == "application/pdf" {
		err := processIncomingInvoice(attachment.FileName, attachment.Content)
		errStr := "success"
		if err != nil {
			errStr = err.Error()
		}
		notificationText := fmt.Sprintf(
			`Received e-mail invoice:
File name: <b>%v</b>
Subject: <b>%v</b>
Sender: <b>%v</b>
Processing result: <b>%v</b>
			`,
			attachment.FileName, attachment.Subject, attachment.SenderEmail, errStr)

		notifyAllChats(notificationText)

		return nil
	}
	if attachment.MimeType == "application/zip" {
		// calculate sha256 of the zip file
		sha265 := fmt.Sprintf("%x", sha256.Sum256(attachment.Content))
		// check if we know this zip file
		zipFile := &GeneratedZip{}
		if err := db.Where("sha256 = ?", sha265).First(&zipFile).Error; err != nil {
			return nil
		}
		// if we know this zip file, update LastAcknowledgedYear and LastAcknowledgedMonth for every notified chat
		// and send a notification to the chat
		err := db.Model(&NotifiedChat{}).Where("1 = 1").Updates(NotifiedChat{
			LastAcknowledgedMonth: zipFile.Month,
			LastAcknowledgedYear:  zipFile.Year,
		}).Error
		if err != nil {
			return fmt.Errorf("error updating notified chats: %v", err)
		}

		notificationText := fmt.Sprintf(`Received e-mail zip file: <b>%v</b> from <b>%v</b>. 
Marking month %v-%02d as sent. 
CC, To: <b>`, attachment.FileName, attachment.SenderEmail, zipFile.Year, zipFile.Month)
		for _, cc := range attachment.CC {
			notificationText += cc + ", "
		}
		for _, to := range attachment.To {
			notificationText += to + ", "
		}
		notificationText += "</b>"
		notifyAllChats(notificationText)

	}
	return nil
}

// trashy golang function to fetch all eail attachments
func doCheckEmail() error {
	emailCheckMutex.Lock()
	defer emailCheckMutex.Unlock()
	log.Printf("Checking email...")
	conn, err := setupEmailConn()
	if err != nil {
		return err
	}
	defer conn.Logout()
	log.Printf("Connected to email server")

	mbox, err := conn.Select("INBOX", false)
	if err != nil {
		return err
	}
	log.Printf("Found %d messages", mbox.Messages)
	if mbox.Messages == 0 {
		return nil
	}
	// fetch the new messages
	seqset := new(imap.SeqSet)
	from := uint32(1)
	if mbox.Messages > 10 {
		from = mbox.Messages - 10
	}

	seqset.AddRange(from, mbox.Messages)
	messages := make(chan *imap.Message, 10)
	done := make(chan error, 1)
	go func() {
		done <- conn.Fetch(seqset, []imap.FetchItem{imap.FetchRFC822, imap.FetchEnvelope}, messages)
	}()
	select {
	case err := <-done:
		if err != nil {
			return err
		}
	case <-time.After(10 * time.Second):
		return errors.New("Timeout")
	}
	for msg := range messages {
		log.Printf("Message: %v", msg.Envelope.Subject)
		// fetch it's attachments
		for _, p := range msg.Body {
			if p == nil {
				continue
			}
			entity, err := message.Read(p)
			if err != nil {
				return err
			}

			multiPartReader := entity.MultipartReader()

			for e, err := multiPartReader.NextPart(); err != io.EOF; e, err = multiPartReader.NextPart() {
				kind, params, cErr := e.Header.ContentType()
				if cErr != nil {
					return cErr
				}
				log.Printf("Part: %v, %v", kind, params)
				if kind == "multipart/alternative" {
					continue
				}
				attachmentToProcess := &AttachmentToHandle{
					SenderEmail: msg.Envelope.From[0].MailboxName + "@" + msg.Envelope.From[0].HostName,
					Subject:     msg.Envelope.Subject,
					FileName:    params["name"],
					MimeType:    kind,
					CC:          []string{},
					To:          []string{},
				}
				for _, cc := range msg.Envelope.Cc {
					attachmentToProcess.CC = append(attachmentToProcess.CC, cc.MailboxName+"@"+cc.HostName)
				}
				for _, to := range msg.Envelope.To {
					attachmentToProcess.To = append(attachmentToProcess.To, to.MailboxName+"@"+to.HostName)
				}
				attachmentToProcess.Content, err = io.ReadAll(e.Body)
				if err != nil {
					return err
				}
				err = handleEmailAttachment(*attachmentToProcess)
				if err != nil {
					return err
				}

			}

		}
		// delete the message
		seqset := new(imap.SeqSet)
		seqset.AddNum(msg.SeqNum)

		flags := []any{imap.DeletedFlag}
		if err := conn.Store(seqset, imap.FormatFlagsOp(imap.AddFlags, true), flags, nil); err != nil {
			return fmt.Errorf("error deleting message: %v", err)
		}
		// expunge the mailbox
		if err := conn.Expunge(nil); err != nil {
			return fmt.Errorf("error expunging mailbox: %v", err)
		}

	}
	log.Printf("Done checking email")
	return nil

}

func runEmailCheckerLoop() {
	func() {

		emailCheckMutex.Lock()
		defer emailCheckMutex.Unlock()
		log.Printf("Checking email configuration & listing mailboxes...")
		conn, err := setupEmailConn()
		if err != nil {
			log.Fatalf("Error setting up first email connection: %v", err)
		}
		defer conn.Logout()
		// list mailboxes
		mailboxes := make(chan *imap.MailboxInfo, 10)
		done := make(chan error, 1)
		go func() {
			done <- conn.List("", "*", mailboxes)
		}()

		log.Println("Mailboxes:")
		for m := range mailboxes {
			log.Println("* " + m.Name)
		}
	}()
	sleepDuration, _ := time.ParseDuration(config.EmailCheckInterval)
	for {
		err := doCheckEmail()
		if err != nil {
			log.Printf("Error checking email: %v", err)
		}
		time.Sleep(sleepDuration)
	}
}
