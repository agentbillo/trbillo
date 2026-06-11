package main

import (
	"testing"
	"time"
)

func TestDatabaseOperations(t *testing.T) {
	// 1. Initialize an in-memory SQLite database
	err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	// 2. Test User Creation
	user, err := CreateUser("testuser", "test@example.com", "hashed_pwd_123", "#6366f1")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if user.Username != "testuser" || user.Email != "test@example.com" {
		t.Errorf("User details mismatch. Got Username=%s, Email=%s", user.Username, user.Email)
	}

	// 3. Test Retrieve User
	retrievedUser, err := GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if retrievedUser.ID != user.ID {
		t.Errorf("Retrieved user ID mismatch. Got %s, expected %s", retrievedUser.ID, user.ID)
	}

	retrievedUser2, err := GetUserByUsernameOrEmail("test@example.com")
	if err != nil {
		t.Fatalf("Failed to retrieve user by email: %v", err)
	}
	if retrievedUser2.ID != user.ID {
		t.Errorf("Retrieved user ID mismatch on email lookup. Got %s, expected %s", retrievedUser2.ID, user.ID)
	}

	// 4. Test Board Creation (which initializes 4 default columns and 5 default labels)
	board, err := CreateBoard("Test Board", "A board for unit testing", user.ID)
	if err != nil {
		t.Fatalf("Failed to create board: %v", err)
	}
	if board.Name != "Test Board" || board.OwnerID != user.ID {
		t.Errorf("Board details mismatch. Got Name=%s, OwnerID=%s", board.Name, board.OwnerID)
	}

	// 5. Verify default lists are created for the board
	lists, err := GetListsByBoard(board.ID)
	if err != nil {
		t.Fatalf("Failed to fetch board lists: %v", err)
	}
	if len(lists) != 4 {
		t.Errorf("Expected 4 default lists, got %d", len(lists))
	}
	expectedListNames := []string{"To Do", "In Progress", "Review", "Done"}
	for i, list := range lists {
		if list.Name != expectedListNames[i] {
			t.Errorf("List order mismatch. Position %d expected %s, got %s", i, expectedListNames[i], list.Name)
		}
	}

	// 6. Verify default labels are created for the board
	labels, err := GetBoardLabels(board.ID)
	if err != nil {
		t.Fatalf("Failed to fetch board labels: %v", err)
	}
	if len(labels) != 5 {
		t.Errorf("Expected 5 default labels, got %d", len(labels))
	}

	// 7. Test Task Creation
	todoList := lists[0]
	task, err := CreateTask(todoList.ID, "Write unit tests", "", "", 0)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	if task.Title != "Write unit tests" || task.ListID != todoList.ID {
		t.Errorf("Task details mismatch. Got Title=%s, ListID=%s", task.Title, task.ListID)
	}

	// 8. Test Task Update
	newDesc := "Write more robust unit tests and verify they pass."
	err = UpdateTask(task.ID, "Write unit tests (Updated)", newDesc, "", todoList.ID, 0, nil)
	if err != nil {
		t.Fatalf("Failed to update task: %v", err)
	}

	updatedTask, err := GetTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to fetch updated task: %v", err)
	}
	if updatedTask.Title != "Write unit tests (Updated)" || updatedTask.Description != newDesc {
		t.Errorf("Updated task fields mismatch. Title=%s, Description=%s", updatedTask.Title, updatedTask.Description)
	}

	// 9. Test Activity Logging
	activity, err := LogActivity(board.ID, user.ID, user.Username, "test_action", "completed unit test database run")
	if err != nil {
		t.Fatalf("Failed to log activity: %v", err)
	}
	if activity.BoardID != board.ID || activity.Username != user.Username {
		t.Errorf("Activity log mismatch. Got BoardID=%s, Username=%s", activity.BoardID, activity.Username)
	}

	activities, err := GetBoardActivities(board.ID, 10)
	if err != nil {
		t.Fatalf("Failed to fetch board activities: %v", err)
	}
	if len(activities) < 1 {
		t.Errorf("Expected at least 1 board activity, got %d", len(activities))
	}
}

func TestAdminDBOperations(t *testing.T) {
	if err := InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	// EnsureAdminUser creates the locked admin once, then is idempotent
	admin, created, err := EnsureAdminUser()
	if err != nil || !created {
		t.Fatalf("Expected admin to be created, got created=%v err=%v", created, err)
	}
	if admin.Username != AdminUsername {
		t.Errorf("Admin username mismatch: %s", admin.Username)
	}
	if _, createdAgain, err := EnsureAdminUser(); err != nil || createdAgain {
		t.Errorf("Expected existing admin on second call, got created=%v err=%v", createdAgain, err)
	}
	stored, err := GetUserByID(admin.ID)
	if err != nil {
		t.Fatalf("Failed to fetch admin: %v", err)
	}
	if stored.PasswordHash != UnusablePasswordHash {
		t.Errorf("Expected unusable password hash, got %q", stored.PasswordHash)
	}

	// Seed two users and a board owned by alice
	alice, err := CreateUser("alice", "alice@example.com", "h1", "#ffffff")
	if err != nil {
		t.Fatalf("Failed to create alice: %v", err)
	}
	bob, err := CreateUser("bob", "bob@example.com", "h2", "#000000")
	if err != nil {
		t.Fatalf("Failed to create bob: %v", err)
	}
	board, err := CreateBoard("Owned Board", "desc", alice.ID)
	if err != nil {
		t.Fatalf("Failed to create board: %v", err)
	}

	// Admin listing queries
	boards, err := ListAllBoards()
	if err != nil {
		t.Fatalf("ListAllBoards failed: %v", err)
	}
	if len(boards) != 1 || boards[0].OwnerUsername != "alice" || boards[0].MemberCount != 1 {
		t.Errorf("ListAllBoards mismatch: %+v", boards)
	}
	users, err := ListAllUsers()
	if err != nil {
		t.Fatalf("ListAllUsers failed: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("Expected 3 users, got %d", len(users))
	}
	for _, u := range users {
		if u.Username == "alice" && (u.BoardsOwned != 1 || u.BoardsMemberOf != 1) {
			t.Errorf("alice counts mismatch: owned=%d memberOf=%d", u.BoardsOwned, u.BoardsMemberOf)
		}
	}

	// Reassign ownership to bob: bob becomes owner+member, alice stays as member
	if err := UpdateBoardOwner(board.ID, bob.ID); err != nil {
		t.Fatalf("UpdateBoardOwner failed: %v", err)
	}
	updated, err := GetBoard(board.ID)
	if err != nil {
		t.Fatalf("GetBoard failed: %v", err)
	}
	if updated.OwnerID != bob.ID {
		t.Errorf("Owner not reassigned: %s", updated.OwnerID)
	}
	members, err := GetBoardMembers(board.ID)
	if err != nil {
		t.Fatalf("GetBoardMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("Expected 2 members after reassign, got %d", len(members))
	}
	if owned, _ := CountBoardsOwnedBy(bob.ID); owned != 1 {
		t.Errorf("Expected bob to own 1 board, got %d", owned)
	}

	// Alice no longer owns anything and can be deleted; membership cascades
	if owned, _ := CountBoardsOwnedBy(alice.ID); owned != 0 {
		t.Errorf("Expected alice to own 0 boards, got %d", owned)
	}
	if err := DeleteUser(alice.ID); err != nil {
		t.Fatalf("DeleteUser(alice) failed: %v", err)
	}
	members, _ = GetBoardMembers(board.ID)
	if len(members) != 1 {
		t.Errorf("Expected 1 member after alice deleted, got %d", len(members))
	}
	if err := DeleteUser(alice.ID); err == nil {
		t.Errorf("Expected error deleting alice twice")
	}
}

func TestUpdateUserPasswordAndDeleteSessions(t *testing.T) {
	if err := InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	user, err := CreateUser("pwuser", "pw@example.com", "old_hash", "#6366f1")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err := UpdateUserPassword(user.ID, "new_hash"); err != nil {
		t.Fatalf("Failed to update password: %v", err)
	}
	updated, err := GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("Failed to retrieve user: %v", err)
	}
	if updated.PasswordHash != "new_hash" {
		t.Errorf("Password hash not updated. Got %s", updated.PasswordHash)
	}

	if err := UpdateUserPassword("no-such-id", "x"); err == nil {
		t.Errorf("Expected error updating password for unknown user, got nil")
	}

	if err := CreateSession("tok1", user.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	if err := DeleteUserSessions(user.ID); err != nil {
		t.Fatalf("Failed to delete user sessions: %v", err)
	}
	if _, err := GetSession("tok1"); err == nil {
		t.Errorf("Expected session to be gone after DeleteUserSessions")
	}
}
