package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/junaid18183/sanduk/vault"
)

func future(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format(vault.ExpiryLayout)
}

func TestAddRecordsAnExpiry(t *testing.T) {
	testEnv(t)
	date := future(30)
	if _, err := runCmd(t, "sk-test-value", "add", "stripe", "--as", "STRIPE_KEY", "--expires", date); err != nil {
		t.Fatalf("add: %v", err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[expires "+date+"]") {
		t.Fatalf("list = %q", out)
	}
}

func TestListFlagsAnExpiredEntry(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "stripe", "--expires", future(-3)); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[EXPIRED ") {
		t.Fatalf("list = %q", out)
	}
}

func TestAddRejectsABadDateBeforeStoringAnything(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "stripe", "--expires", "next tuesday"); err == nil {
		t.Fatal("a non-date was accepted")
	}
	// The refusal must come before the write, not after it.
	if out, err := runCmd(t, "", "list", "--names"); err != nil || strings.Contains(out, "stripe") {
		t.Fatalf("entry was stored despite the bad date: %q (%v)", out, err)
	}
}

func TestRotateMovesTheDeadline(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-one", "add", "stripe", "--expires", future(2)); err != nil {
		t.Fatal(err)
	}
	later := future(400)
	if _, err := runCmd(t, "sk-test-two", "rotate", "stripe", "--expires", later); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[expires "+later+"]") {
		t.Fatalf("list = %q", out)
	}
}

func TestExpiresNoneClearsTheDeadline(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "stripe", "--expires", future(5)); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sk-test-value2", "rotate", "stripe", "--expires", "none"); err != nil {
		t.Fatalf("clear: %v", err)
	}

	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "expires") {
		t.Fatalf("deadline survived: %q", out)
	}
}

func TestGroupExpiry(t *testing.T) {
	testEnv(t)
	date := future(20)
	if _, err := runCmd(t, `{"A_KEY":"v"}`, "add-group", "warden", "--from-json", "-", "--expires", date); err != nil {
		t.Fatalf("add-group: %v", err)
	}
	out, err := runCmd(t, "", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[expires "+date+"]") {
		t.Fatalf("list = %q", out)
	}
}

func TestDoctorExpiryExitsThreeWhenDue(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "fine", "--expires", future(400)); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "sk-test-value", "add", "stale", "--expires", future(-1)); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "", "doctor", "--expiry")
	if !strings.Contains(out, "expired 1 day(s) ago") {
		t.Fatalf("doctor = %q", out)
	}
	// Soonest first: the overdue entry leads.
	if i, j := strings.Index(out, "stale"), strings.Index(out, "fine"); i > j {
		t.Fatalf("not sorted by soonest: %q", out)
	}
	var ee *vault.ExitError
	if !errors.As(err, &ee) || ee.Code != exitDrift {
		t.Fatalf("err = %v, want exit %d", err, exitDrift)
	}
}

func TestDoctorExpiryWithinWindow(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "soon", "--expires", future(10)); err != nil {
		t.Fatal(err)
	}

	// Outside the window: clean.
	if _, err := runCmd(t, "", "doctor", "--expiry", "--within", "3"); err != nil {
		t.Fatalf("doctor --within 3: %v", err)
	}
	// Inside it: drift.
	_, err := runCmd(t, "", "doctor", "--expiry", "--within", "30")
	var ee *vault.ExitError
	if !errors.As(err, &ee) || ee.Code != exitDrift {
		t.Fatalf("err = %v, want exit %d", err, exitDrift)
	}
}

func TestDoctorExpiryIsQuietWithNoDeadlines(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "sk-test-value", "add", "stripe"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, "", "doctor", "--expiry"); err != nil {
		t.Fatalf("doctor: %v", err)
	}
}

func TestDoctorModesAreExclusive(t *testing.T) {
	testEnv(t)
	if _, err := runCmd(t, "", "doctor", "--expiry", "--links"); err == nil {
		t.Fatal("two check modes were accepted together")
	}
}
