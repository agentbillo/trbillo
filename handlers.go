package main

import (
	"context"
	crypto_rand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Email validation regex - stricter validation:
// - No consecutive dots
// - Must start and end with alphanumeric
// - Valid domain with at least one dot
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._%+\-]*[a-zA-Z0-9])?@[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)+$`)

// Hex color validation regex (e.g., #RGB, #RRGGBB)
var hexColorRegex = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// UUID validation regex
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Rate limiter for auth endpoints
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int           // max requests
	window   time.Duration // time window
}

var authRateLimiter = &rateLimiter{
	requests: make(map[string][]time.Time),
	limit:    10,              // 10 requests
	window:   1 * time.Minute, // per minute
}

func (rl *rateLimiter) isAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// Filter out old requests
	var recent []time.Time
	for _, t := range rl.requests[ip] {
		if t.After(windowStart) {
			recent = append(recent, t)
		}
	}

	if len(recent) >= rl.limit {
		rl.requests[ip] = recent
		return false
	}

	rl.requests[ip] = append(recent, now)
	return true
}

func getClientIP(r *http.Request) string {
	// Get the direct connection IP first
	ip := r.RemoteAddr
	// Strip port if present
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}

	// Only trust proxy headers if request comes from localhost (trusted reverse proxy)
	// This prevents attackers from spoofing their IP via X-Forwarded-For
	if ip == "127.0.0.1" || ip == "::1" || ip == "localhost" {
		// Check X-Forwarded-For header (for reverse proxies)
		xff := r.Header.Get("X-Forwarded-For")
		if xff != "" {
			// Take the first IP in the list (original client)
			if commaIdx := strings.Index(xff, ","); commaIdx != -1 {
				return strings.TrimSpace(xff[:commaIdx])
			}
			return strings.TrimSpace(xff)
		}
		// Check X-Real-IP header
		xri := r.Header.Get("X-Real-IP")
		if xri != "" {
			return xri
		}
	}

	return ip
}

// Middleware: Rate limiting for auth endpoints
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		if !authRateLimiter.isAllowed(ip) {
			writeError(w, http.StatusTooManyRequests, "Too many requests. Please try again later.")
			return
		}
		next.ServeHTTP(w, r)
	}
}

// ContextKey represents context key type
type ContextKey string

const UserIDKey ContextKey = "userID"

// Helper: JSON writing
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// Helper: Error writing
func writeError(w http.ResponseWriter, status int, errMsg string) {
	writeJSON(w, status, map[string]string{"error": errMsg})
}

// Helper: Check if request is from localhost (for secure cookie decisions)
func isLocalhost(r *http.Request) bool {
	host := r.Host
	// Strip port if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Helper: Set session cookie with appropriate security settings
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Expires:  expires,
		Path:     "/",
		HttpOnly: true,
		Secure:   !isLocalhost(r), // Secure=true for non-localhost (HTTPS)
		SameSite: http.SameSiteStrictMode,
	})
}

// Helper: Clear session cookie
func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour),
		Path:     "/",
		HttpOnly: true,
		Secure:   !isLocalhost(r),
		SameSite: http.SameSiteStrictMode,
	})
}

// Middleware: CSRF Protection
// Validates Origin/Referer header for state-changing requests.
// Combined with SameSite=Strict cookies for defense-in-depth.
func csrfMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only check state-changing methods
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" || r.Method == "DELETE" {
			// Skip CSRF check for localhost
			if isLocalhost(r) {
				next.ServeHTTP(w, r)
				return
			}

			// Check Origin header first, then Referer as fallback
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")

			if origin == "" && referer == "" {
				writeError(w, http.StatusForbidden, "CSRF validation failed: missing origin")
				return
			}

			// Validate that origin/referer matches the host
			checkValue := origin
			if checkValue == "" {
				checkValue = referer
			}

			// Extract host from origin/referer
			checkHost := checkValue
			if strings.HasPrefix(checkHost, "https://") {
				checkHost = checkHost[8:]
			} else if strings.HasPrefix(checkHost, "http://") {
				checkHost = checkHost[7:]
			}
			// Remove path from referer if present
			if slashIdx := strings.Index(checkHost, "/"); slashIdx != -1 {
				checkHost = checkHost[:slashIdx]
			}
			// Strip port for comparison
			if colonIdx := strings.LastIndex(checkHost, ":"); colonIdx != -1 {
				checkHost = checkHost[:colonIdx]
			}

			requestHost := r.Host
			if colonIdx := strings.LastIndex(requestHost, ":"); colonIdx != -1 {
				requestHost = requestHost[:colonIdx]
			}

			if checkHost != requestHost {
				writeError(w, http.StatusForbidden, "CSRF validation failed: origin mismatch")
				return
			}
		}

		next.ServeHTTP(w, r)
	}
}

// Middleware: Authenticate Session
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Unauthorized: No session cookie")
			return
		}

		session, err := GetSession(cookie.Value)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "Unauthorized: Invalid session")
				return
			}
			writeError(w, http.StatusInternalServerError, "Internal Server Error")
			return
		}

		if time.Now().After(session.ExpiresAt) {
			_ = DeleteSession(cookie.Value)
			writeError(w, http.StatusUnauthorized, "Unauthorized: Session expired")
			return
		}

		// Inject userID into context
		ctx := context.WithValue(r.Context(), UserIDKey, session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Helper to get UserID from context
func getUserID(r *http.Request) string {
	val := r.Context().Value(UserIDKey)
	if val == nil {
		return ""
	}
	userID, ok := val.(string)
	if !ok {
		return ""
	}
	return userID
}

// --- INPUT VALIDATION CONSTANTS ---
const (
	MaxUsernameLen    = 50
	MaxEmailLen       = 255
	MaxPasswordLen    = 128
	MinPasswordLen    = 6
	MaxBoardNameLen   = 100
	MaxBoardDescLen   = 1000
	MaxListNameLen    = 100
	MaxTaskTitleLen   = 255
	MaxTaskDescLen    = 5000
	MaxCommentLen     = 5000
	MaxLabelNameLen   = 50
	MaxChecklistLen   = 255
)

// --- AUTH HANDLERS ---

type RegisterReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req RegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Username == "" || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "Username, email, and password are required")
		return
	}

	// Input length validation
	if len(req.Username) > MaxUsernameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Username must be %d characters or less", MaxUsernameLen))
		return
	}
	if len(req.Email) > MaxEmailLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Email must be %d characters or less", MaxEmailLen))
		return
	}
	if len(req.Password) < MinPasswordLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Password must be at least %d characters", MinPasswordLen))
		return
	}
	if len(req.Password) > MaxPasswordLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Password must be %d characters or less", MaxPasswordLen))
		return
	}

	// Email format validation
	if !emailRegex.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "Invalid email format")
		return
	}

	// Password hashing
	pwHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	// Pick a random aesthetic avatar background color
	avatarColors := []string{
		"#6366f1", // Indigo
		"#3b82f6", // Blue
		"#10b981", // Emerald
		"#f59e0b", // Amber
		"#ef4444", // Red
		"#ec4899", // Pink
		"#8b5cf6", // Violet
		"#06b6d4", // Cyan
	}
	// Pick random avatar color using crypto/rand
	colorIndexBytes := make([]byte, 1)
	crypto_rand.Read(colorIndexBytes)
	avatarColor := avatarColors[int(colorIndexBytes[0])%len(avatarColors)]

	u, err := CreateUser(req.Username, req.Email, string(pwHash), avatarColor)
	if err != nil {
		writeError(w, http.StatusConflict, "Username or Email already exists")
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

type LoginReq struct {
	UsernameOrEmail string `json:"username_or_email"`
	Password        string `json:"password"`
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req LoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	u, err := GetUserByUsernameOrEmail(req.UsernameOrEmail)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Create Session with cryptographically secure token
	tokenBytes := make([]byte, 32)
	if _, err := crypto_rand.Read(tokenBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate session token")
		return
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(24 * 7 * time.Hour) // 1 week duration

	if err := CreateSession(token, u.ID, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	// Set Cookie
	setSessionCookie(w, r, token, expiresAt)

	writeJSON(w, http.StatusOK, u)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		if deleteErr := DeleteSession(cookie.Value); deleteErr != nil {
			// Log error but continue - user should still be logged out client-side
			log.Printf("Warning: failed to delete session from database: %v", deleteErr)
		}
	}

	// Clear Cookie
	clearSessionCookie(w, r)

	writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

func MeHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	u, err := GetUserByID(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// --- BOARD HANDLERS ---

type BoardReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func CreateBoardHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var req BoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Board name is required")
		return
	}

	// Input length validation
	if len(req.Name) > MaxBoardNameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Board name must be %d characters or less", MaxBoardNameLen))
		return
	}
	if len(req.Description) > MaxBoardDescLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Description must be %d characters or less", MaxBoardDescLen))
		return
	}

	b, err := CreateBoard(req.Name, req.Description, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create board")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(b.ID, userID, u.Username, "create_board", fmt.Sprintf("created board %q", b.Name))
	}

	writeJSON(w, http.StatusCreated, b)
}

func ListBoardsHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	boards, err := ListUserBoards(userID)
	if err != nil {
		log.Printf("ListUserBoards error: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to fetch boards")
		return
	}
	writeJSON(w, http.StatusOK, boards)
}

func GetBoardHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	// Check membership
	isMember, err := IsBoardMember(boardID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify board membership")
		return
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "Access denied: You are not a member of this board")
		return
	}

	board, err := GetBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	// Populate Members (ensure empty slice instead of nil for JSON)
	members, err := GetBoardMembers(boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch members")
		return
	}
	if members == nil {
		members = []*User{}
	}
	board.Members = members

	// Populate Lists
	lists, err := GetListsByBoard(boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch lists")
		return
	}
	if lists == nil {
		lists = []*List{}
	}

	// Fetch all tasks, assignees, labels, and checklists in batch to assemble the board
	for _, l := range lists {
		tasks, err := GetTasksByList(l.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to fetch tasks")
			return
		}
		if tasks == nil {
			tasks = []*Task{}
		}
		for _, t := range tasks {
			// Assignees (ensure empty slice instead of nil)
			assignees, err := GetTaskAssignees(t.ID)
			if err == nil && assignees != nil {
				t.Assignees = assignees
			} else {
				t.Assignees = []*User{}
			}
			// Labels (ensure empty slice instead of nil)
			labels, err := GetTaskLabels(t.ID)
			if err == nil && labels != nil {
				t.Labels = labels
			} else {
				t.Labels = []*Label{}
			}
			// Checklist (ensure empty slice instead of nil)
			checklist, err := GetChecklistItems(t.ID)
			if err == nil && checklist != nil {
				t.Checklist = checklist
			} else {
				t.Checklist = []*ChecklistItem{}
			}
		}
		l.Tasks = tasks
	}
	board.Lists = lists

	writeJSON(w, http.StatusOK, board)
}

type UpdateBoardReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Theme       string `json:"theme"`
	Icon        string `json:"icon"`
}

func UpdateBoardHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	// Get current board to check ownership
	currentBoard, err := GetBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	// Only board owner can update settings
	if currentBoard.OwnerID != userID {
		writeError(w, http.StatusForbidden, "Only board owner can update settings")
		return
	}

	var req UpdateBoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Validate lengths
	if len(req.Name) > MaxBoardNameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Board name exceeds maximum length of %d characters", MaxBoardNameLen))
		return
	}
	if len(req.Description) > MaxBoardDescLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Board description exceeds maximum length of %d characters", MaxBoardDescLen))
		return
	}

	// Validate theme
	validThemes := map[string]bool{"dark": true, "light": true, "autumn": true, "spring": true}
	if req.Theme == "" {
		req.Theme = "dark"
	}
	if !validThemes[req.Theme] {
		writeError(w, http.StatusBadRequest, "Invalid theme")
		return
	}

	// Use existing icon if not provided
	if req.Icon == "" {
		req.Icon = currentBoard.Icon
	}

	updatedBoard, err := UpdateBoard(boardID, req.Name, req.Description, req.Theme, req.Icon)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update board")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(boardID, userID, u.Username, "update_board", "updated board settings")
	}

	// Broadcast WS update
	broadcastBoardUpdate(boardID, "board_updated", map[string]interface{}{
		"board": updatedBoard,
	})

	writeJSON(w, http.StatusOK, updatedBoard)
}

func DeleteBoardHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	board, err := GetBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	if board.OwnerID != userID {
		writeError(w, http.StatusForbidden, "Only the board owner can delete the board")
		return
	}

	if err := DeleteBoard(boardID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete board")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Board deleted successfully"})
}

type InviteMemberReq struct {
	UsernameOrEmail string `json:"username_or_email"`
}

func AddBoardMemberHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	// Only board owner can invite members
	board, err := GetBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}
	if board.OwnerID != userID {
		writeError(w, http.StatusForbidden, "Only board owner can invite members")
		return
	}

	var req InviteMemberReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Find the user to invite
	invitee, err := GetUserByUsernameOrEmail(req.UsernameOrEmail)
	if err != nil {
		writeError(w, http.StatusNotFound, "User not found")
		return
	}

	if err := AddBoardMember(boardID, invitee.ID, "member"); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to add member")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(boardID, userID, u.Username, "add_member", fmt.Sprintf("added %s to the board", invitee.Username))
	}

	// Broadcast WS update to board viewers
	broadcastBoardUpdate(boardID, "member_added", map[string]interface{}{
		"user": invitee,
	})

	// Also broadcast to the invited user's personal channel so board appears in their sidebar
	// Refresh board data to include new member
	board, _ = GetBoard(boardID)
	userPayload, _ := json.Marshal(map[string]interface{}{
		"type":  "added_to_board",
		"board": board,
	})
	HubInstance.BroadcastToUser(invitee.ID, userPayload)

	writeJSON(w, http.StatusOK, invitee)
}

func RemoveBoardMemberHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)
	targetUserID := r.PathValue("user_id")

	// Check if current user is board member
	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Get board to check ownership
	board, err := GetBoard(boardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}

	// Permission checks:
	// - Owner cannot leave (must delete board instead)
	// - Only owner can remove others
	// - Anyone can remove themselves (leave)
	isSelfRemoval := userID == targetUserID
	isOwner := board.OwnerID == userID

	if isSelfRemoval && isOwner {
		writeError(w, http.StatusBadRequest, "Board owner cannot leave. Delete the board instead.")
		return
	}

	if !isSelfRemoval && !isOwner {
		writeError(w, http.StatusForbidden, "Only the board owner can remove other members")
		return
	}

	if err := RemoveBoardMember(boardID, targetUserID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	targetUser, _ := GetUserByID(targetUserID)
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(boardID, userID, u.Username, "remove_member", fmt.Sprintf("removed %s from the board", targetUser.Username))
	}

	broadcastBoardUpdate(boardID, "member_removed", map[string]interface{}{
		"user_id": targetUserID,
	})

	// Also broadcast to the removed user's personal channel so they see the removal
	// even if they're not viewing this board
	userPayload, _ := json.Marshal(map[string]interface{}{
		"type":     "removed_from_board",
		"board_id": boardID,
	})
	HubInstance.BroadcastToUser(targetUserID, userPayload)

	writeJSON(w, http.StatusOK, map[string]string{"message": "Member removed successfully"})
}

// GetCollaboratorsHandler returns users the current user has collaborated with on other boards
func GetCollaboratorsHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	// Check if current user is board member
	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	collaborators, err := GetCollaborators(userID, boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to get collaborators")
		return
	}

	writeJSON(w, http.StatusOK, collaborators)
}

type CopyBoardReq struct {
	Name           string `json:"name"`
	IncludeMembers bool   `json:"include_members"`
}

func CopyBoardHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	// Check if current user is board member
	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req CopyBoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Board name is required")
		return
	}

	// Copy the board
	newBoard, err := CopyBoard(boardID, req.Name, userID, req.IncludeMembers)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to copy board")
		return
	}

	// Log activity on the source board
	sourceBoard, _ := GetBoard(boardID)
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(boardID, userID, u.Username, "copy_board", fmt.Sprintf("copied board to %q", req.Name))
		// Log activity on the new board
		_, _ = LogActivity(newBoard.ID, userID, u.Username, "create_board", fmt.Sprintf("created board %q (copied from %q)", newBoard.Name, sourceBoard.Name))
	}

	// If members were included, notify them about being added to the new board
	if req.IncludeMembers {
		members, _ := GetBoardMembers(newBoard.ID)
		for _, member := range members {
			if member.ID != userID {
				userPayload, _ := json.Marshal(map[string]interface{}{
					"type":  "added_to_board",
					"board": newBoard,
				})
				HubInstance.BroadcastToUser(member.ID, userPayload)
			}
		}
	}

	writeJSON(w, http.StatusCreated, newBoard)
}

// --- LIST HANDLERS ---

type ListReq struct {
	Name     string `json:"name"`
	Position int    `json:"position"`
}

func CreateListHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("board_id")
	userID := getUserID(r)

	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "List name is required")
		return
	}
	if len(req.Name) > MaxListNameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("List name exceeds maximum length of %d characters", MaxListNameLen))
		return
	}

	l, err := CreateList(boardID, req.Name, req.Position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create list")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(boardID, userID, u.Username, "create_list", fmt.Sprintf("created list %q", l.Name))
	}

	// Broadcast WS
	broadcastBoardUpdate(boardID, "list_created", l)

	writeJSON(w, http.StatusCreated, l)
}

func UpdateListHandler(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	userID := getUserID(r)

	list, err := GetList(listID)
	if err != nil {
		writeError(w, http.StatusNotFound, "List not found")
		return
	}

	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ListReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name == "" {
		req.Name = list.Name
	}

	if err := UpdateList(listID, req.Name, req.Position); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update list")
		return
	}

	list.Name = req.Name
	list.Position = req.Position

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "list_updated", list)

	writeJSON(w, http.StatusOK, list)
}

func DeleteListHandler(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	userID := getUserID(r)

	list, err := GetList(listID)
	if err != nil {
		writeError(w, http.StatusNotFound, "List not found")
		return
	}

	// Only board owner can delete lists
	board, err := GetBoard(list.BoardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Board not found")
		return
	}
	if board.OwnerID != userID {
		writeError(w, http.StatusForbidden, "Only board owner can delete lists")
		return
	}

	if err := DeleteList(listID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete list")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "delete_list", fmt.Sprintf("deleted list %q", list.Name))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "list_deleted", map[string]string{"list_id": listID})

	writeJSON(w, http.StatusOK, map[string]string{"message": "List deleted successfully"})
}

// --- TASK HANDLERS ---

type TaskReq struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	ListID      string     `json:"list_id"`
	Position    int        `json:"position"`
	DueDate     *time.Time `json:"due_date"`
}

func CreateTaskHandler(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("list_id")
	userID := getUserID(r)

	list, err := GetList(listID)
	if err != nil {
		writeError(w, http.StatusNotFound, "List not found")
		return
	}

	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req TaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "Task title is required")
		return
	}
	if len(req.Title) > MaxTaskTitleLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Task title exceeds maximum length of %d characters", MaxTaskTitleLen))
		return
	}

	t, err := CreateTask(listID, req.Title, req.Position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create task")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "create_task", fmt.Sprintf("added card %q to %s", t.Title, list.Name))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "card_created", t)

	writeJSON(w, http.StatusCreated, t)
}

func GetTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error fetching list details")
		return
	}

	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	// Fetch assignees, labels, and checklist items
	t.Assignees, _ = GetTaskAssignees(taskID)
	t.Labels, _ = GetTaskLabels(taskID)
	t.Checklist, _ = GetChecklistItems(taskID)

	writeJSON(w, http.StatusOK, t)
}

func UpdateTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	oldList, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error validating task's board")
		return
	}

	isMember, err := IsBoardMember(oldList.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req TaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Handle partial updates
	if req.Title == "" {
		req.Title = t.Title
	}
	if r.Header.Get("Content-Type") != "application/json" { // fallback
		req.Description = t.Description
	}
	if req.ListID == "" {
		req.ListID = t.ListID
	}

	// Validate lengths
	if len(req.Title) > MaxTaskTitleLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Task title exceeds maximum length of %d characters", MaxTaskTitleLen))
		return
	}
	if len(req.Description) > MaxTaskDescLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Task description exceeds maximum length of %d characters", MaxTaskDescLen))
		return
	}

	// Check if moving to another list in the same board
	if req.ListID != t.ListID {
		newList, err := GetList(req.ListID)
		if err != nil || newList.BoardID != oldList.BoardID {
			writeError(w, http.StatusBadRequest, "Cannot move task to a list on a different board")
			return
		}
	}

	// Perform update
	if err := UpdateTask(taskID, req.Title, req.Description, req.ListID, req.Position, req.DueDate); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update task")
		return
	}

	tUpdated, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to retrieve updated task")
		return
	}

	// Log activity for moves or details edit
	u, _ := GetUserByID(userID)
	if req.ListID != t.ListID {
		newList, _ := GetList(req.ListID)
		_, _ = LogActivity(oldList.BoardID, userID, u.Username, "move_task",
			fmt.Sprintf("moved card %q from %s to %s", t.Title, oldList.Name, newList.Name))
	} else if req.Title != t.Title || req.Description != t.Description {
		_, _ = LogActivity(oldList.BoardID, userID, u.Username, "edit_task",
			fmt.Sprintf("updated card details for %q", tUpdated.Title))
	}

	// Populate assignees and labels for the broadcast
	tUpdated.Assignees, _ = GetTaskAssignees(taskID)
	tUpdated.Labels, _ = GetTaskLabels(taskID)
	tUpdated.Checklist, _ = GetChecklistItems(taskID)

	// Broadcast WS
	broadcastBoardUpdate(oldList.BoardID, "card_updated", map[string]interface{}{
		"task":        tUpdated,
		"old_list_id": t.ListID,
	})

	writeJSON(w, http.StatusOK, tUpdated)
}

func DeleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error verifying board details")
		return
	}

	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := DeleteTask(taskID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete task")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "delete_task", fmt.Sprintf("deleted card %q", t.Title))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "card_deleted", map[string]string{
		"task_id": t.ID,
		"list_id": t.ListID,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "Task deleted successfully"})
}

// --- TASK ASSIGNEES ---

type AssigneeReq struct {
	UserID string `json:"user_id"`
}

func AssignTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req AssigneeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	// Verify assignee is member of board
	isAssigneeMember, err := IsBoardMember(list.BoardID, req.UserID)
	if err != nil || !isAssigneeMember {
		writeError(w, http.StatusBadRequest, "Assignee must be a member of the board")
		return
	}

	if err := AssignUserToTask(taskID, req.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to assign user")
		return
	}

	assigneeUser, _ := GetUserByID(req.UserID)
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "assign_task", fmt.Sprintf("assigned %s to card %q", assigneeUser.Username, t.Title))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "card_assignees_updated", map[string]interface{}{
		"task_id": taskID,
		"user":    assigneeUser,
		"action":  "assigned",
	})

	writeJSON(w, http.StatusOK, assigneeUser)
}

func UnassignTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req AssigneeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := UnassignUserFromTask(taskID, req.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to unassign user")
		return
	}

	unassigneeUser, _ := GetUserByID(req.UserID)
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "unassign_task", fmt.Sprintf("unassigned %s from card %q", unassigneeUser.Username, t.Title))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "card_assignees_updated", map[string]interface{}{
		"task_id": taskID,
		"user_id": req.UserID,
		"action":  "unassigned",
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "User unassigned successfully"})
}

// --- LABELS HANDLERS ---

type LabelCreateReq struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func CreateBoardLabelHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req LabelCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Name == "" || req.Color == "" {
		writeError(w, http.StatusBadRequest, "Label name and color are required")
		return
	}
	if len(req.Name) > MaxLabelNameLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Label name exceeds maximum length of %d characters", MaxLabelNameLen))
		return
	}
	if !hexColorRegex.MatchString(req.Color) {
		writeError(w, http.StatusBadRequest, "Invalid color format. Use hex color (e.g., #FF5733 or #F53)")
		return
	}

	l, err := CreateLabel(boardID, req.Name, req.Color)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create label")
		return
	}

	writeJSON(w, http.StatusCreated, l)
}

func ListBoardLabelsHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	labels, err := GetBoardLabels(boardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch labels")
		return
	}

	writeJSON(w, http.StatusOK, labels)
}

type ToggleLabelReq struct {
	LabelID string `json:"label_id"`
}

func AddTaskLabelHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ToggleLabelReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := AddLabelToTask(taskID, req.LabelID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to add label")
		return
	}

	// Broadcast WS
	labels, _ := GetTaskLabels(taskID)
	broadcastBoardUpdate(list.BoardID, "card_labels_updated", map[string]interface{}{
		"task_id": taskID,
		"labels":  labels,
	})

	writeJSON(w, http.StatusOK, labels)
}

func RemoveTaskLabelHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ToggleLabelReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if err := RemoveLabelFromTask(taskID, req.LabelID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to remove label")
		return
	}

	// Broadcast WS
	labels, _ := GetTaskLabels(taskID)
	broadcastBoardUpdate(list.BoardID, "card_labels_updated", map[string]interface{}{
		"task_id": taskID,
		"labels":  labels,
	})

	writeJSON(w, http.StatusOK, labels)
}

// --- CHECKLIST HANDLERS ---

type ChecklistCreateReq struct {
	Title    string `json:"title"`
	Position int    `json:"position"`
}

func CreateChecklistItemHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ChecklistCreateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "Checklist item title is required")
		return
	}
	if len(req.Title) > MaxChecklistLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Checklist item title exceeds maximum length of %d characters", MaxChecklistLen))
		return
	}

	item, err := CreateChecklistItem(taskID, req.Title, req.Position)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create checklist item")
		return
	}

	// Broadcast WS
	checklist, _ := GetChecklistItems(taskID)
	broadcastBoardUpdate(list.BoardID, "card_checklist_updated", map[string]interface{}{
		"task_id":   taskID,
		"checklist": checklist,
	})

	writeJSON(w, http.StatusCreated, item)
}

type ChecklistUpdateReq struct {
	Title       string `json:"title"`
	IsCompleted bool   `json:"is_completed"`
}

func UpdateChecklistItemHandler(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("id")
	userID := getUserID(r)

	// Since we only have item ID, let's find it in DB. For safety, we would query the parent task/list.
	var taskID string
	var currentTitle string
	query := `SELECT task_id, title FROM checklist_items WHERE id = ?`
	err := DB.QueryRow(query, itemID).Scan(&taskID, &currentTitle)
	if err != nil {
		writeError(w, http.StatusNotFound, "Checklist item not found")
		return
	}

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Parent task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req ChecklistUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Title == "" {
		req.Title = currentTitle
	}

	if err := UpdateChecklistItem(itemID, req.Title, req.IsCompleted); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update checklist item")
		return
	}

	// Broadcast WS
	checklist, _ := GetChecklistItems(taskID)
	broadcastBoardUpdate(list.BoardID, "card_checklist_updated", map[string]interface{}{
		"task_id":   taskID,
		"checklist": checklist,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "Checklist item updated"})
}

func DeleteChecklistItemHandler(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("id")
	userID := getUserID(r)

	var taskID string
	query := `SELECT task_id FROM checklist_items WHERE id = ?`
	err := DB.QueryRow(query, itemID).Scan(&taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Checklist item not found")
		return
	}

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Parent task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := DeleteChecklistItem(itemID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete checklist item")
		return
	}

	// Broadcast WS
	checklist, _ := GetChecklistItems(taskID)
	broadcastBoardUpdate(list.BoardID, "card_checklist_updated", map[string]interface{}{
		"task_id":   taskID,
		"checklist": checklist,
	})

	writeJSON(w, http.StatusOK, map[string]string{"message": "Checklist item deleted"})
}

// --- COMMENTS HANDLERS ---

type CommentReq struct {
	Content string `json:"content"`
}

func CreateCommentHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	var req CommentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "Comment content is required")
		return
	}
	if len(req.Content) > MaxCommentLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Comment exceeds maximum length of %d characters", MaxCommentLen))
		return
	}

	comment, err := CreateComment(taskID, userID, req.Content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create comment")
		return
	}

	// Log activity
	u, err := GetUserByID(userID)
	if err == nil {
		_, _ = LogActivity(list.BoardID, userID, u.Username, "comment", fmt.Sprintf("commented on card %q: %q", t.Title, comment.Content))
	}

	// Broadcast WS
	broadcastBoardUpdate(list.BoardID, "comment_added", map[string]interface{}{
		"task_id": taskID,
		"comment": comment,
	})

	writeJSON(w, http.StatusCreated, comment)
}

func ListTaskCommentsHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	userID := getUserID(r)

	t, err := GetTask(taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	list, err := GetList(t.ListID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch list details")
		return
	}
	isMember, err := IsBoardMember(list.BoardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	comments, err := GetTaskComments(taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch comments")
		return
	}

	writeJSON(w, http.StatusOK, comments)
}

// --- ACTIVITIES & WS HANDLERS ---

func GetBoardActivitiesHandler(w http.ResponseWriter, r *http.Request) {
	boardID := r.PathValue("id")
	userID := getUserID(r)

	isMember, err := IsBoardMember(boardID, userID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	limit := 50
	const maxActivityLimit = 500
	limitStr := r.URL.Query().Get("limit")
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > maxActivityLimit {
				limit = maxActivityLimit
			}
		}
	}

	activities, err := GetBoardActivities(boardID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to fetch activities")
		return
	}

	writeJSON(w, http.StatusOK, activities)
}

func WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	boardID := r.URL.Query().Get("board_id")
	if boardID == "" {
		// Cannot use standard writeError since it requires standard HTTP.
		// If we haven't upgraded yet, we can write normal HTTP.
		writeError(w, http.StatusBadRequest, "Missing board_id query parameter")
		return
	}
	if !uuidRegex.MatchString(boardID) {
		writeError(w, http.StatusBadRequest, "Invalid board_id format")
		return
	}

	// Verify authentication token from cookie manually since it's a websocket upgrade
	cookie, err := r.Cookie("session_token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	session, err := GetSession(cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "Session expired")
		return
	}

	isMember, err := IsBoardMember(boardID, session.UserID)
	if err != nil || !isMember {
		writeError(w, http.StatusForbidden, "Access denied")
		return
	}

	ServeWS(w, r, session.UserID, boardID)
}

// UserWebSocketHandler handles user-level WebSocket connections (no board required).
func UserWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	// Verify authentication token from cookie
	cookie, err := r.Cookie("session_token")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	session, err := GetSession(cookie.Value)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "Session expired")
		return
	}

	ServeUserWS(w, r, session.UserID)
}

// Helper to broadcast event updates
func broadcastBoardUpdate(boardID, event string, data interface{}) {
	payload, err := json.Marshal(map[string]interface{}{
		"event":    event,
		"board_id": boardID,
		"data":     data,
		"time":     time.Now(),
	})
	if err != nil {
		return
	}
	HubInstance.BroadcastToBoard(boardID, payload)
}
