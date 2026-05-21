package main

import (
	"time"
)

type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	PasswordHash string    `json:"-"`
	AvatarColor string    `json:"avatar_color"`
	CreatedAt   time.Time `json:"created_at"`
}

type Session struct {
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Board struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Theme       string       `json:"theme"`
	Icon        string       `json:"icon"`
	OwnerID     string       `json:"owner_id"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Members     []*User      `json:"members,omitempty"`
	Lists       []*List      `json:"lists,omitempty"`
}

type BoardMember struct {
	BoardID  string    `json:"board_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type List struct {
	ID        string    `json:"id"`
	BoardID   string    `json:"board_id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	Tasks     []*Task   `json:"tasks,omitempty"`
}

type Task struct {
	ID          string          `json:"id"`
	ListID      string          `json:"list_id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Link        string          `json:"link"`
	Position    int             `json:"position"`
	DueDate     *time.Time      `json:"due_date"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Assignees   []*User         `json:"assignees,omitempty"`
	Labels      []*Label        `json:"labels,omitempty"`
	Checklist   []*ChecklistItem `json:"checklist,omitempty"`
}

type ChecklistItem struct {
	ID          string `json:"id"`
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	IsCompleted bool   `json:"is_completed"`
	Position    int    `json:"position"`
}

type Label struct {
	ID      string `json:"id"`
	BoardID string `json:"board_id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

type Comment struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	AvatarColor string    `json:"avatar_color"`
	Content     string    `json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}

type Activity struct {
	ID          string    `json:"id"`
	BoardID     string    `json:"board_id"`
	UserID      string    `json:"user_id"`
	Username    string    `json:"username"`
	ActionType  string    `json:"action_type"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}
