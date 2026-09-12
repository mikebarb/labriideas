package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Session record for any authenticated identity (token or session)
type Identity struct {
	UserID    string
	ExpiresAt time.Time
}

// Global stores
var (
	sessionStore sync.Map          // tokenHash -> Identity
	passwordMap  map[string]string // passwordHash -> UserID
	staticTokens map[string]string // tokenHash -> UserID (from .env)
)

// InitAuth loads passwords and static tokens into memory.
func InitAuth() {
	passwordMap = make(map[string]string)
	staticTokens = make(map[string]string)

	// Load Passwords
	for _, entry := range strings.Split(os.Getenv("ADMIN_PASSWORDS"), ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) == 2 {
			hash := sha256.Sum256([]byte(parts[1]))
			passwordMap[string(hash[:])] = parts[0]
		}
	}

	// Load Static Tokens
	for _, entry := range strings.Split(os.Getenv("ADMIN_API_TOKENS"), ",") {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) == 2 {
			hash := sha256.Sum256([]byte(parts[1]))
			staticTokens[string(hash[:])] = parts[0]
		}
	}
}

// authorize identifies the requestor.
func authorize(r *http.Request) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(authHeader[7:])
	hash := sha256.Sum256([]byte(token))
	hashKey := string(hash[:])

	// 1. Check Static Tokens (Permanent)
	if userID, ok := staticTokens[hashKey]; ok {
		return userID, true
	}

	// 2. Check Dynamic Sessions (Volatile)
	if val, ok := sessionStore.Load(hashKey); ok {
		sess := val.(Identity)
		if time.Now().Before(sess.ExpiresAt) {
			return sess.UserID, true
		}
		sessionStore.Delete(hashKey)
	}
	return "", false
}

// authMiddleware guards admin routes.
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := authorize(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		// Pass UserID to next handler via context if needed:
		// ctx := context.WithValue(r.Context(), "userID", userID)
		// next(w, r.WithContext(ctx))
		next(w, r)
	}
}

// loginHandler authenticates password and returns a dynamic token.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	hash := sha256.Sum256([]byte(req.Password))
	userID, ok := passwordMap[string(hash[:])]
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	token, _ := newSessionToken()
	tokenHash := sha256.Sum256([]byte(token))

	sessionStore.Store(string(tokenHash[:]), Identity{
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), nil
}
