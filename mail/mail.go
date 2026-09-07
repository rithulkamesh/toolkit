// Package mail sends transactional email over SMTP using only the standard
// library. It works with any SMTP relay — Amazon SES, Postmark, Mailgun,
// Resend, a self-hosted server — on the submission port (587), where STARTTLS
// is negotiated automatically by net/smtp.
package mail

import (
	"fmt"
	"net/smtp"
	"strings"
)

// Sender delivers a single HTML email. Depend on this interface so tests can
// substitute [LogSender].
type Sender interface {
	Send(to, subject, htmlBody string) error
}

// SMTP is an SMTP-backed [Sender].
type SMTP struct {
	addr string
	auth smtp.Auth
	from string
}

// NewSMTP builds an SMTP sender. from may carry a display name, e.g.
// "Acme <no-reply@acme.example>"; the envelope sender is the bare address.
func NewSMTP(host string, port int, username, password, from string) *SMTP {
	return &SMTP{
		addr: fmt.Sprintf("%s:%d", host, port),
		auth: smtp.PlainAuth("", username, password, host),
		from: from,
	}
}

// Send delivers an HTML email to a single recipient.
func (m *SMTP) Send(to, subject, htmlBody string) error {
	return m.send(to, subject, `text/html; charset="UTF-8"`, htmlBody)
}

// SendText delivers a plain-text email to a single recipient.
func (m *SMTP) SendText(to, subject, body string) error {
	return m.send(to, subject, `text/plain; charset="UTF-8"`, body)
}

func (m *SMTP) send(to, subject, contentType, body string) error {
	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
		"",
		body,
	}, "\r\n")
	return smtp.SendMail(m.addr, m.auth, bareAddress(m.from), []string{to}, []byte(msg))
}

// bareAddress strips a display name: "Acme <x@y>" -> "x@y".
func bareAddress(from string) string {
	if i := strings.LastIndex(from, "<"); i >= 0 {
		return strings.TrimRight(from[i+1:], ">")
	}
	return from
}

// SentMessage is one email captured by [LogSender].
type SentMessage struct{ To, Subject, Body string }

// LogSender records messages instead of sending them — for local development
// and tests. The zero value is ready to use.
type LogSender struct{ Sent []SentMessage }

// Send appends the message to Sent and returns nil.
func (l *LogSender) Send(to, subject, htmlBody string) error {
	l.Sent = append(l.Sent, SentMessage{To: to, Subject: subject, Body: htmlBody})
	return nil
}
