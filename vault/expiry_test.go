package vault

import (
	"testing"
	"time"
)

func TestParseExpiryRejectsNonDates(t *testing.T) {
	for _, bad := range []string{"", "2026-13-01", "01/02/2026", "2026-12-01T00:00:00Z", "soon"} {
		if _, err := ParseExpiry(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	if _, err := ParseExpiry("2026-12-01"); err != nil {
		t.Fatalf("valid date rejected: %v", err)
	}
}

func TestSetExpiryOnlyTouchesAnExistingRecord(t *testing.T) {
	c := &Config{}
	if c.SetExpiry("nope", "2026-12-01") {
		t.Fatal("an entry with no metadata record accepted an expiry")
	}
	c.RecordSecret("stripe", "STRIPE_KEY", "")
	if !c.SetExpiry("stripe", "2026-12-01") {
		t.Fatal("a recorded secret refused an expiry")
	}
	if c.Secrets["stripe"].Expires != "2026-12-01" {
		t.Fatalf("expires = %q", c.Secrets["stripe"].Expires)
	}
	if c.ExpiryOf("stripe") != "2026-12-01" {
		t.Fatalf("ExpiryOf = %q", c.ExpiryOf("stripe"))
	}
	// Clearing it leaves the rest of the record alone.
	c.SetExpiry("stripe", "")
	if c.Secrets["stripe"].Expires != "" || c.Secrets["stripe"].Env != "STRIPE_KEY" {
		t.Fatalf("meta = %+v", c.Secrets["stripe"])
	}
}

func TestExpiriesAreSortedBySoonest(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	c := &Config{}
	c.RecordSecret("later", "", "")
	c.RecordSecret("overdue", "", "")
	c.RecordGroup("soon", "")
	c.RecordSecret("undated", "", "")
	c.SetExpiry("later", "2026-12-01")
	c.SetExpiry("overdue", "2026-10-09")
	c.SetExpiry("soon", "2026-10-21")

	got, err := c.Expiries(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (undated must be omitted)", len(got))
	}
	want := []string{"overdue", "soon", "later"}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if !got[0].Expired() || got[0].Days != -10 {
		t.Fatalf("overdue = %+v, want 10 days past", got[0])
	}
	if got[1].Expired() || got[1].Days != 2 {
		t.Fatalf("soon = %+v, want due in 2 days", got[1])
	}
}

// A date that falls today is not yet expired.
func TestExpiryTodayIsNotPast(t *testing.T) {
	now := time.Date(2026, 10, 19, 23, 59, 0, 0, time.UTC)
	c := &Config{}
	c.RecordSecret("today", "", "")
	c.SetExpiry("today", "2026-10-19")

	got, err := c.Expiries(now)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Expired() || got[0].Days != 0 {
		t.Fatalf("today = %+v, want 0 days and not expired", got[0])
	}
}

// A hand-edited config must surface, not be silently skipped.
func TestExpiriesReportsAnUnparseableDate(t *testing.T) {
	c := &Config{}
	c.RecordSecret("broken", "", "")
	c.Secrets["broken"] = SecretMeta{Expires: "next tuesday"}

	if _, err := c.Expiries(time.Now()); err == nil {
		t.Fatal("expected an error naming the bad entry")
	}
}

// An expiry is metadata; it must never reach the keystore.
func TestExpiryIsNotStoredInTheEntry(t *testing.T) {
	v := newFileVault(t)
	if err := v.AddSecret("stripe", "sk-test-value"); err != nil {
		t.Fatal(err)
	}
	e, err := v.Get("stripe")
	if err != nil {
		t.Fatal(err)
	}
	if e.Fields != nil || e.Links != nil {
		t.Fatalf("entry carries more than a value: %+v", e)
	}
}
