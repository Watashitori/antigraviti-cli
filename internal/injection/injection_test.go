package injection

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func createTempDB(t *testing.T) string {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to create temp db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	return dbPath
}

func TestInjectIdentity_MissingRecord(t *testing.T) {
	dbPath := createTempDB(t)
	
	err := InjectIdentity(dbPath, "test_access", "test_refresh", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("InjectIdentity failed: %v", err)
	}

	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()

	// Check minimal state
	var val string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "antigravityAuthStatus").Scan(&val)
	if err != nil {
		t.Errorf("antigravityAuthStatus not found")
	} else {
		var status AuthStatus
		if err := json.Unmarshal([]byte(val), &status); err != nil {
			t.Errorf("Invalid JSON in auth status: %v", err)
		}
		if status.ApiKey != "test_access" {
			t.Errorf("Expected ApiKey=%s, got %s", "test_access", status.ApiKey)
		}
	}

	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "antigravityOnboarding").Scan(&val)
	if err != nil || val != "true" {
		t.Errorf("antigravityOnboarding missing or invalid")
	}
}

func TestInjectIdentity_ExistingRecord(t *testing.T) {
	dbPath := createTempDB(t)
	db, _ := sql.Open("sqlite", dbPath)
	defer db.Close()

	// Setup initial state: Field 6 with old token, and dummy "google.antigravity"
	// Create a dummy protobuf: Field 5 (string) = "some_data", Field 6 (complex)
	// We'll just create field 6 using helper for convenience
	field6 := CreateOAuthTokenInfo("old_access", "old_refresh", time.Now().Unix())
	initialData := append([]byte{}, field6...)
	initialB64 := base64.StdEncoding.EncodeToString(initialData)

	_, err := db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", "jetskiStateSync.agentManagerInitState", initialB64)
	if err != nil {
		t.Fatalf("Failed to insert initial state: %v", err)
	}
	_, err = db.Exec("INSERT INTO ItemTable (key, value) VALUES (?, ?)", "google.antigravity", "some_cache")
	if err != nil {
		t.Fatalf("Failed to insert cache: %v", err)
	}

	// Run Injection
	err = InjectIdentity(dbPath, "new_access", "new_refresh", "new@example.com", "New User")
	if err != nil {
		t.Fatalf("InjectIdentity failed: %v", err)
	}

	// Verify google.antigravity is gone
	var val string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "google.antigravity").Scan(&val)
	if err != sql.ErrNoRows {
		t.Errorf("google.antigravity should be deleted, found: %v", val)
	}

	// Verify token update in jetskiStateSync.agentManagerInitState
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "jetskiStateSync.agentManagerInitState").Scan(&val)
	if err != nil {
		t.Fatalf("Main record missing")
	}
	
	decoded, _ := base64.StdEncoding.DecodeString(val)
	access, refresh, err := ExtractOAuthTokenInfo(decoded)
	if err != nil {
		t.Fatalf("Failed to extract token info: %v", err)
	}
	
	if access != "new_access" {
		t.Errorf("Expected access token 'new_access', got '%s'", access)
	}
	if refresh != "new_refresh" {
		t.Errorf("Expected refresh token 'new_refresh', got '%s'", refresh)
	}
}
