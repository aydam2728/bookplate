package auth_test

import (
	"testing"

	"github.com/aydam2728/bookplate/internal/auth"
)

func TestLimiter_AllowsUntilTheAttemptsRunOut(t *testing.T) {
	l := auth.NewLimiter()

	allowed := 0
	for i := 0; i < 50; i++ {
		if !l.Allow("1.2.3.4") {
			break
		}
		l.Record("1.2.3.4")
		allowed++
	}

	if allowed == 0 {
		t.Fatal("the first attempt was refused")
	}
	if allowed >= 50 {
		t.Fatal("attempts were never throttled: credential stuffing is unimpeded")
	}
	if l.Allow("1.2.3.4") {
		t.Error("attempts are allowed again immediately after the limit")
	}
}

func TestLimiter_IsPerKey(t *testing.T) {
	l := auth.NewLimiter()
	for i := 0; i < 50 && l.Allow("1.2.3.4"); i++ {
		l.Record("1.2.3.4")
	}

	if !l.Allow("5.6.7.8") {
		t.Error("one throttled address locked out everybody else")
	}
}

func TestLimiter_ResetClearsTheHistory(t *testing.T) {
	l := auth.NewLimiter()
	for i := 0; i < 50 && l.Allow("1.2.3.4"); i++ {
		l.Record("1.2.3.4")
	}

	l.Reset("1.2.3.4")

	if !l.Allow("1.2.3.4") {
		t.Error("a successful login did not clear the failures, so one " +
			"forgetful evening locks someone out")
	}
}
