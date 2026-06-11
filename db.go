package main

import (
	crypto_rand "crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

func InitDB(dataSourceName string) error {
	var err error
	DB, err = sql.Open("sqlite", dataSourceName)
	if err != nil {
		return err
	}

	// Enable foreign keys
	if _, err := DB.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return err
	}

	// Set connection limits
	DB.SetMaxOpenConns(1) // SQLite works best with 1 open connection for writing to avoid locks

	return migrate()
}

func migrate() error {
	schemas := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			avatar_color TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS boards (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			theme TEXT NOT NULL DEFAULT 'dark',
			icon TEXT NOT NULL DEFAULT '📋',
			owner_id TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(owner_id) REFERENCES users(id)
		);`,
		`CREATE TABLE IF NOT EXISTS board_members (
			board_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(board_id, user_id),
			FOREIGN KEY(board_id) REFERENCES boards(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS lists (
			id TEXT PRIMARY KEY,
			board_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(board_id) REFERENCES boards(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			list_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			link TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL,
			due_date TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(list_id) REFERENCES lists(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS checklist_items (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			title TEXT NOT NULL,
			is_completed INTEGER DEFAULT 0,
			position INTEGER NOT NULL,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS labels (
			id TEXT PRIMARY KEY,
			board_id TEXT NOT NULL,
			name TEXT NOT NULL,
			color TEXT NOT NULL,
			FOREIGN KEY(board_id) REFERENCES boards(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS task_labels (
			task_id TEXT NOT NULL,
			label_id TEXT NOT NULL,
			PRIMARY KEY(task_id, label_id),
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(label_id) REFERENCES labels(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS task_assignees (
			task_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			PRIMARY KEY(task_id, user_id),
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS comments (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS activities (
			id TEXT PRIMARY KEY,
			board_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			username TEXT NOT NULL,
			action_type TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(board_id) REFERENCES boards(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS teams (
			name TEXT PRIMARY KEY,
			code TEXT UNIQUE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, schema := range schemas {
		if _, err := DB.Exec(schema); err != nil {
			return fmt.Errorf("migration failure: %w", err)
		}
	}

	// Run optional migrations for adding new columns to existing tables
	optionalMigrations := []string{
		`ALTER TABLE boards ADD COLUMN theme TEXT NOT NULL DEFAULT 'dark'`,
		`ALTER TABLE boards ADD COLUMN icon TEXT NOT NULL DEFAULT '📋'`,
		`ALTER TABLE boards ADD COLUMN updated_at TIMESTAMP`,
		`ALTER TABLE tasks ADD COLUMN link TEXT NOT NULL DEFAULT ''`,
		// Empty team = permanent account with no team (admin, pre-team users)
		`ALTER TABLE users ADD COLUMN team TEXT NOT NULL DEFAULT ''`,
	}

	for _, migration := range optionalMigrations {
		// Log errors but continue (column may already exist, which is expected)
		if _, err := DB.Exec(migration); err != nil {
			// Only log if it's not a "duplicate column" error
			if !strings.Contains(err.Error(), "duplicate column") {
				log.Printf("Migration note: %v (may be expected if column exists)", err)
			}
		}
	}

	// Best-effort conversion from the short-lived invite_codes schema, where
	// the code doubled as the team identity. Errors are expected and ignored
	// on databases that never had it.
	legacyMigrations := []string{
		`INSERT OR IGNORE INTO teams (name, code, created_at) SELECT code, code, created_at FROM invite_codes`,
		`UPDATE users SET team = invite_code WHERE team = '' AND invite_code <> ''`,
		`DROP TABLE IF EXISTS invite_codes`,
	}
	for _, migration := range legacyMigrations {
		_, _ = DB.Exec(migration)
	}

	// The PUBLIC team always exists so open signups work out of the box.
	// OR IGNORE keeps an operator-rotated code (e.g. PUBLIC999) on restart.
	if _, err := DB.Exec(`INSERT OR IGNORE INTO teams (name, code, created_at) VALUES (?, ?, ?)`, PublicTeam, PublicTeam, time.Now()); err != nil {
		return fmt.Errorf("seeding public team: %w", err)
	}

	return nil
}

// --- USER OPERATIONS ---

func CreateUser(username, email, passwordHash, avatarColor, team string) (*User, error) {
	u := &User{
		ID:          uuid.New().String(),
		Username:    username,
		Email:       email,
		AvatarColor: avatarColor,
		Team:        team,
		CreatedAt:   time.Now(),
	}
	query := `INSERT INTO users (id, username, email, password_hash, avatar_color, team, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, u.ID, u.Username, u.Email, passwordHash, u.AvatarColor, u.Team, u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByID(id string) (*User, error) {
	u := &User{}
	query := `SELECT id, username, email, password_hash, avatar_color, team, created_at FROM users WHERE id = ?`
	err := DB.QueryRow(query, id).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.AvatarColor, &u.Team, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func GetUserByUsernameOrEmail(identifier string) (*User, error) {
	u := &User{}
	query := `SELECT id, username, email, password_hash, avatar_color, team, created_at FROM users WHERE username = ? OR email = ?`
	err := DB.QueryRow(query, identifier, identifier).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.AvatarColor, &u.Team, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// AdminUsername is reserved: it cannot be registered, and the matching user
// is the single admin account.
const AdminUsername = "admin"

// PublicTeam is the open-signup team (and its initial code). Accounts on
// this team are trial accounts and are wiped after TrialDuration. The team's
// signup code can be rotated by the admin without affecting its members.
const PublicTeam = "PUBLIC"

// TrialDuration is how long PUBLIC trial accounts live before being wiped.
const TrialDuration = 30 * 24 * time.Hour

// UnusablePasswordHash never matches any bcrypt comparison, locking the
// account until a real hash is set (e.g. via `trbillo -set-admin`).
const UnusablePasswordHash = "*"

// EnsureAdminUser creates the admin user with an unusable password hash if it
// does not exist yet. Returns the user and whether it was just created.
func EnsureAdminUser() (*User, bool, error) {
	u, err := GetUserByUsernameOrEmail(AdminUsername)
	if err == nil {
		return u, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	u, err = CreateUser(AdminUsername, "admin@trbillo.local", UnusablePasswordHash, "#ef4444", "")
	if err != nil {
		return nil, false, err
	}
	return u, true, nil
}

// --- TEAM OPERATIONS ---

// GetTeamNameByCode resolves a signup code to its team name.
func GetTeamNameByCode(code string) (string, error) {
	var name string
	err := DB.QueryRow(`SELECT name FROM teams WHERE code = ?`, code).Scan(&name)
	return name, err
}

// TeamExists reports whether a team with this name exists (case-insensitive,
// to avoid confusable team names).
func TeamExists(name string) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE name = ? COLLATE NOCASE`, name).Scan(&count)
	return count > 0, err
}

// CodeInUse reports whether any team already uses this signup code.
func CodeInUse(code string) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE code = ?`, code).Scan(&count)
	return count > 0, err
}

func CreateTeam(name, code string) error {
	_, err := DB.Exec(`INSERT INTO teams (name, code, created_at) VALUES (?, ?, ?)`, name, code, time.Now())
	return err
}

// UpdateTeamCode rotates a team's signup code. Members are unaffected.
func UpdateTeamCode(name, code string) error {
	res, err := DB.Exec(`UPDATE teams SET code = ? WHERE name = ?`, code, name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("team not found")
	}
	return nil
}

func DeleteTeam(name string) error {
	res, err := DB.Exec(`DELETE FROM teams WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("team not found")
	}
	return nil
}

// ListTeams returns every team with its code and user count, for the admin
// teams view.
func ListTeams() ([]*TeamRow, error) {
	query := `SELECT t.name, t.code, t.created_at,
	                 (SELECT COUNT(*) FROM users u WHERE u.team = t.name)
	          FROM teams t
	          ORDER BY t.name COLLATE NOCASE`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	teams := []*TeamRow{}
	for rows.Next() {
		t := &TeamRow{}
		if err := rows.Scan(&t.Name, &t.Code, &t.CreatedAt, &t.UserCount); err != nil {
			return nil, err
		}
		teams = append(teams, t)
	}
	return teams, nil
}

// UpdateUserTeam moves a user to a different team ('' = no team). created_at
// is left untouched, so moving someone onto PUBLIC starts their trial clock
// from their original signup, and moving them off PUBLIC makes them permanent.
func UpdateUserTeam(userID, team string) error {
	res, err := DB.Exec(`UPDATE users SET team = ? WHERE id = ?`, team, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("user not found")
	}
	return nil
}

// GetUserTeam returns the team a user belongs to ('' = no team).
func GetUserTeam(userID string) (string, error) {
	var team string
	err := DB.QueryRow(`SELECT team FROM users WHERE id = ?`, userID).Scan(&team)
	return team, err
}

// ListTeamUsers returns all users on the given team. Callers must not pass
// the public or empty team — those have no member visibility.
func ListTeamUsers(team string) ([]*User, error) {
	query := `SELECT id, username, email, avatar_color, created_at
	          FROM users WHERE team = ?
	          ORDER BY username COLLATE NOCASE`
	rows, err := DB.Query(query, team)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarColor, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// CleanExpiredTrialUsers wipes PUBLIC-team trial accounts older than
// TrialDuration, including any boards they own. Returns how many users were
// removed.
func CleanExpiredTrialUsers() (int, error) {
	cutoff := time.Now().Add(-TrialDuration)
	rows, err := DB.Query(`SELECT id, username FROM users WHERE team = ? AND created_at < ?`, PublicTeam, cutoff)
	if err != nil {
		return 0, err
	}
	type expired struct{ id, username string }
	var victims []expired
	for rows.Next() {
		var e expired
		if err := rows.Scan(&e.id, &e.username); err != nil {
			rows.Close()
			return 0, err
		}
		victims = append(victims, e)
	}
	rows.Close()

	removed := 0
	for _, v := range victims {
		boardRows, err := DB.Query(`SELECT id FROM boards WHERE owner_id = ?`, v.id)
		if err != nil {
			return removed, err
		}
		var boardIDs []string
		for boardRows.Next() {
			var id string
			if err := boardRows.Scan(&id); err != nil {
				boardRows.Close()
				return removed, err
			}
			boardIDs = append(boardIDs, id)
		}
		boardRows.Close()

		for _, boardID := range boardIDs {
			if err := DeleteBoard(boardID); err != nil {
				return removed, fmt.Errorf("deleting board %s of expired trial user %s: %w", boardID, v.username, err)
			}
		}
		if err := DeleteUser(v.id); err != nil {
			return removed, fmt.Errorf("deleting expired trial user %s: %w", v.username, err)
		}
		log.Printf("Trial cleanup: removed expired PUBLIC account %q (%d boards)", v.username, len(boardIDs))
		removed++
	}
	return removed, nil
}

func UpdateUserPassword(userID, passwordHash string) error {
	res, err := DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("user not found")
	}
	return nil
}

// DeleteUser removes a user; sessions, memberships, assignments, and comments
// cascade. Fails (FK constraint) if the user still owns boards — callers must
// check CountBoardsOwnedBy first to give a friendly error.
func DeleteUser(userID string) error {
	res, err := DB.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("user not found")
	}
	return nil
}

func CountBoardsOwnedBy(userID string) (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM boards WHERE owner_id = ?`, userID).Scan(&count)
	return count, err
}

// ListAllUsers returns every user with board ownership/membership counts,
// for the admin users view.
func ListAllUsers() ([]*AdminUserRow, error) {
	query := `SELECT u.id, u.username, u.email, u.avatar_color, u.team, u.created_at,
	                 (SELECT COUNT(*) FROM boards b WHERE b.owner_id = u.id),
	                 (SELECT COUNT(*) FROM board_members bm WHERE bm.user_id = u.id)
	          FROM users u
	          ORDER BY u.username COLLATE NOCASE`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*AdminUserRow{}
	for rows.Next() {
		u := &AdminUserRow{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarColor, &u.Team, &u.CreatedAt, &u.BoardsOwned, &u.BoardsMemberOf); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// --- SESSION OPERATIONS ---

func CreateSession(token, userID string, expiresAt time.Time) error {
	query := `INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)`
	_, err := DB.Exec(query, token, userID, expiresAt)
	return err
}

func GetSession(token string) (*Session, error) {
	s := &Session{}
	query := `SELECT token, user_id, expires_at FROM sessions WHERE token = ?`
	err := DB.QueryRow(query, token).Scan(&s.Token, &s.UserID, &s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func DeleteSession(token string) error {
	query := `DELETE FROM sessions WHERE token = ?`
	_, err := DB.Exec(query, token)
	return err
}

// DeleteUserSessions logs the user out everywhere by removing all their sessions.
func DeleteUserSessions(userID string) error {
	_, err := DB.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func CleanExpiredSessions() {
	_, _ = DB.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now())
}

// --- BOARD OPERATIONS ---

// Default board icons for random selection
var defaultBoardIcons = []string{"❤️", "🎃", "🎵", "🤖", "🐶", "🐟", "🐢", "⚽️", "🥎", "🚗", "✈️"}

func CreateBoard(name, description, ownerID string) (*Board, error) {
	// Pick random icon using crypto/rand
	iconIndexBytes := make([]byte, 1)
	crypto_rand.Read(iconIndexBytes)
	icon := defaultBoardIcons[int(iconIndexBytes[0])%len(defaultBoardIcons)]

	now := time.Now()
	b := &Board{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Theme:       "dark",
		Icon:        icon,
		OwnerID:     ownerID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	queryBoard := `INSERT INTO boards (id, name, description, theme, icon, owner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(queryBoard, b.ID, b.Name, b.Description, b.Theme, b.Icon, b.OwnerID, b.CreatedAt, b.UpdatedAt); err != nil {
		return nil, err
	}

	queryMember := `INSERT INTO board_members (board_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`
	if _, err := tx.Exec(queryMember, b.ID, ownerID, time.Now()); err != nil {
		return nil, err
	}

	// Create default lists for the board
	defaultLists := []string{"To Do", "In Progress", "Review", "Done"}
	for i, listName := range defaultLists {
		listID := uuid.New().String()
		queryList := `INSERT INTO lists (id, board_id, name, position, created_at) VALUES (?, ?, ?, ?, ?)`
		if _, err := tx.Exec(queryList, listID, b.ID, listName, i, time.Now()); err != nil {
			return nil, err
		}
	}

	// Create default labels for the board
	defaultLabels := []struct {
		name  string
		color string
	}{
		{"High Priority", "#ef4444"},   // Red
		{"Medium Priority", "#f59e0b"}, // Amber
		{"Low Priority", "#3b82f6"},    // Blue
		{"Bug", "#ec4899"},             // Pink
		{"Feature", "#10b981"},         // Emerald
	}
	for _, label := range defaultLabels {
		labelID := uuid.New().String()
		queryLabel := `INSERT INTO labels (id, board_id, name, color) VALUES (?, ?, ?, ?)`
		if _, err := tx.Exec(queryLabel, labelID, b.ID, label.name, label.color); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return b, nil
}

func GetBoard(id string) (*Board, error) {
	b := &Board{}
	var updatedAtStr sql.NullString
	query := `SELECT id, name, description, theme, COALESCE(icon, '📋'), owner_id, created_at, updated_at FROM boards WHERE id = ?`
	err := DB.QueryRow(query, id).Scan(&b.ID, &b.Name, &b.Description, &b.Theme, &b.Icon, &b.OwnerID, &b.CreatedAt, &updatedAtStr)
	if err != nil {
		return nil, err
	}
	// Parse updated_at or fall back to created_at
	if updatedAtStr.Valid && updatedAtStr.String != "" {
		if parsed, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", updatedAtStr.String); err == nil {
			b.UpdatedAt = parsed
		} else if parsed, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
			b.UpdatedAt = parsed
		} else {
			b.UpdatedAt = b.CreatedAt
		}
	} else {
		b.UpdatedAt = b.CreatedAt
	}
	return b, nil
}

func UpdateBoard(id, name, description, theme, icon string) (*Board, error) {
	query := `UPDATE boards SET name = ?, description = ?, theme = ?, icon = ?, updated_at = ? WHERE id = ?`
	_, err := DB.Exec(query, name, description, theme, icon, time.Now(), id)
	if err != nil {
		return nil, err
	}
	return GetBoard(id)
}

func ListUserBoards(userID string) ([]*Board, error) {
	query := `SELECT b.id, b.name, b.description, b.theme, COALESCE(b.icon, '📋'), b.owner_id, b.created_at, b.updated_at
	          FROM boards b
	          JOIN board_members bm ON b.id = bm.board_id
	          WHERE bm.user_id = ?
	          ORDER BY COALESCE(b.updated_at, b.created_at) DESC`
	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boards := []*Board{}
	for rows.Next() {
		b := &Board{}
		var updatedAtStr sql.NullString
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Theme, &b.Icon, &b.OwnerID, &b.CreatedAt, &updatedAtStr); err != nil {
			return nil, err
		}
		// Parse updated_at or fall back to created_at
		if updatedAtStr.Valid && updatedAtStr.String != "" {
			if parsed, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", updatedAtStr.String); err == nil {
				b.UpdatedAt = parsed
			} else if parsed, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
				b.UpdatedAt = parsed
			} else {
				b.UpdatedAt = b.CreatedAt
			}
		} else {
			b.UpdatedAt = b.CreatedAt
		}
		boards = append(boards, b)
	}
	return boards, nil
}

// parseBoardUpdatedAt parses the stored updated_at string, falling back to
// the given created_at on null/empty/unparseable values.
func parseBoardUpdatedAt(updatedAtStr sql.NullString, createdAt time.Time) time.Time {
	if updatedAtStr.Valid && updatedAtStr.String != "" {
		if parsed, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", updatedAtStr.String); err == nil {
			return parsed
		}
		if parsed, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
			return parsed
		}
	}
	return createdAt
}

// ListAllBoards returns every board with owner username and member count,
// for the admin boards view.
func ListAllBoards() ([]*AdminBoardRow, error) {
	query := `SELECT b.id, b.name, b.description, b.theme, COALESCE(b.icon, '📋'), b.owner_id, b.created_at, b.updated_at,
	                 u.username,
	                 (SELECT COUNT(*) FROM board_members bm WHERE bm.board_id = b.id)
	          FROM boards b
	          JOIN users u ON u.id = b.owner_id
	          ORDER BY COALESCE(b.updated_at, b.created_at) DESC`
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	boards := []*AdminBoardRow{}
	for rows.Next() {
		b := &AdminBoardRow{}
		var updatedAtStr sql.NullString
		if err := rows.Scan(&b.ID, &b.Name, &b.Description, &b.Theme, &b.Icon, &b.OwnerID, &b.CreatedAt, &updatedAtStr, &b.OwnerUsername, &b.MemberCount); err != nil {
			return nil, err
		}
		b.UpdatedAt = parseBoardUpdatedAt(updatedAtStr, b.CreatedAt)
		boards = append(boards, b)
	}
	return boards, nil
}

// UpdateBoardOwner transfers board ownership: the new owner gains the 'owner'
// member role (added to the board if needed) and the previous owner stays on
// the board as a regular member.
func UpdateBoardOwner(boardID, newOwnerID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldOwnerID string
	if err := tx.QueryRow(`SELECT owner_id FROM boards WHERE id = ?`, boardID).Scan(&oldOwnerID); err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE boards SET owner_id = ? WHERE id = ?`, newOwnerID, boardID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO board_members (board_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)
	                      ON CONFLICT(board_id, user_id) DO UPDATE SET role = 'owner'`, boardID, newOwnerID, time.Now()); err != nil {
		return err
	}
	if oldOwnerID != newOwnerID {
		if _, err := tx.Exec(`UPDATE board_members SET role = 'member' WHERE board_id = ? AND user_id = ?`, boardID, oldOwnerID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func AddBoardMember(boardID, userID, role string) error {
	query := `INSERT INTO board_members (board_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)
	          ON CONFLICT(board_id, user_id) DO UPDATE SET role = excluded.role`
	_, err := DB.Exec(query, boardID, userID, role, time.Now())
	return err
}

func RemoveBoardMember(boardID, userID string) error {
	// Cannot remove the owner
	board, err := GetBoard(boardID)
	if err != nil {
		return err
	}
	if board.OwnerID == userID {
		return errors.New("cannot remove the board owner")
	}

	query := `DELETE FROM board_members WHERE board_id = ? AND user_id = ?`
	_, err = DB.Exec(query, boardID, userID)
	return err
}

func GetBoardMembers(boardID string) ([]*User, error) {
	query := `SELECT u.id, u.username, u.email, u.avatar_color, u.created_at 
	          FROM users u 
	          JOIN board_members bm ON u.id = bm.user_id 
	          WHERE bm.board_id = ?`
	rows, err := DB.Query(query, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarColor, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func IsBoardMember(boardID, userID string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM board_members WHERE board_id = ? AND user_id = ?`
	err := DB.QueryRow(query, boardID, userID).Scan(&count)
	return count > 0, err
}

// GetCollaborators returns users the current user has collaborated with on other boards,
// excluding users already on the specified board. Returns top 10 most frequent collaborators.
func GetCollaborators(userID, excludeBoardID string) ([]*User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.avatar_color, u.created_at, COUNT(*) as board_count
		FROM users u
		JOIN board_members bm ON u.id = bm.user_id
		WHERE bm.board_id IN (
			SELECT board_id FROM board_members WHERE user_id = ?
		)
		AND u.id != ?
		AND u.id NOT IN (
			SELECT user_id FROM board_members WHERE board_id = ?
		)
		GROUP BY u.id
		ORDER BY board_count DESC, u.username ASC
		LIMIT 10
	`
	rows, err := DB.Query(query, userID, userID, excludeBoardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}
	for rows.Next() {
		u := &User{}
		var boardCount int
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarColor, &u.CreatedAt, &boardCount); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func DeleteBoard(id string) error {
	query := `DELETE FROM boards WHERE id = ?`
	_, err := DB.Exec(query, id)
	return err
}

// CopyBoard creates a copy of a board with all its lists, tasks, labels, and checklist items.
// The newOwnerID becomes the owner of the new board.
// If includeMembers is true, all members (except the new owner) are copied as members,
// and the original owner becomes a regular member.
func CopyBoard(sourceBoardID, newName, newOwnerID string, includeMembers bool) (*Board, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get the source board within the transaction to avoid race conditions
	var sourceDescription, sourceTheme, sourceIcon string
	querySource := `SELECT description, theme, COALESCE(icon, '') FROM boards WHERE id = ?`
	if err := tx.QueryRow(querySource, sourceBoardID).Scan(&sourceDescription, &sourceTheme, &sourceIcon); err != nil {
		return nil, err
	}

	// Create new board
	newBoardID := uuid.New().String()
	now := time.Now()
	queryBoard := `INSERT INTO boards (id, name, description, theme, icon, owner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := tx.Exec(queryBoard, newBoardID, newName, sourceDescription, sourceTheme, sourceIcon, newOwnerID, now, now); err != nil {
		return nil, err
	}

	// Add new owner as board member
	queryMember := `INSERT INTO board_members (board_id, user_id, role, joined_at) VALUES (?, ?, 'owner', ?)`
	if _, err := tx.Exec(queryMember, newBoardID, newOwnerID, now); err != nil {
		return nil, err
	}

	// Copy members if requested
	if includeMembers {
		queryMembers := `SELECT user_id FROM board_members WHERE board_id = ? AND user_id != ?`
		rows, err := tx.Query(queryMembers, sourceBoardID, newOwnerID)
		if err != nil {
			return nil, err
		}
		memberIDs := []string{}
		for rows.Next() {
			var memberID string
			if err := rows.Scan(&memberID); err != nil {
				rows.Close()
				return nil, err
			}
			memberIDs = append(memberIDs, memberID)
		}
		rows.Close()

		for _, memberID := range memberIDs {
			queryAddMember := `INSERT INTO board_members (board_id, user_id, role, joined_at) VALUES (?, ?, 'member', ?)`
			if _, err := tx.Exec(queryAddMember, newBoardID, memberID, now); err != nil {
				return nil, err
			}
		}
	}

	// Copy labels and track old->new ID mapping
	labelMap := make(map[string]string) // oldLabelID -> newLabelID
	queryLabels := `SELECT id, name, color FROM labels WHERE board_id = ?`
	labelRows, err := tx.Query(queryLabels, sourceBoardID)
	if err != nil {
		return nil, err
	}
	type labelData struct {
		oldID string
		name  string
		color string
	}
	labels := []labelData{}
	for labelRows.Next() {
		var l labelData
		if err := labelRows.Scan(&l.oldID, &l.name, &l.color); err != nil {
			labelRows.Close()
			return nil, err
		}
		labels = append(labels, l)
	}
	labelRows.Close()

	for _, l := range labels {
		newLabelID := uuid.New().String()
		labelMap[l.oldID] = newLabelID
		queryInsertLabel := `INSERT INTO labels (id, board_id, name, color) VALUES (?, ?, ?, ?)`
		if _, err := tx.Exec(queryInsertLabel, newLabelID, newBoardID, l.name, l.color); err != nil {
			return nil, err
		}
	}

	// Copy lists and track old->new ID mapping
	listMap := make(map[string]string) // oldListID -> newListID
	queryLists := `SELECT id, name, position FROM lists WHERE board_id = ? ORDER BY position ASC`
	listRows, err := tx.Query(queryLists, sourceBoardID)
	if err != nil {
		return nil, err
	}
	type listData struct {
		oldID    string
		name     string
		position int
	}
	lists := []listData{}
	for listRows.Next() {
		var l listData
		if err := listRows.Scan(&l.oldID, &l.name, &l.position); err != nil {
			listRows.Close()
			return nil, err
		}
		lists = append(lists, l)
	}
	listRows.Close()

	for _, l := range lists {
		newListID := uuid.New().String()
		listMap[l.oldID] = newListID
		queryInsertList := `INSERT INTO lists (id, board_id, name, position, created_at) VALUES (?, ?, ?, ?, ?)`
		if _, err := tx.Exec(queryInsertList, newListID, newBoardID, l.name, l.position, now); err != nil {
			return nil, err
		}
	}

	// Copy tasks for each list
	taskMap := make(map[string]string) // oldTaskID -> newTaskID
	for oldListID, newListID := range listMap {
		queryTasks := `SELECT id, title, description, link, position, due_date FROM tasks WHERE list_id = ? ORDER BY position ASC`
		taskRows, err := tx.Query(queryTasks, oldListID)
		if err != nil {
			return nil, err
		}
		type taskData struct {
			oldID       string
			title       string
			description string
			link        string
			position    int
			dueDate     sql.NullTime
		}
		tasks := []taskData{}
		for taskRows.Next() {
			var t taskData
			if err := taskRows.Scan(&t.oldID, &t.title, &t.description, &t.link, &t.position, &t.dueDate); err != nil {
				taskRows.Close()
				return nil, err
			}
			tasks = append(tasks, t)
		}
		taskRows.Close()

		for _, t := range tasks {
			newTaskID := uuid.New().String()
			taskMap[t.oldID] = newTaskID
			var queryInsertTask string
			if t.dueDate.Valid {
				queryInsertTask = `INSERT INTO tasks (id, list_id, title, description, link, position, due_date, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
				if _, err := tx.Exec(queryInsertTask, newTaskID, newListID, t.title, t.description, t.link, t.position, t.dueDate.Time, now, now); err != nil {
					return nil, err
				}
			} else {
				queryInsertTask = `INSERT INTO tasks (id, list_id, title, description, link, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
				if _, err := tx.Exec(queryInsertTask, newTaskID, newListID, t.title, t.description, t.link, t.position, now, now); err != nil {
					return nil, err
				}
			}
		}
	}

	// Copy task labels
	for oldTaskID, newTaskID := range taskMap {
		queryTaskLabels := `SELECT label_id FROM task_labels WHERE task_id = ?`
		tlRows, err := tx.Query(queryTaskLabels, oldTaskID)
		if err != nil {
			return nil, err
		}
		oldLabelIDs := []string{}
		for tlRows.Next() {
			var oldLabelID string
			if err := tlRows.Scan(&oldLabelID); err != nil {
				tlRows.Close()
				return nil, err
			}
			oldLabelIDs = append(oldLabelIDs, oldLabelID)
		}
		tlRows.Close()

		for _, oldLabelID := range oldLabelIDs {
			newLabelID, ok := labelMap[oldLabelID]
			if !ok {
				continue
			}
			queryInsertTL := `INSERT INTO task_labels (task_id, label_id) VALUES (?, ?)`
			if _, err := tx.Exec(queryInsertTL, newTaskID, newLabelID); err != nil {
				return nil, err
			}
		}
	}

	// Copy checklist items
	for oldTaskID, newTaskID := range taskMap {
		queryChecklist := `SELECT title, is_completed, position FROM checklist_items WHERE task_id = ? ORDER BY position ASC`
		clRows, err := tx.Query(queryChecklist, oldTaskID)
		if err != nil {
			return nil, err
		}
		type checklistData struct {
			title       string
			isCompleted int
			position    int
		}
		items := []checklistData{}
		for clRows.Next() {
			var c checklistData
			if err := clRows.Scan(&c.title, &c.isCompleted, &c.position); err != nil {
				clRows.Close()
				return nil, err
			}
			items = append(items, c)
		}
		clRows.Close()

		for _, c := range items {
			newItemID := uuid.New().String()
			queryInsertCL := `INSERT INTO checklist_items (id, task_id, title, is_completed, position) VALUES (?, ?, ?, ?, ?)`
			if _, err := tx.Exec(queryInsertCL, newItemID, newTaskID, c.title, c.isCompleted, c.position); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Return the new board
	return GetBoard(newBoardID)
}

// --- LIST OPERATIONS ---

func CreateList(boardID, name string, position int) (*List, error) {
	l := &List{
		ID:        uuid.New().String(),
		BoardID:   boardID,
		Name:      name,
		Position:  position,
		CreatedAt: time.Now(),
	}
	query := `INSERT INTO lists (id, board_id, name, position, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, l.ID, l.BoardID, l.Name, l.Position, l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func GetList(id string) (*List, error) {
	l := &List{}
	query := `SELECT id, board_id, name, position, created_at FROM lists WHERE id = ?`
	err := DB.QueryRow(query, id).Scan(&l.ID, &l.BoardID, &l.Name, &l.Position, &l.CreatedAt)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func UpdateList(id, name string, position int) error {
	query := `UPDATE lists SET name = ?, position = ? WHERE id = ?`
	_, err := DB.Exec(query, name, position, id)
	return err
}

func DeleteList(id string) error {
	query := `DELETE FROM lists WHERE id = ?`
	_, err := DB.Exec(query, id)
	return err
}

func GetListsByBoard(boardID string) ([]*List, error) {
	query := `SELECT id, board_id, name, position, created_at FROM lists WHERE board_id = ? ORDER BY position ASC`
	rows, err := DB.Query(query, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lists := []*List{}
	for rows.Next() {
		l := &List{}
		if err := rows.Scan(&l.ID, &l.BoardID, &l.Name, &l.Position, &l.CreatedAt); err != nil {
			return nil, err
		}
		lists = append(lists, l)
	}
	return lists, nil
}

// --- TASK OPERATIONS ---

func CreateTask(listID, title, description, link string, position int) (*Task, error) {
	t := &Task{
		ID:          uuid.New().String(),
		ListID:      listID,
		Title:       title,
		Description: description,
		Link:        link,
		Position:    position,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	query := `INSERT INTO tasks (id, list_id, title, description, link, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, t.ID, t.ListID, t.Title, t.Description, t.Link, t.Position, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func GetTask(id string) (*Task, error) {
	t := &Task{}
	var dueDate sql.NullTime
	query := `SELECT id, list_id, title, description, link, position, due_date, created_at, updated_at FROM tasks WHERE id = ?`
	err := DB.QueryRow(query, id).Scan(&t.ID, &t.ListID, &t.Title, &t.Description, &t.Link, &t.Position, &dueDate, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if dueDate.Valid {
		t.DueDate = &dueDate.Time
	}
	return t, nil
}

func UpdateTask(id, title, description, link, listID string, position int, dueDate *time.Time) error {
	var err error
	if dueDate != nil {
		query := `UPDATE tasks SET title = ?, description = ?, link = ?, list_id = ?, position = ?, due_date = ?, updated_at = ? WHERE id = ?`
		_, err = DB.Exec(query, title, description, link, listID, position, *dueDate, time.Now(), id)
	} else {
		query := `UPDATE tasks SET title = ?, description = ?, link = ?, list_id = ?, position = ?, due_date = NULL, updated_at = ? WHERE id = ?`
		_, err = DB.Exec(query, title, description, link, listID, position, time.Now(), id)
	}
	return err
}

func DeleteTask(id string) error {
	query := `DELETE FROM tasks WHERE id = ?`
	_, err := DB.Exec(query, id)
	return err
}

func GetTasksByList(listID string) ([]*Task, error) {
	query := `SELECT id, list_id, title, description, link, position, due_date, created_at, updated_at FROM tasks WHERE list_id = ? ORDER BY position ASC`
	rows, err := DB.Query(query, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []*Task{}
	for rows.Next() {
		t := &Task{}
		var dueDate sql.NullTime
		if err := rows.Scan(&t.ID, &t.ListID, &t.Title, &t.Description, &t.Link, &t.Position, &dueDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if dueDate.Valid {
			t.DueDate = &dueDate.Time
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// --- ASSIGNEE & LABEL OPERATIONS ---

func AssignUserToTask(taskID, userID string) error {
	query := `INSERT OR IGNORE INTO task_assignees (task_id, user_id) VALUES (?, ?)`
	_, err := DB.Exec(query, taskID, userID)
	return err
}

func UnassignUserFromTask(taskID, userID string) error {
	query := `DELETE FROM task_assignees WHERE task_id = ? AND user_id = ?`
	_, err := DB.Exec(query, taskID, userID)
	return err
}

func GetTaskAssignees(taskID string) ([]*User, error) {
	query := `SELECT u.id, u.username, u.email, u.avatar_color, u.created_at 
	          FROM users u 
	          JOIN task_assignees ta ON u.id = ta.user_id 
	          WHERE ta.task_id = ?`
	rows, err := DB.Query(query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assignees := []*User{}
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.AvatarColor, &u.CreatedAt); err != nil {
			return nil, err
		}
		assignees = append(assignees, u)
	}
	return assignees, nil
}

func CreateLabel(boardID, name, color string) (*Label, error) {
	l := &Label{
		ID:      uuid.New().String(),
		BoardID: boardID,
		Name:    name,
		Color:   color,
	}
	query := `INSERT INTO labels (id, board_id, name, color) VALUES (?, ?, ?, ?)`
	_, err := DB.Exec(query, l.ID, l.BoardID, l.Name, l.Color)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func GetBoardLabels(boardID string) ([]*Label, error) {
	query := `SELECT id, board_id, name, color FROM labels WHERE board_id = ?`
	rows, err := DB.Query(query, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []*Label{}
	for rows.Next() {
		l := &Label{}
		if err := rows.Scan(&l.ID, &l.BoardID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, nil
}

func AddLabelToTask(taskID, labelID string) error {
	query := `INSERT OR IGNORE INTO task_labels (task_id, label_id) VALUES (?, ?)`
	_, err := DB.Exec(query, taskID, labelID)
	return err
}

func RemoveLabelFromTask(taskID, labelID string) error {
	query := `DELETE FROM task_labels WHERE task_id = ? AND label_id = ?`
	_, err := DB.Exec(query, taskID, labelID)
	return err
}

func GetTaskLabels(taskID string) ([]*Label, error) {
	query := `SELECT l.id, l.board_id, l.name, l.color 
	          FROM labels l 
	          JOIN task_labels tl ON l.id = tl.label_id 
	          WHERE tl.task_id = ?`
	rows, err := DB.Query(query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []*Label{}
	for rows.Next() {
		l := &Label{}
		if err := rows.Scan(&l.ID, &l.BoardID, &l.Name, &l.Color); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, nil
}

// --- CHECKLIST OPERATIONS ---

func CreateChecklistItem(taskID, title string, position int) (*ChecklistItem, error) {
	item := &ChecklistItem{
		ID:          uuid.New().String(),
		TaskID:      taskID,
		Title:       title,
		IsCompleted: false,
		Position:    position,
	}
	query := `INSERT INTO checklist_items (id, task_id, title, is_completed, position) VALUES (?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, item.ID, item.TaskID, item.Title, 0, item.Position)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func UpdateChecklistItem(id, title string, isCompleted bool) error {
	completedInt := 0
	if isCompleted {
		completedInt = 1
	}
	query := `UPDATE checklist_items SET title = ?, is_completed = ? WHERE id = ?`
	_, err := DB.Exec(query, title, completedInt, id)
	return err
}

func DeleteChecklistItem(id string) error {
	query := `DELETE FROM checklist_items WHERE id = ?`
	_, err := DB.Exec(query, id)
	return err
}

func GetChecklistItems(taskID string) ([]*ChecklistItem, error) {
	query := `SELECT id, task_id, title, is_completed, position FROM checklist_items WHERE task_id = ? ORDER BY position ASC`
	rows, err := DB.Query(query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []*ChecklistItem{}
	for rows.Next() {
		item := &ChecklistItem{}
		var completedInt int
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Title, &completedInt, &item.Position); err != nil {
			return nil, err
		}
		item.IsCompleted = completedInt == 1
		items = append(items, item)
	}
	return items, nil
}

// --- COMMENT OPERATIONS ---

func CreateComment(taskID, userID, content string) (*Comment, error) {
	c := &Comment{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
	}
	query := `INSERT INTO comments (id, task_id, user_id, content, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, c.ID, c.TaskID, c.UserID, c.Content, c.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Fetch user details for the created comment struct
	u, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}
	c.Username = u.Username
	c.AvatarColor = u.AvatarColor

	return c, nil
}

func GetTaskComments(taskID string) ([]*Comment, error) {
	query := `SELECT c.id, c.task_id, c.user_id, u.username, u.avatar_color, c.content, c.created_at 
	          FROM comments c 
	          JOIN users u ON c.user_id = u.id 
	          WHERE c.task_id = ? 
	          ORDER BY c.created_at DESC`
	rows, err := DB.Query(query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := []*Comment{}
	for rows.Next() {
		c := &Comment{}
		if err := rows.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Username, &c.AvatarColor, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// --- ACTIVITY OPERATIONS ---

func LogActivity(boardID, userID, username, actionType, description string) (*Activity, error) {
	a := &Activity{
		ID:          uuid.New().String(),
		BoardID:     boardID,
		UserID:      userID,
		Username:    username,
		ActionType:  actionType,
		Description: description,
		CreatedAt:   time.Now(),
	}
	query := `INSERT INTO activities (id, board_id, user_id, username, action_type, description, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := DB.Exec(query, a.ID, a.BoardID, a.UserID, a.Username, a.ActionType, a.Description, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func GetBoardActivities(boardID string, limit int) ([]*Activity, error) {
	query := `SELECT id, board_id, user_id, username, action_type, description, created_at 
	          FROM activities 
	          WHERE board_id = ? 
	          ORDER BY created_at DESC 
	          LIMIT ?`
	rows, err := DB.Query(query, boardID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	activities := []*Activity{}
	for rows.Next() {
		a := &Activity{}
		if err := rows.Scan(&a.ID, &a.BoardID, &a.UserID, &a.Username, &a.ActionType, &a.Description, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	return activities, nil
}
