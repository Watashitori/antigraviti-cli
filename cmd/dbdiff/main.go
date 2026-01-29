package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

type DBEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func getDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming", "Antigravity", "User", "globalStorage", "state.vscdb")
}

func scanDB() (map[string]string, error) {
	dbPath := getDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT key, value FROM ItemTable")
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		result[key] = value
	}
	return result, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func compareSnapshots(before, after map[string]string) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("COMPARISON RESULTS")
	fmt.Println(strings.Repeat("=", 60))

	// Find deleted keys
	var deleted []string
	for key := range before {
		if _, exists := after[key]; !exists {
			deleted = append(deleted, key)
		}
	}
	sort.Strings(deleted)

	// Find added keys
	var added []string
	for key := range after {
		if _, exists := before[key]; !exists {
			added = append(added, key)
		}
	}
	sort.Strings(added)

	// Find modified keys
	var modified []string
	for key, beforeVal := range before {
		if afterVal, exists := after[key]; exists && beforeVal != afterVal {
			modified = append(modified, key)
		}
	}
	sort.Strings(modified)

	fmt.Printf("\n🗑️  DELETED KEYS (%d):\n", len(deleted))
	if len(deleted) == 0 {
		fmt.Println("   (none)")
	}
	for _, key := range deleted {
		fmt.Printf("   - %s\n", key)
		fmt.Printf("     OLD: %s\n", truncate(before[key], 100))
	}

	fmt.Printf("\n➕ ADDED KEYS (%d):\n", len(added))
	if len(added) == 0 {
		fmt.Println("   (none)")
	}
	for _, key := range added {
		fmt.Printf("   + %s\n", key)
		fmt.Printf("     NEW: %s\n", truncate(after[key], 100))
	}

	fmt.Printf("\n✏️  MODIFIED KEYS (%d):\n", len(modified))
	if len(modified) == 0 {
		fmt.Println("   (none)")
	}
	for _, key := range modified {
		fmt.Printf("   ~ %s\n", key)
		fmt.Printf("     BEFORE: %s\n", truncate(before[key], 100))
		fmt.Printf("     AFTER:  %s\n", truncate(after[key], 100))
	}

	// Save full diff to file
	diffFile := "db_diff.json"
	diffData := map[string]interface{}{
		"deleted":  deleted,
		"added":    added,
		"modified": modified,
		"details": map[string]interface{}{
			"deleted_values":  extractValues(before, deleted),
			"added_values":    extractValues(after, added),
			"modified_before": extractValues(before, modified),
			"modified_after":  extractValues(after, modified),
		},
	}
	jsonData, _ := json.MarshalIndent(diffData, "", "  ")
	os.WriteFile(diffFile, jsonData, 0644)
	fmt.Printf("\n📁 Full diff saved to: %s\n", diffFile)
}

func extractValues(data map[string]string, keys []string) map[string]string {
	result := make(map[string]string)
	for _, key := range keys {
		if val, ok := data[key]; ok {
			result[key] = val
		}
	}
	return result
}

func waitForEnter(msg string) {
	fmt.Print(msg)
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func main() {
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Println("   DATABASE DIFF TOOL - Sign Out Comparison")
	fmt.Println("=" + strings.Repeat("=", 59))
	fmt.Printf("\nDatabase: %s\n", getDBPath())

	// Step 1: Scan before
	waitForEnter("\n[Step 1] Make sure you are LOGGED IN, then press Enter...")
	
	fmt.Println("🔍 Scanning database (BEFORE logout)...")
	before, err := scanDB()
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		waitForEnter("Press Enter to exit...")
		return
	}
	fmt.Printf("✅ Found %d keys\n", len(before))

	// Step 2: Wait for user to logout
	waitForEnter("\n[Step 2] Now click SIGN OUT in the app, then press Enter...")

	// Step 3: Scan after
	fmt.Println("🔍 Scanning database (AFTER logout)...")
	after, err := scanDB()
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		waitForEnter("Press Enter to exit...")
		return
	}
	fmt.Printf("✅ Found %d keys\n", len(after))

	// Step 4: Compare
	compareSnapshots(before, after)

	waitForEnter("\nPress Enter to exit...")
}
