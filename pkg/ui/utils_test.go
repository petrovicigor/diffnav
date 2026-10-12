package ui

import "testing"

func TestTeeAt(t *testing.T) {
	block := "ab│\n│ c│\nxy│"
	cases := []struct {
		row  int
		want string
	}{
		{-1, block},
		{3, block},
		{0, "ab├\n│ c│\nxy│"},
		{1, "ab│\n│ c├\nxy│"}, // only the last bar of the row
		{2, "ab│\n│ c│\nxy├"},
	}
	for _, c := range cases {
		if got := teeAt(block, c.row); got != c.want {
			t.Errorf("row %d: got %q, want %q", c.row, got, c.want)
		}
	}
	if got := teeAt("abc\ndef", 0); got != "abc\ndef" {
		t.Errorf("a row without a border must be left alone, got %q", got)
	}
}
