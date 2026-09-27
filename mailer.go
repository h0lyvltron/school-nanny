package main

import (
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Mailer sends a plain-text message. A nil Mailer means email is not configured.
type Mailer interface {
	Send(to, subject, textBody string) error
}

type mailMessage struct {
	To      string
	Subject string
	Body    string
}

type logMailer struct{}

func (logMailer) Send(to, subject, textBody string) error {
	log.Printf("mail to %s\nsubject: %s\n%s", to, subject, textBody)
	return nil
}

// memoryMailer keeps messages for tests.
type memoryMailer struct {
	mu       sync.Mutex
	messages []mailMessage
}

func (m *memoryMailer) Send(to, subject, textBody string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, mailMessage{To: to, Subject: subject, Body: textBody})
	return nil
}

func (m *memoryMailer) last() (mailMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return mailMessage{}, false
	}
	return m.messages[len(m.messages)-1], true
}

type smtpMailer struct {
	addr     string
	host     string
	username string
	password string
	from     string
}

func (m *smtpMailer) Send(to, subject, textBody string) error {
	body := strings.ReplaceAll(textBody, "\n", "\r\n")
	msg := "From: " + m.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" + body
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}
	return smtp.SendMail(m.addr, auth, m.from, []string{to}, []byte(msg))
}

func mailerFromEnv() Mailer {
	flag := strings.TrimSpace(os.Getenv("MAIL_LOG"))
	if flag == "1" || strings.EqualFold(flag, "true") {
		return logMailer{}
	}
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		return nil
	}
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "587"
	}
	from := strings.TrimSpace(os.Getenv("MAIL_FROM"))
	if from == "" {
		from = "school-nanny@" + host
	}
	return &smtpMailer{
		addr:     host + ":" + port,
		host:     host,
		username: os.Getenv("SMTP_USERNAME"),
		password: os.Getenv("SMTP_PASSWORD"),
		from:     from,
	}
}

func (a *App) sendMail(to, subject, body string) error {
	if a.mailer == nil {
		return fmt.Errorf("email is not configured")
	}
	return a.mailer.Send(to, subject, body)
}
