package db

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var (
	ErrEmailOTPLimited = errors.New("email otp challenge limit reached")
	ErrEmailOTPInvalid = errors.New("email otp challenge is invalid or expired")
)

func (d *DB) SaveEmailOTPChallenge(challenge *EmailOTPChallenge) error {
	if challenge == nil {
		return fmt.Errorf("email otp challenge is required")
	}
	if challenge.Token == "" {
		return fmt.Errorf("token is required")
	}
	if challenge.Email == "" {
		return fmt.Errorf("email is required")
	}
	if challenge.Status == "" {
		challenge.Status = "pending"
	}
	return d.conn.Save(challenge).Error
}

func (d *DB) GetEmailOTPChallenge(token string) (*EmailOTPChallenge, error) {
	var challenge EmailOTPChallenge
	if err := d.conn.First(&challenge, "token = ?", token).Error; err != nil {
		return nil, fmt.Errorf("email otp challenge %s not found", token)
	}
	return &challenge, nil
}

// IssueEmailOTPCode reserves a delivery before sending mail, including failed deliveries.
// A resend never replenishes the challenge's verification budget.
func (d *DB) IssueEmailOTPCode(token, codeHash string, expiresAt time.Time, maxAttempts, maxResends int, cooldown time.Duration) error {
	if maxAttempts <= 0 || maxResends < 0 || cooldown <= 0 {
		return fmt.Errorf("invalid email otp limits")
	}
	now := time.Now().UTC()
	result := d.conn.Model(&EmailOTPChallenge{}).
		Where("token = ? AND status = ? AND failed_attempts < ? AND send_count <= ?", token, "pending", maxAttempts, maxResends).
		Where("last_sent_at IS NULL OR last_sent_at <= ?", now.Add(-cooldown)).
		Updates(map[string]any{
			"code_hash": codeHash, "code_expires_at": expiresAt,
			"send_count": gorm.Expr("send_count + 1"), "last_sent_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 0 {
		return nil
	}
	challenge, err := d.GetEmailOTPChallenge(token)
	if err != nil {
		return err
	}
	if challenge.Status == "pending" || challenge.Status == "locked" {
		return ErrEmailOTPLimited
	}
	return ErrEmailOTPInvalid
}

// VerifyEmailOTPChallenge commits verification or a failed attempt against the exact
// version that was checked. Concurrent sends/verifications force a fresh check.
func (d *DB) VerifyEmailOTPChallenge(token string, maxAttempts int, validate func(*EmailOTPChallenge) bool) error {
	if maxAttempts <= 0 || validate == nil {
		return fmt.Errorf("invalid email otp verification policy")
	}
	for {
		challenge, err := d.GetEmailOTPChallenge(token)
		if err != nil {
			return err
		}
		if challenge.Status == "locked" || challenge.FailedAttempts >= maxAttempts {
			return ErrEmailOTPLimited
		}
		if challenge.Status != "pending" {
			return ErrEmailOTPInvalid
		}
		valid := validate(challenge)
		now := time.Now().UTC()
		valid = valid && challenge.CodeExpiresAt != nil && now.Before(*challenge.CodeExpiresAt)
		updates := map[string]any{}
		if valid {
			updates["status"] = "verified"
			updates["used_at"] = now
		} else {
			updates["failed_attempts"] = challenge.FailedAttempts + 1
			if challenge.FailedAttempts+1 >= maxAttempts {
				updates["status"] = "locked"
			}
		}
		result := d.conn.Model(&EmailOTPChallenge{}).
			Where("token = ? AND status = ? AND failed_attempts = ? AND send_count = ? AND code_hash = ?",
				token, "pending", challenge.FailedAttempts, challenge.SendCount, challenge.CodeHash).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		if valid {
			return nil
		}
		if challenge.FailedAttempts+1 >= maxAttempts {
			return ErrEmailOTPLimited
		}
		return ErrEmailOTPInvalid
	}
}
