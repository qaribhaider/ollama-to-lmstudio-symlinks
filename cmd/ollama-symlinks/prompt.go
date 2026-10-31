package main

import (
	"errors"

	"github.com/charmbracelet/huh"
)

// askConfirm shows a yes/no prompt. See confirmOutcome for how results are read.
func askConfirm(title string) (bool, error) {
	var confirmed bool
	err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		Run()
	return confirmOutcome(confirmed, err)
}

// confirmOutcome separates a user cancel (a plain "no") from a prompt that could
// not run at all, such as when there is no TTY. The latter is returned so the
// command exits non-zero instead of reporting "Cancelled" and exiting 0.
func confirmOutcome(confirmed bool, err error) (bool, error) {
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return confirmed, nil
}
