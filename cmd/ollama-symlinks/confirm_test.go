package main

import (
	"errors"
	"testing"

	"github.com/charmbracelet/huh"
)

func TestConfirmOutcome(t *testing.T) {
	promptFailure := errors.New("open /dev/tty: no such device or address")

	tests := []struct {
		name      string
		confirmed bool
		err       error
		wantOK    bool
		wantErr   error
	}{
		{"user said yes", true, nil, true, nil},
		{"user said no", false, nil, false, nil},
		{"user cancelled with Esc or Ctrl+C", false, huh.ErrUserAborted, false, nil},
		{"prompt could not run, e.g. no TTY", false, promptFailure, false, promptFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, gotErr := confirmOutcome(tt.confirmed, tt.err)
			if gotOK != tt.wantOK || !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("confirmOutcome(%v, %v) = (%v, %v), want (%v, %v)",
					tt.confirmed, tt.err, gotOK, gotErr, tt.wantOK, tt.wantErr)
			}
		})
	}
}
