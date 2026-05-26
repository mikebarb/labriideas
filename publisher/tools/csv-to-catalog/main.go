package main

import (
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mikebarb/labriideas-publisher/pkg/schema"
)

// "github.com/mikebarb/labriideas-publisher/pkg/storage" // ADJUST TO YOUR MODULE NAME
// "github.com/mikebarb/labriideas-publisher/pkg/storage" // ADJUST TO YOUR MODULE NAME
func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run main.go <input.csv> <output.json.gz>")
		os.Exit(1)
	}

	inputFile := os.Args[1]
	outputFile := os.Args[2]

	// 1. Open the CSV file
	csvFile, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Error opening CSV: %v\n", err)
		os.Exit(1)
	}
	defer csvFile.Close()

	// 2. Parse the CSV
	csvReader := csv.NewReader(csvFile)
	records, err := csvReader.ReadAll()
	if err != nil {
		fmt.Printf("Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	if len(records) == 0 {
		fmt.Println("CSV file is empty.")
		os.Exit(1)
	}

	// 1. BUILD THE HEADER MAP
	headerMap := make(map[string]int)
	for i, colName := range records[0] {
		headerMap[colName] = i
	}

	// 3. Map Header Indices (so order doesn't matter)
	//header := records[0]
	//colIndex := make(map[string]int)
	//for i, colName := range header {
	//	colIndex[colName] = i
	//}

	// Helper to safely get column data
	//getCol := func(row []string, colName string) string {
	//	if idx, ok := colIndex[colName]; ok && idx < len(row) {
	//		return row[idx]
	//	}
	//	return ""
	//}

	// Helper to safely extract data by column name
	getValue := func(row []string, columnName string) string {
		if idx, exists := headerMap[columnName]; exists {
			if idx < len(row) {
				return row[idx]
			}
		}
		return "" // Return blank if column is missing
	}

	// 2. PARSE ROWS DYNAMICALLY USING THE GLOBAL SCHEMA
	var tracks []map[string]string // Dynamic map instead of struct!

	for i, row := range records {
		if i == 0 {
			continue // Skip header row
		}

		trackData := make(map[string]string)
		// The schema drives the structure. No hardcoding!
		for _, fieldName := range schema.CatalogSchema {
			trackData[fieldName] = getValue(row, fieldName)
		}

		tracks = append(tracks, trackData)
	}

	// 4. Convert Rows to Track Structs
	//var tracks []storage.Track
	//for i, row := range records {
	//	if i == 0 {
	//		continue // Skip header row
	//	}
	//
	//	track := storage.Track{
	//		ID:       getCol(row, "id"),
	//		Title:    getCol(row, "title"),
	//		Artist:   getCol(row, "artist"),
	//		FileName: getCol(row, "filename"),
	//	}
	//	tracks = append(tracks, track)
	//}

	// 5. Build the Catalog Object
	//catalog := storage.Catalog{
	//	Version: fmt.Sprintf("csv-import-%s", time.Now().Format("2006-01-02-15-04-05")),
	//	Count:   len(tracks),
	//	Tracks:  tracks,
	//}

	// 3. BUILD CATALOG DYNAMICALLY
	// We use a generic interface map for the wrapper
	catalogData := map[string]interface{}{
		"release": fmt.Sprintf("csv-import-%s", time.Now().Format("2006-01-02-15-04-05")),
		"count":   len(tracks),
		"tracks":  tracks,
	}

	// 6. Marshal to JSON
	//jsonData, err := json.MarshalIndent(catalog, "", "  ")
	//if err != nil {
	//	fmt.Printf("Error marshaling JSON: %v\n", err)
	//	os.Exit(1)
	//}

	// 4. MARSHAL & COMPRESS
	// This outputs EXACTLY the same JSON structure as before
	jsonData, err := json.MarshalIndent(catalogData, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	// 7. Create the Output Gzip File
	outFile, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	// 8. Write compressed data via Gzip Writer
	gzWriter := gzip.NewWriter(outFile)
	gzWriter.Name = "catalog.json" // Metadata inside the gz file

	_, err = gzWriter.Write(jsonData)
	if err != nil {
		fmt.Printf("Error writing compressed data: %v\n", err)
		os.Exit(1)
	}

	// MUST close the gzip writer to flush the compressed buffers to disk!
	gzWriter.Close()

	fmt.Printf("✅ Successfully converted %d dynamic tracks to %s\n", len(tracks), outputFile)
}
