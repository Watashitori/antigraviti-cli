package injection

import (
	"antigravity-cli/internal/utils"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type AuthStatus struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	ApiKey string `json:"apiKey"`
}

type Account struct {
	Name        string
	Email       string
	AccessToken string
}

// SyncFromIDE retrieves the account info from the IDE database.
func SyncFromIDE() (*Account, error) {
	dbPath := utils.GetAntigravityDBPath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	var valueBase64 string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "jetskiStateSync.agentManagerInitState").Scan(&valueBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to query token: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(valueBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	accessToken, _, err := ExtractOAuthTokenInfo(data)
	if err != nil {
		return nil, fmt.Errorf("failed to extract token info: %w", err)
	}

	// We might not be able to get Name/Email from the protobuf since we only extract token info.
	// But we can try to find them in "antigravityAuthStatus" or just return stub/partial.
	var authStatusJSON string
	var name, email string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "antigravityAuthStatus").Scan(&authStatusJSON)
	if err == nil {
		var auth AuthStatus
		if err := json.Unmarshal([]byte(authStatusJSON), &auth); err == nil {
			name = auth.Name
			email = auth.Email
		}
	}

	if email == "" {
		email = "unknown@example.com"
	}

	return &Account{
		Name:        name,
		Email:       email,
		AccessToken: accessToken,
	}, nil
}

// InjectIdentity injects the access and refresh tokens into the antigravity database.
// It performs aggressive cleanup to prevent account linking.
func InjectIdentity(dbPath, accessToken, refreshToken, email, name string) error {
	// 1. Connect to SQLite
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// 2. AGGRESSIVE CLEANUP: Delete google.antigravity cache first
	// This key often contains cached session data that could link accounts
	_, _ = db.Exec("DELETE FROM ItemTable WHERE key = ?", "google.antigravity")

	// 3. Set default values for email/name if empty
	if email == "" {
		email = "user@antigravity.dev"
	}
	if name == "" {
		name = "Antigravity User"
	}

	// 4. Update antigravityAuthStatus with actual credentials
	authStatus := AuthStatus{
		Name:   name,
		Email:  email,
		ApiKey: accessToken,
	}
	authBytes, err := json.Marshal(authStatus)
	if err != nil {
		return fmt.Errorf("failed to marshal auth status: %w", err)
	}

	_, err = db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)", "antigravityAuthStatus", string(authBytes))
	if err != nil {
		return fmt.Errorf("failed to insert antigravityAuthStatus: %w", err)
	}

	// 5. Force antigravityOnboarding to true to skip welcome dialogs
	_, err = db.Exec("INSERT OR REPLACE INTO ItemTable (key, value) VALUES (?, ?)", "antigravityOnboarding", "true")
	if err != nil {
		return fmt.Errorf("failed to insert antigravityOnboarding: %w", err)
	}

	// 6. Find and update protobuf record
	var valueBase64 string
	err = db.QueryRow("SELECT value FROM ItemTable WHERE key = ?", "jetskiStateSync.agentManagerInitState").Scan(&valueBase64)

	if err == sql.ErrNoRows {
		// Record doesn't exist - that's okay, we've already set auth status above
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to query ItemTable: %w", err)
	}

	// 7. Decode and replace field 6 in protobuf
	data, err := base64.StdEncoding.DecodeString(valueBase64)
	if err != nil {
		return fmt.Errorf("failed to decode base64 value: %w", err)
	}

	newData := replaceProtobufField6(data, accessToken, refreshToken)
	newBase64 := base64.StdEncoding.EncodeToString(newData)

	_, err = db.Exec("UPDATE ItemTable SET value = ? WHERE key = ?", newBase64, "jetskiStateSync.agentManagerInitState")
	if err != nil {
		return fmt.Errorf("failed to update ItemTable: %w", err)
	}

	return nil
}

func replaceProtobufField6(data []byte, accessToken, refreshToken string) []byte {
	cleanData, err := RemoveField(data, 6)
	if err != nil {
		fmt.Printf("Warning: RemoveField error: %v\n", err)
	}

	expiry := time.Now().Add(24 * time.Hour).Unix()
	newField := CreateOAuthTokenInfo(accessToken, refreshToken, expiry)

	return append(cleanData, newField...)
}
