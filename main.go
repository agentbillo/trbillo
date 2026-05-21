package main

import (
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

func main() {
	// Environment configuration
	basePath := strings.TrimRight(os.Getenv("BASE_PATH"), "/")
	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "./trbillo.db")
	staticDir := envOr("STATIC_DIR", "./static")

	// Initialize Database
	if err := InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize SQLite database: %v", err)
	}
	log.Printf("SQLite Database initialized at %s", dbPath)

	// Initialize Real-time WebSocket Hub
	InitHub()
	go HubInstance.Run()
	log.Println("WebSocket Hub started")

	// Start background routine for cleaning expired sessions every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			CleanExpiredSessions()
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

	// --- AUDIT LOG & REAL-TIME ROUTING ---
	mux.HandleFunc("GET "+p+"/api/boards/{id}/activities", authMiddleware(GetBoardActivitiesHandler))
	mux.HandleFunc("GET "+p+"/api/ws", WebSocketHandler)
	mux.HandleFunc("GET "+p+"/api/ws/user", UserWebSocketHandler)

	// Start Server
	addr := ":" + port
	log.Printf("Server listening on %s (base path: %q)", addr, basePath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
