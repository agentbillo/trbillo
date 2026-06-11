package main

import (
	"bufio"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// computeStaticVersion returns the latest mtime of any file under dir as a
// unix-seconds string. Used as a cache-busting query string on asset URLs.
func computeStaticVersion(dir string) string {
	var maxMtime int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		if t := info.ModTime().Unix(); t > maxMtime {
			maxMtime = t
		}
		return nil
	})
	if maxMtime == 0 {
		return strconv.FormatInt(time.Now().Unix(), 10)
	}
	return strconv.FormatInt(maxMtime, 10)
}

// openCLIDB opens the database for one-off CLI modes that may run alongside
// the live server.
func openCLIDB(dbPath string) error {
	if err := InitDB(dbPath); err != nil {
		return fmt.Errorf("opening database at %s: %w", dbPath, err)
	}
	// Wait out the running server's write lock instead of failing immediately.
	if _, err := DB.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		return err
	}
	return nil
}

// setPasswordFor prompts for a new password and replaces the stored hash for
// the given user, logging them out everywhere.
func setPasswordFor(u *User) error {
	password, err := promptNewPassword()
	if err != nil {
		return err
	}
	if len(password) < MinPasswordLen {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLen)
	}
	if len(password) > MaxPasswordLen {
		return fmt.Errorf("password must be %d characters or less", MaxPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := UpdateUserPassword(u.ID, string(hash)); err != nil {
		return err
	}
	if err := DeleteUserSessions(u.ID); err != nil {
		return fmt.Errorf("password updated, but clearing existing sessions failed: %w", err)
	}

	fmt.Printf("Password updated for %s (%s); all existing sessions logged out.\n", u.Username, u.Email)
	return nil
}

// resetPassword implements the -reset-password CLI mode.
func resetPassword(dbPath, identifier string) error {
	if err := openCLIDB(dbPath); err != nil {
		return err
	}
	defer DB.Close()

	u, err := GetUserByUsernameOrEmail(identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no user with username or email %q", identifier)
	}
	if err != nil {
		return err
	}
	return setPasswordFor(u)
}

// setAdminPassword implements the -set-admin CLI mode: creates the admin
// user if missing, then sets its password.
func setAdminPassword(dbPath string) error {
	if err := openCLIDB(dbPath); err != nil {
		return err
	}
	defer DB.Close()

	u, created, err := EnsureAdminUser()
	if err != nil {
		return fmt.Errorf("ensuring admin user exists: %w", err)
	}
	if created {
		fmt.Println("Created admin user.")
	}
	return setPasswordFor(u)
}

// promptNewPassword reads the new password without echo when stdin is a
// terminal (asking twice to confirm), or takes a single line when piped.
func promptNewPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	fmt.Print("New password: ")
	first, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	fmt.Print("Confirm password: ")
	second, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", errors.New("passwords do not match")
	}
	return string(first), nil
}

func main() {
	resetTarget := flag.String("reset-password", "",
		"reset the password for the given username or email, then exit")
	setAdmin := flag.Bool("set-admin", false,
		"create the admin user if missing and set its password, then exit")
	flag.Parse()

	// Environment configuration
	basePath := strings.TrimRight(os.Getenv("BASE_PATH"), "/")
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "./trbillo.db")
	staticDir := envOr("STATIC_DIR", "./static")

	if *setAdmin {
		if err := setAdminPassword(dbPath); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}
	if *resetTarget != "" {
		if err := resetPassword(dbPath, *resetTarget); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	// Initialize Database
	if err := InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	log.Printf("SQLite Database initialized at %s", dbPath)

	// Make sure the admin account exists (locked until -set-admin gives it a password)
	if _, created, err := EnsureAdminUser(); err != nil {
		log.Printf("Warning: could not ensure admin user exists: %v", err)
	} else if created {
		log.Printf("Admin user created (locked). Run 'trbillo -set-admin' to set its password.")
	}

	// Initialize Real-time WebSocket Hub
	InitHub()
	go HubInstance.Run()
	log.Println("WebSocket Hub started")

	// Clean up expired sessions and expired PUBLIC trial accounts hourly
	// (and once at startup so restarts don't postpone the trial wipe)
	cleanup := func() {
		CleanExpiredSessions()
		if _, err := CleanExpiredTrialUsers(); err != nil {
			log.Printf("Trial cleanup error: %v", err)
		}
	}
	cleanup()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanup()
		}
	}()

	// Read index.html once at startup and template in the base path and
	// a cache-busting version string derived from the static dir mtime.
	indexBytes, err := os.ReadFile(staticDir + "/index.html")
	if err != nil {
		log.Fatalf("Failed to read index.html: %v", err)
	}
	version := computeStaticVersion(staticDir)
	indexStr := string(indexBytes)
	indexStr = strings.ReplaceAll(indexStr, "{{BASE_PATH}}", basePath)
	indexStr = strings.ReplaceAll(indexStr, "{{VERSION}}", version)
	indexHTML := []byte(indexStr)
	log.Printf("Static asset version: %s", version)

	// Setup Mux (Router) using Go 1.22+ native routing enhancements
	mux := http.NewServeMux()
	p := basePath

	// --- STATIC ASSETS ---
	mux.HandleFunc("GET "+p+"/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(indexHTML)
	})
	fileServer := http.FileServer(http.Dir(staticDir))
	staticPrefix := p + "/static/"
	mux.Handle("GET "+staticPrefix, http.StripPrefix(staticPrefix, fileServer))

	// Helper: combine CSRF + Auth middleware for protected state-changing routes
	protectedHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return csrfMiddleware(authMiddleware(h))
	}

	// --- AUTH ROUTING (with rate limiting) ---
	mux.HandleFunc("POST "+p+"/api/auth/register", rateLimitMiddleware(csrfMiddleware(RegisterHandler)))
	mux.HandleFunc("POST "+p+"/api/auth/login", rateLimitMiddleware(csrfMiddleware(LoginHandler)))
	mux.HandleFunc("POST "+p+"/api/auth/logout", csrfMiddleware(LogoutHandler))
	mux.HandleFunc("GET "+p+"/api/auth/me", authMiddleware(MeHandler))

	// --- BOARDS ROUTING ---
	mux.HandleFunc("GET "+p+"/api/boards", authMiddleware(ListBoardsHandler))
	mux.HandleFunc("POST "+p+"/api/boards", protectedHandler(CreateBoardHandler))
	mux.HandleFunc("GET "+p+"/api/boards/{id}", authMiddleware(GetBoardHandler))
	mux.HandleFunc("PATCH "+p+"/api/boards/{id}", protectedHandler(UpdateBoardHandler))
	mux.HandleFunc("DELETE "+p+"/api/boards/{id}", protectedHandler(DeleteBoardHandler))
	mux.HandleFunc("POST "+p+"/api/boards/{id}/members", protectedHandler(AddBoardMemberHandler))
	mux.HandleFunc("DELETE "+p+"/api/boards/{id}/members/{user_id}", protectedHandler(RemoveBoardMemberHandler))
	mux.HandleFunc("GET "+p+"/api/boards/{id}/collaborators", authMiddleware(GetCollaboratorsHandler))
	mux.HandleFunc("POST "+p+"/api/boards/{id}/copy", protectedHandler(CopyBoardHandler))

	// --- LISTS ROUTING ---
	mux.HandleFunc("POST "+p+"/api/boards/{board_id}/lists", protectedHandler(CreateListHandler))
	mux.HandleFunc("PATCH "+p+"/api/lists/{id}", protectedHandler(UpdateListHandler))
	mux.HandleFunc("DELETE "+p+"/api/lists/{id}", protectedHandler(DeleteListHandler))

	// --- TASKS & COMMENTS ROUTING ---
	mux.HandleFunc("POST "+p+"/api/lists/{list_id}/tasks", protectedHandler(CreateTaskHandler))
	mux.HandleFunc("GET "+p+"/api/tasks/{id}", authMiddleware(GetTaskHandler))
	mux.HandleFunc("PATCH "+p+"/api/tasks/{id}", protectedHandler(UpdateTaskHandler))
	mux.HandleFunc("DELETE "+p+"/api/tasks/{id}", protectedHandler(DeleteTaskHandler))
	mux.HandleFunc("POST "+p+"/api/tasks/{id}/comments", protectedHandler(CreateCommentHandler))
	mux.HandleFunc("GET "+p+"/api/tasks/{id}/comments", authMiddleware(ListTaskCommentsHandler))

	// --- ASSIGNEES ROUTING ---
	mux.HandleFunc("POST "+p+"/api/tasks/{id}/assignees", protectedHandler(AssignTaskHandler))
	mux.HandleFunc("DELETE "+p+"/api/tasks/{id}/assignees", protectedHandler(UnassignTaskHandler))

	// --- LABELS ROUTING ---
	mux.HandleFunc("POST "+p+"/api/boards/{id}/labels", protectedHandler(CreateBoardLabelHandler))
	mux.HandleFunc("GET "+p+"/api/boards/{id}/labels", authMiddleware(ListBoardLabelsHandler))
	mux.HandleFunc("POST "+p+"/api/tasks/{id}/labels", protectedHandler(AddTaskLabelHandler))
	mux.HandleFunc("DELETE "+p+"/api/tasks/{id}/labels", protectedHandler(RemoveTaskLabelHandler))

	// --- CHECKLIST ROUTING ---
	mux.HandleFunc("POST "+p+"/api/tasks/{id}/checklist", protectedHandler(CreateChecklistItemHandler))
	mux.HandleFunc("PATCH "+p+"/api/checklist/{id}", protectedHandler(UpdateChecklistItemHandler))
	mux.HandleFunc("DELETE "+p+"/api/checklist/{id}", protectedHandler(DeleteChecklistItemHandler))

	// --- TEAM ROUTING ---
	mux.HandleFunc("GET "+p+"/api/team", authMiddleware(TeamHandler))

	// --- AUDIT LOG & REAL-TIME ROUTING ---
	mux.HandleFunc("GET "+p+"/api/boards/{id}/activities", authMiddleware(GetBoardActivitiesHandler))
	mux.HandleFunc("GET "+p+"/api/ws", WebSocketHandler)
	mux.HandleFunc("GET "+p+"/api/ws/user", UserWebSocketHandler)

	// --- ADMIN ROUTING ---
	adminRead := func(h http.HandlerFunc) http.HandlerFunc {
		return authMiddleware(requireAdmin(h))
	}
	adminWrite := func(h http.HandlerFunc) http.HandlerFunc {
		return csrfMiddleware(authMiddleware(requireAdmin(h)))
	}
	mux.HandleFunc("GET "+p+"/api/admin/boards", adminRead(AdminListBoardsHandler))
	mux.HandleFunc("GET "+p+"/api/admin/users", adminRead(AdminListUsersHandler))
	mux.HandleFunc("GET "+p+"/api/admin/boards/{id}/members", adminRead(AdminGetBoardMembersHandler))
	mux.HandleFunc("POST "+p+"/api/admin/users", adminWrite(AdminCreateUserHandler))
	mux.HandleFunc("DELETE "+p+"/api/admin/users/{id}", adminWrite(AdminDeleteUserHandler))
	mux.HandleFunc("POST "+p+"/api/admin/users/{id}/password", adminWrite(AdminSetUserPasswordHandler))
	mux.HandleFunc("POST "+p+"/api/admin/users/{id}/team", adminWrite(AdminSetUserTeamHandler))
	mux.HandleFunc("DELETE "+p+"/api/admin/boards/{id}/members/{user_id}", adminWrite(AdminRemoveBoardMemberHandler))
	mux.HandleFunc("POST "+p+"/api/admin/boards/{id}/owner", adminWrite(AdminSetBoardOwnerHandler))
	mux.HandleFunc("GET "+p+"/api/admin/teams", adminRead(AdminListTeamsHandler))
	mux.HandleFunc("POST "+p+"/api/admin/teams", adminWrite(AdminCreateTeamHandler))
	mux.HandleFunc("POST "+p+"/api/admin/teams/{name}/code", adminWrite(AdminSetTeamCodeHandler))
	mux.HandleFunc("DELETE "+p+"/api/admin/teams/{name}", adminWrite(AdminDeleteTeamHandler))

	// Start Server
	addr := ":" + port
	log.Printf("Server listening on %s (base path: %q)", addr, basePath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
