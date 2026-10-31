package main

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/qaribhaider/ollama-to-lmstudio-symlinks/internal/ui"
)

// selectModels asks which of items to link, all preselected. proceed is false
// when the user cancelled or selected nothing, which is not an error.
func selectModels[T any](items []T, name, label func(T) string) (selected []T, proceed bool, err error) {
	var options []huh.Option[string]
	for _, item := range items {
		options = append(options, huh.NewOption(label(item), name(item)).Selected(true))
	}

	var selectedNames []string
	err = huh.NewMultiSelect[string]().
		Title("Select models to link").
		Description("Use Space to toggle, Enter to confirm").
		Options(options...).
		Value(&selectedNames).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			ui.PrintInfo("Cancelled")
			return nil, false, nil
		}
		return nil, false, err
	}

	if len(selectedNames) == 0 {
		ui.PrintWarning("No models selected")
		return nil, false, nil
	}
	return filterByNames(items, selectedNames, name), true, nil
}

// filterByNames returns the items whose name is in names, in the order of names.
func filterByNames[T any](items []T, names []string, name func(T) string) []T {
	var filtered []T
	for _, wanted := range names {
		for _, item := range items {
			if name(item) == wanted {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

// printFound lists items for the non-interactive path, which links everything.
func printFound[T any](kind string, items []T, line func(T) string) {
	ui.PrintSubheader(fmt.Sprintf("Found %d %s", len(items), kind))
	for _, item := range items {
		ui.PrintBullet(line(item))
	}
	ui.PrintEmptyLine()
}

// countLinked runs link on every item and counts successes and skips.
func countLinked[T any](items []T, link func(T) bool) (created, skipped int) {
	for _, item := range items {
		if link(item) {
			created++
		} else {
			skipped++
		}
	}
	return created, skipped
}

// printResults prints the closing summary box shared by every command.
func printResults(lines ...string) {
	fmt.Println(ui.FormatSummary(append([]string{ui.HeaderStyle.Render("Results")}, lines...)...))
}
