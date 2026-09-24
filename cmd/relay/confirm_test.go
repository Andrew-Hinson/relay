package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestConfirmApply(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		yes         bool
		interactive bool
		wantErr     string
	}{
		{"yes flag skips prompt", "", true, false, ""},
		{"typed yes", "yes\n", false, true, ""},
		{"typed yes without newline", "yes", false, true, ""},
		{"y is not enough", "y\n", false, true, "cancelled"},
		{"empty input", "", false, true, "cancelled"},
		{"no terminal refuses", "yes\n", false, false, "--yes"},
	}
	for _, c := range cases {
		var out bytes.Buffer
		err := confirmApply(strings.NewReader(c.input), &out, c.yes, c.interactive)
		if c.wantErr == "" && err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Fatalf("%s: got %v, want %q", c.name, err, c.wantErr)
		}
		if c.yes && out.Len() > 0 {
			t.Fatalf("%s: prompted despite --yes", c.name)
		}
	}
	if err := confirmApply(strings.NewReader("no\n"), &bytes.Buffer{}, false, true); !errors.Is(err, errApplyCancelled) {
		t.Fatalf("got %v", err)
	}
}

func TestFormatReview(t *testing.T) {
	if got := formatReview(nil, ""); got != "" {
		t.Fatalf("got %q", got)
	}
	got := formatReview([]planLine{{"role", "acme_acmedb"}}, "CREATE DATABASE acmedb OWNER acme_acmedb\n\nCREATE TABLE IF NOT EXISTS public.orders (\n  id SERIAL PRIMARY KEY\n)\n")
	want := "adopt\n  ~ role acme_acmedb\n\nsql\n  CREATE DATABASE acmedb OWNER acme_acmedb\n\n  CREATE TABLE IF NOT EXISTS public.orders (\n    id SERIAL PRIMARY KEY\n  )\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
