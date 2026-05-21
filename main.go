package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	// Initialize Database
	dbPath := "./trbillo.db"
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

	// Setup Mux (Router) using Go 1.22+ native routing enhancements
	mux := http.NewServeMux()

	// --- STATIC ASSETS ---
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/index.html")
	})
	// Serve static JS/CSS assets
	fileServer := http.FileServer(http.Dir("./static"))
	mux.Handle("GET /static/", http.StripPrefix("/static/", fileServer))

	// Helper: combine CSRF + Auth middleware for protected state-changing routes
	protectedHandler := func(h http.HandlerFunc) http.HandlerFunc {
		return csrfMiddleware(authMiddleware(h))
	}

	// --- AUTH ROUTING (with rate limiting) ---
	mux.HandleFunc("POST /api/auth/register", rateLimitMiddleware(csrfMiddleware(RegisterHandler)))
	mux.HandleFunc("POST /api/auth/login", rateLimitMiddleware(csrfMiddleware(LoginHandler)))
	mux.HandleFunc("POST /api/auth/logout", csrfMiddleware(LogoutHandler))
	mux.HandleFunc("GET /api/auth/me", authMiddleware(MeHandler))

	// --- BOARDS ROUTING ---
	mux.HandleFunc("GET /api/boards", authMiddleware(ListBoardsHandler))
	mux.HandleFunc("POST /api/boards", protectedHandler(CreateBoardHandler))
	mux.HandleFunc("GET /api/boards/{id}", authMiddleware(GetBoardHandler))
	mux.HandleFunc("PATCH /api/boards/{id}", protectedHandler(UpdateBoardHandler))
	mux.HandleFunc("DELETE /api/boards/{id}", protectedHandler(DeleteBoardHandler))
	mux.HandleFunc("POST /api/boards/{id}/members", protectedHandler(AddBoardMemberHandler))
	mux.HandleFunc("DELETE /api/boards/{id}/members/{user_id}", protectedHandler(RemoveBoardMemberHandler))
	mux.HandleFunc("GET /api/boards/{id}/collaborators", authMiddleware(GetCollaboratorsHandler))
	mux.HandleFunc("POST /api/boards/{id}/copy", protectedHandler(CopyBoardHandler))

	// --- LISTS ROUTING ---
	mux.HandleFunc("POST /api/boards/{board_id}/lists", protectedHandler(CreateListHandler))
	mux.HandleFunc("PATCH /api/lists/{id}", protectedHandler(UpdateListHandler))
	mux.HandleFunc("DELETE /api/lists/{id}", protectedHandler(DeleteListHandler))

	// --- TASKS & COMMENTS ROUTING ---
	mux.HandleFunc("POST /api/lists/{list_id}/tasks", protectedHandler(CreateTaskHandler))
	mux.HandleFunc("GET /api/tasks/{id}", authMiddleware(GetTaskHandler))
	mux.HandleFunc("PATCH /api/tasks/{id}", protectedHandler(UpdateTaskHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}", protectedHandler(DeleteTaskHandler))
	mux.HandleFunc("POST /api/tasks/{id}/comments", protectedHandler(CreateCommentHandler))
	mux.HandleFunc("GET /api/tasks/{id}/comments", authMiddleware(ListTaskCommentsHandler))

	// --- ASSIGNEES ROUTING ---
	mux.HandleFunc("POST /api/tasks/{id}/assignees", protectedHandler(AssignTaskHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}/assignees", protectedHandler(UnassignTaskHandler))

	// --- LABELS ROUTING ---
	mux.HandleFunc("POST /api/boards/{id}/labels", protectedHandler(CreateBoardLabelHandler))
	mux.HandleFunc("GET /api/boards/{id}/labels", authMiddleware(ListBoardLabelsHandler))
	mux.HandleFunc("POST /api/tasks/{id}/labels", protectedHandler(AddTaskLabelHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}/labels", protectedHandler(RemoveTaskLabelHandler))

	// --- CHECKLIST ROUTING ---
	mux.HandleFunc("POST /api/tasks/{id}/checklist", protectedHandler(CreateChecklistItemHandler))
	mux.HandleFunc("PATCH /api/checklist/{id}", protectedHandler(UpdateChecklistItemHandler))
	mux.HandleFunc("DELETE /api/checklist/{id}", protectedHandler(DeleteChecklistItemHandler))

	// --- AUDIT LOG & REAL-TIME ROUTING ---
	mux.HandleFunc("GET /api/boards/{id}/activities", authMiddleware(GetBoardActivitiesHandler))
	mux.HandleFunc("GET /api/ws", WebSocketHandler)
	mux.HandleFunc("GET /api/ws/user", UserWebSocketHandler)

	// Start Server
	port := ":8080"
	log.Printf("Server listening on port %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
