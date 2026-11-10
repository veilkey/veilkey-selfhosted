package db

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newEmailOTPDB(t *testing.T) *DB {
	t.Helper()
	d, err := New(filepath.Join(t.TempDir(), "otp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func saveEmailOTPTestChallenge(t *testing.T, d *DB) string {
	t.Helper()
	expires := time.Now().UTC().Add(time.Minute)
	challenge := &EmailOTPChallenge{Token: "otp-test", Email: "otp@example.invalid", Status: "pending", CodeHash: "test-hash", CodeExpiresAt: &expires}
	if err := d.SaveEmailOTPChallenge(challenge); err != nil {
		t.Fatal(err)
	}
	return challenge.Token
}

func TestEmailOTPAttemptsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "otp.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	token := saveEmailOTPTestChallenge(t, d)
	for attempt := 0; attempt < 2; attempt++ {
		if err := d.VerifyEmailOTPChallenge(token, 3, func(*EmailOTPChallenge) bool { return false }); !errors.Is(err, ErrEmailOTPInvalid) {
			t.Fatalf("wrong attempt: %v", err)
		}
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.VerifyEmailOTPChallenge(token, 3, func(*EmailOTPChallenge) bool { return false }); !errors.Is(err, ErrEmailOTPLimited) {
		t.Fatalf("restart reset attempts: %v", err)
	}
	challenge, err := d.GetEmailOTPChallenge(token)
	if err != nil || challenge.FailedAttempts != 3 || challenge.Status != "locked" {
		t.Fatalf("lockout not persisted: challenge %+v, error %v", challenge, err)
	}
}

func TestEmailOTPConcurrentFailedAttempts(t *testing.T) {
	d := newEmailOTPDB(t)
	token := saveEmailOTPTestChallenge(t, d)
	results := make(chan error, 24)
	var wg sync.WaitGroup
	for worker := 0; worker < cap(results); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- d.VerifyEmailOTPChallenge(token, 6, func(*EmailOTPChallenge) bool { return false })
		}()
	}
	wg.Wait()
	close(results)
	invalid := 0
	for err := range results {
		if errors.Is(err, ErrEmailOTPInvalid) {
			invalid++
		} else if !errors.Is(err, ErrEmailOTPLimited) {
			t.Errorf("unexpected verification error: %v", err)
		}
	}
	challenge, err := d.GetEmailOTPChallenge(token)
	if err != nil || challenge.FailedAttempts != 6 || challenge.Status != "locked" || invalid != 5 {
		t.Fatalf("concurrent attempt budget: challenge %+v, invalid %d, error %v", challenge, invalid, err)
	}
}

func TestEmailOTPConcurrentSendBudget(t *testing.T) {
	d := newEmailOTPDB(t)
	token := saveEmailOTPTestChallenge(t, d)
	results := make(chan error, 24)
	var wg sync.WaitGroup
	for worker := 0; worker < cap(results); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- d.IssueEmailOTPCode(token, "replacement-hash", time.Now().Add(time.Minute), 6, 2, time.Nanosecond)
		}()
	}
	wg.Wait()
	close(results)
	sent := 0
	for err := range results {
		if err == nil {
			sent++
		} else if !errors.Is(err, ErrEmailOTPLimited) {
			t.Errorf("unexpected send error: %v", err)
		}
	}
	challenge, err := d.GetEmailOTPChallenge(token)
	if err != nil || sent < 1 || sent > 3 || challenge.SendCount != sent {
		t.Fatalf("concurrent send budget: challenge %+v, sent %d, error %v", challenge, sent, err)
	}
	// Requests timestamped before a concurrent delivery may still be in cooldown.
	// Exhaust any remaining budget sequentially, then verify the terminal bound.
	for remaining := sent; remaining < 3; remaining++ {
		if err := d.IssueEmailOTPCode(token, "replacement-hash", time.Now().Add(time.Minute), 6, 2, time.Nanosecond); err != nil {
			t.Fatalf("remaining send budget: %v", err)
		}
	}
	if err := d.IssueEmailOTPCode(token, "replacement-hash", time.Now().Add(time.Minute), 6, 2, time.Nanosecond); !errors.Is(err, ErrEmailOTPLimited) {
		t.Fatalf("exhausted send budget: %v", err)
	}
	challenge, err = d.GetEmailOTPChallenge(token)
	if err != nil || challenge.SendCount != 3 {
		t.Fatalf("final send budget: challenge %+v, error %v", challenge, err)
	}
}

func TestEmailOTPConcurrentVerificationSingleUse(t *testing.T) {
	d := newEmailOTPDB(t)
	token := saveEmailOTPTestChallenge(t, d)
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for worker := 0; worker < cap(results); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- d.VerifyEmailOTPChallenge(token, 6, func(*EmailOTPChallenge) bool { return true })
		}()
	}
	wg.Wait()
	close(results)
	verified := 0
	for err := range results {
		if err == nil {
			verified++
		} else if !errors.Is(err, ErrEmailOTPInvalid) {
			t.Errorf("unexpected verification error: %v", err)
		}
	}
	if verified != 1 {
		t.Fatalf("successful verifications = %d, want 1", verified)
	}
}

func TestEmailOTPConcurrentResendInvalidatesCheckedCode(t *testing.T) {
	d := newEmailOTPDB(t)
	token := saveEmailOTPTestChallenge(t, d)
	checked := make(chan struct{})
	resume := make(chan struct{})
	result := make(chan error, 1)
	var once sync.Once
	go func() {
		result <- d.VerifyEmailOTPChallenge(token, 6, func(challenge *EmailOTPChallenge) bool {
			once.Do(func() {
				close(checked)
				<-resume
			})
			return challenge.CodeHash == "test-hash"
		})
	}()
	<-checked
	err := d.IssueEmailOTPCode(token, "replacement-hash", time.Now().Add(time.Minute), 6, 2, time.Nanosecond)
	close(resume)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, ErrEmailOTPInvalid) {
		t.Fatalf("stale code accepted after resend: %v", err)
	}
	challenge, err := d.GetEmailOTPChallenge(token)
	if err != nil || challenge.Status != "pending" || challenge.FailedAttempts != 1 {
		t.Fatalf("unexpected challenge after stale verification: %+v, error %v", challenge, err)
	}
}
