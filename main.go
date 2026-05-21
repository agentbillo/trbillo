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

	// --- AUTH ROUTING ---
	mux.HandleFunc("POST /api/auth/register", RegisterHandler)
	mux.HandleFunc("POST /api/auth/login", LoginHandler)
	mux.HandleFunc("POST /api/auth/logout", LogoutHandler)
	mux.HandleFunc("GET /api/auth/me", authMiddleware(MeHandler))

	// --- BOARDS ROUTING ---
	mux.HandleFunc("GET /api/boards", authMiddleware(ListBoardsHandler))
	mux.HandleFunc("POST /api/boards", authMiddleware(CreateBoardHandler))
	mux.HandleFunc("GET /api/boards/{id}", authMiddleware(GetBoardHandler))
	mux.HandleFunc("PATCH /api/boards/{id}", authMiddleware(UpdateBoardHandler))
	mux.HandleFunc("DELETE /api/boards/{id}", authMiddleware(DeleteBoardHandler))
	mux.HandleFunc("POST /api/boards/{id}/members", authMiddleware(AddBoardMemberHandler))
	mux.HandleFunc("DELETE /api/boards/{id}/members/{user_id}", authMiddleware(RemoveBoardMemberHandler))
	mux.HandleFunc("GET /api/boards/{id}/collaborators", authMiddleware(GetCollaboratorsHandler))

	// --- LISTS ROUTING ---
	mux.HandleFunc("POST /api/boards/{board_id}/lists", authMiddleware(CreateListHandler))
	mux.HandleFunc("PATCH /api/lists/{id}", authMiddleware(UpdateListHandler))
	mux.HandleFunc("DELETE /api/lists/{id}", authMiddleware(DeleteListHandler))

	// --- TASKS & COMMENTS ROUTING ---
	mux.HandleFunc("POST /api/lists/{list_id}/tasks", authMiddleware(CreateTaskHandler))
	mux.HandleFunc("GET /api/tasks/{id}", authMiddleware(GetTaskHandler))
	mux.HandleFunc("PATCH /api/tasks/{id}", authMiddleware(UpdateTaskHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}", authMiddleware(DeleteTaskHandler))
	mux.HandleFunc("POST /api/tasks/{id}/comments", authMiddleware(CreateCommentHandler))
	mux.HandleFunc("GET /api/tasks/{id}/comments", authMiddleware(ListTaskCommentsHandler))

	// --- ASSIGNEES ROUTING ---
	mux.HandleFunc("POST /api/tasks/{id}/assignees", authMiddleware(AssignTaskHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}/assignees", authMiddleware(UnassignTaskHandler))

	// --- LABELS ROUTING ---
	mux.HandleFunc("POST /api/boards/{id}/labels", authMiddleware(CreateBoardLabelHandler))
	mux.HandleFunc("GET /api/boards/{id}/labels", authMiddleware(ListBoardLabelsHandler))
	mux.HandleFunc("POST /api/tasks/{id}/labels", authMiddleware(AddTaskLabelHandler))
	mux.HandleFunc("DELETE /api/tasks/{id}/labels", authMiddleware(RemoveTaskLabelHandler))

	// --- CHECKLIST ROUTING ---
	mux.HandleFunc("POST /api/tasks/{id}/checklist", authMiddleware(CreateChecklistItemHandler))
	mux.HandleFunc("PATCH /api/checklist/{id}", authMiddleware(UpdateChecklistItemHandler))
	mux.HandleFunc("DELETE /api/checklist/{id}", authMiddleware(DeleteChecklistItemHandler))

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
