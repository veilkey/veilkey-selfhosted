package mailer

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/wneessen/go-mail"
)

const defaultSendmailBin = "/usr/sbin/sendmail"
const defaultSMTPPort = 587
const defaultSTARTTLS = true

// Send sends a plain-text email using either SMTP (when VEILKEY_OTP_SMTP_HOST is set)
// or the local sendmail binary.
func Send(to, subject, body string) error {
	from := strings.TrimSpace(os.Getenv("VEILKEY_OTP_FROM"))
	if from == "" {
		from = "veilkey@localhost"
	}
	if strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_HOST")) != "" {
		return sendSMTP(from, to, subject, body)
	}
	return sendSendmail(from, to, subject, body)
}

func sendSendmail(from, to, subject, body string) error {
	sendmailBin := os.Getenv("VEILKEY_OTP_SENDMAIL")
	if sendmailBin == "" {
		sendmailBin = defaultSendmailBin
	}
	if _, err := os.Stat(sendmailBin); err != nil {
		return fmt.Errorf("sendmail binary not found: %s", sendmailBin)
	}
	cmd := exec.Command(sendmailBin, "-t")
	cmd.Stdin = strings.NewReader(formatMail(from, to, subject, body))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sendmail failed: %v: %s", err, string(out))
	}
	return nil
}

func sendSMTP(from, to, subject, body string) error {
	host := strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_HOST"))
	portStr := strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_PORT"))
	port := defaultSMTPPort
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}
	username := strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_USERNAME"))
	password := strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_PASSWORD"))
	startTLSStr := strings.TrimSpace(os.Getenv("VEILKEY_OTP_SMTP_STARTTLS"))
	startTLS := defaultSTARTTLS
	if startTLSStr != "" {
		startTLS = strings.ToLower(startTLSStr) != "false"
	}

	m := mail.NewMsg()
	if err := m.From(from); err != nil {
		return fmt.Errorf("smtp from: %w", err)
	}
	if err := m.To(to); err != nil {
		return fmt.Errorf("smtp to: %w", err)
	}
	m.Subject(subject)
	m.SetBodyString(mail.TypeTextPlain, body)

	tlsPolicy := mail.TLSOpportunistic
	if !startTLS {
		tlsPolicy = mail.NoTLS
	}
	c, err := mail.NewClient(host,
		mail.WithPort(port),
		mail.WithUsername(username),
		mail.WithPassword(password),
		mail.WithTLSPolicy(tlsPolicy),
	)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	return c.DialAndSend(m)
}

func formatMail(from, to, subject, body string) string {
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s", from, to, subject, body)
}
