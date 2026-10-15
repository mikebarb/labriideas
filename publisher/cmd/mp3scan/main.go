package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/ncruces/zenity"
)

// dotInterval: a progress dot is printed for the first MP3 in each
// directory, then one dot for every dotInterval files after that.
const dotInterval = 10

// header is the fixed schema agreed in the spec. Column order matters:
// it defines the CSV layout and must not change between runs.
var header = []string{
	"file_path",
	"dir_path",
	"file_name",
	"Title",
	"Artist",
	"Album_Artist",
	"Album",
	"Year",
	"Genre",
	"Track_Number",
	"Disc_Number",
	"Composer",
	"Grouping",
	"Comment",
}

func main() {
	startTime := time.Now()

	// ---- 1. Chooser: directory to scan ----
	scanRoot, err := zenity.SelectFile(
		zenity.Title("Select the mp3 directory to scan"),
		zenity.Directory(),
	)
	if err != nil {
		// User cancelled the dialog.
		fmt.Println("Cancelled: no directory selected.")
		return
	}

	// ---- 2. Chooser: output file location and name ----
	outPath, err := zenity.SelectFileSave(
		zenity.Title("Save the inventory file as..."),
		zenity.ConfirmOverwrite(),
		zenity.FileFilters{
			zenity.FileFilter{
				Name:     "CSV file (comma delimited)",
				Patterns: []string{"*.csv"},
			},
			zenity.FileFilter{
				Name:     "TXT file (tab delimited)",
				Patterns: []string{"*.txt"},
			},
		},
	)
	if err != nil {
		// User cancelled the dialog.
		fmt.Println("Cancelled: no output file selected.")
		return
	}

	// ---- 3. Determine delimiter from the chosen extension ----
	// .txt -> tab delimited, anything else (default .csv) -> comma.
	ext := strings.ToLower(filepath.Ext(outPath))
	delimiter := ','
	if ext == ".txt" {
		delimiter = '\t'
	} else if ext != ".csv" {
		// Normalise any other extension to .csv so Excel opens it cleanly.
		outPath += ".csv"
	}

	// ---- 4. Create the output file ----
	outFile, err := os.Create(outPath)
	if err != nil {
		fmt.Printf("Error: cannot create output file %s: %v\n", outPath, err)
		return
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	writer.Comma = delimiter
	defer writer.Flush()

	// Write the fixed header row first.
	if err := writer.Write(header); err != nil {
		fmt.Printf("Error: cannot write header row: %v\n", err)
		return
	}

	// ---- 5. Walk the directory tree ----
	startProcessingTime := time.Now()
	fmt.Printf("Scanning: %s\n", scanRoot)

	dirCount := 0
	fileCount := 0
	errorCount := 0
	currentDir := ""
	dotCount := 0 // files processed since the last dot in the current directory

	err = filepath.WalkDir(scanRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("\nError accessing %s: %v\n", path, err)
			errorCount++
			return nil // skip problem entries but keep walking
		}

		if d.IsDir() {
			// Announce each directory (other than the already-announced root)
			// on its own line.
			if path != scanRoot {
				fmt.Printf("\n%s\n", path)
			}
			dirCount++
			currentDir = path
			dotCount = 0
			return nil
		}

		// Filter strictly for .mp3 files (case-insensitive).
		if !strings.EqualFold(filepath.Ext(path), ".mp3") {
			return nil
		}

		row := scanFile(path)

		if err := writer.Write(row); err != nil {
			fmt.Printf("\nError writing row for %s: %v\n", path, err)
			errorCount++
		}

		fileCount++

		// ---- Progress dots ----
		// Dot on the first file processed in a directory, then one dot
		// every dotInterval files after that.
		dotCount++
		if dotCount == 1 || dotCount%dotInterval == 0 {
			fmt.Print(".")
		}

		_ = currentDir
		return nil
	})

	if err != nil {
		fmt.Printf("\nError during directory walk: %v\n", err)
		errorCount++
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		fmt.Printf("Error flushing output file: %v\n", err)
		errorCount++
	}

	// ---- 6. Completion summary ----
	fmt.Printf("\n\nScan complete.\n")
	fmt.Printf("  Directories scanned: 	%d\n", dirCount)
	fmt.Printf("  MP3 files processed: 	%d\n", fileCount)
	fmt.Printf("  Errors encountered:  	%d\n", errorCount)
	fmt.Printf("  Output file:         	%s\n", outPath)
	fmt.Printf("  Total processingtime: %s\n", time.Since(startProcessingTime).Round(time.Millisecond))
	fmt.Printf("  Total runtime:       	%s\n", time.Since(startTime).Round(time.Millisecond))
}

// scanFile extracts the row data for a single MP3 file.
// It relies strictly on ID3 tags for speed, avoiding a full audio stream decode.
func scanFile(path string) []string {
	row := make([]string, len(header))
	for i := range row {
		row[i] = ""
	}

	row[0] = path
	row[1] = filepath.Dir(path)
	row[2] = filepath.Base(path)

	// ---- ID3 tags ----
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		if metadata, err := tag.ReadFrom(f); err == nil {
			row[3] = metadata.Title()
			row[4] = metadata.Artist()
			row[5] = metadata.AlbumArtist()
			row[6] = metadata.Album()

			if year := metadata.Year(); year != 0 {
				row[7] = strconv.Itoa(year)
			}

			row[8] = metadata.Genre()
			// Track and disc come back as "n" or "n/total" - keep raw.
			if track, total := metadata.Track(); track != 0 {
				if total != 0 {
					row[9] = fmt.Sprintf("%d/%d", track, total)
				} else {
					row[9] = fmt.Sprintf("%d", track)
				}
			}

			if disc, total := metadata.Disc(); disc != 0 {
				if total != 0 {
					row[10] = fmt.Sprintf("%d/%d", disc, total)
				} else {
					row[10] = fmt.Sprintf("%d", disc)
				}
			}

			row[11] = metadata.Composer()
			// dhowden/tag maps TIT1 (content group) into its Raw() map.
			if raw, ok := metadata.Raw()["grouping"]; ok {
				if s, ok := raw.(string); ok {
					row[12] = s
				}
			}

			if comment := metadata.Comment(); comment != "" {
				row[13] = comment
			}

		}
	}

	return row
}
