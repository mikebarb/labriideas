// Package params resolves tool parameters with the three-way precedence
// the bulk tools agreed on:
//
//  1. command-line flag  — explicit (-csv, -mp3-dir, -log-dir)
//  2. environment variable — from .env or the environment (BULK_CSV_FILE, …)
//  3. Windows browse dialog — zenity, only when a human is present
//
// The tool declares its flags with env values as defaults, then calls
// Resolve with the flag value and whether it was explicitly set. A
// dialog is attempted only when neither flag nor env supplies a value,
// and its failure is reported — a scripted run with no desktop must
// fail with a clear message, not block.
package params

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/ncruces/zenity"
)

// Spec describes one parameter: its flag name and env var (both used in
// error messages), and what kind of value it is.
type Spec struct {
	Flag   string // flag name, e.g. "csv" — must match the real flag name
	Env    string // env var, e.g. "BULK_CSV_FILE"
	What   string // human name, e.g. "CSV file"
	Dir    bool   // true = pick a directory, false = pick a file
	Filter string // file dialog pattern, e.g. "*.csv" (ignored for dirs)
}

// Resolve returns the value for one parameter.
//
// The flag's default is the env value, so "flag not explicitly set"
// already carries the env value; the caller must pass the flag value
// and whether it was explicitly set (flag.Visit).
func Resolve(spec Spec, val string, changed bool) (string, error) {
	if changed && val == "" {
		return "", fmt.Errorf("-%s was given an empty value", spec.Flag)
	}
	if changed && val != "" {
		return val, nil // 1. explicit flag wins
	}
	if val != "" {
		return val, nil // 2. unchanged flag default = env value
	}
	// 3. Nothing from flag or env. A dialog is the last resort and only
	// works when a human is present. Its failure is reported, never
	// silently swallowed — a run with no desktop must fail with a clear
	// message, not block.
	path, err := pick(spec)
	if errors.Is(err, zenity.ErrCanceled) {
		return "", fmt.Errorf("%s not set: browse dialog cancelled", spec.What)
	}
	if err != nil {
		return "", fmt.Errorf(
			"%s not set: %v\n"+
				"  supply it in one of three ways:\n"+
				"    1. flag:      -%s <value>\n"+
				"    2. env:       %s=<value> in .env or environment\n"+
				"    3. dialog:    leave both unset and run interactively",
			spec.What, err, spec.Flag, spec.Env)
	}
	if path == "" {
		return "", fmt.Errorf("%s not set: browse dialog cancelled", spec.What)
	}
	return filepath.Clean(path), nil
}

// pick opens the native dialog. zenity on Windows uses the real Win32
// dialogs — no cgo, no external binary.
func pick(spec Spec) (string, error) {
	opts := []zenity.Option{zenity.Title("Select " + spec.What)}
	if spec.Dir {
		opts = append(opts, zenity.Directory())
	} else if spec.Filter != "" {
		opts = append(opts, zenity.FileFilters{zenity.FileFilter{
			Name:     "Files",
			Patterns: []string{spec.Filter},
		}})
	}
	path, err := zenity.SelectFile(opts...)
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}
