package scheduler

import (
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	cases := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},
		{"*/5 * * * *", false},
		{"0 0 1 1 *", false},
		{"0 0 * * 0", false},
		{"0 0 * * 7", false}, // Sunday alias
		{"0-30 * * * *", false},
		{"0,15,30,45 * * * *", false},
		{"bad", true},
		{"* * * *", true},    // 4 fields
		{"60 * * * *", true}, // out of range
	}
	for _, c := range cases {
		_, err := ParseCron(c.expr)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseCron(%q): err=%v want=%v", c.expr, err, c.wantErr)
		}
	}
}

func TestCronNext(t *testing.T) {
	c, err := ParseCron("*/5 * * * *")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 4, 14, 12, 3, 30, 0, time.UTC)
	next, err := c.Next(from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 14, 12, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next: got %v want %v", next, want)
	}
}

func TestCronNextDaily(t *testing.T) {
	c, err := ParseCron("0 3 * * *")
	if err != nil {
		t.Fatal(err)
	}
	// Already past 03:00; next is tomorrow.
	from := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	next, _ := c.Next(from)
	want := time.Date(2026, 4, 15, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("daily Next: got %v want %v", next, want)
	}
}
