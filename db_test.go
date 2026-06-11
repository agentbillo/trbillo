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
	user, err := CreateUser("testuser", "test@example.com", "hashed_pwd_123", "#6366f1", "PUBLIC")
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
	alice, err := CreateUser("alice", "alice@example.com", "h1", "#ffffff", "")
	if err != nil {
		t.Fatalf("Failed to create alice: %v", err)
	}
	bob, err := CreateUser("bob", "bob@example.com", "h2", "#000000", "")
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

func TestTeams(t *testing.T) {
	if err := InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	// The PUBLIC team is seeded with code PUBLIC
	if name, err := GetTeamNameByCode("PUBLIC"); err != nil || name != PublicTeam {
		t.Fatalf("Expected PUBLIC team seeded with code PUBLIC, got %q err=%v", name, err)
	}

	// Create a team whose code differs from its name
	if err := CreateTeam("Snakkos", "LUNCHTIME"); err != nil {
		t.Fatalf("CreateTeam failed: %v", err)
	}
	if name, err := GetTeamNameByCode("LUNCHTIME"); err != nil || name != "Snakkos" {
		t.Errorf("Code LUNCHTIME should resolve to Snakkos, got %q err=%v", name, err)
	}
	// Duplicate name and duplicate code both fail (UNIQUE constraints)
	if err := CreateTeam("Snakkos", "OTHERCODE"); err == nil {
		t.Errorf("Expected duplicate team name to fail")
	}
	if err := CreateTeam("Other", "LUNCHTIME"); err == nil {
		t.Errorf("Expected duplicate code to fail")
	}
	// Case-insensitive name collision detection
	if exists, _ := TeamExists("snakkos"); !exists {
		t.Errorf("TeamExists should match case-insensitively")
	}

	// Users join teams (by name), not codes
	pub, _ := CreateUser("trial", "trial@example.com", "h", "#fff", PublicTeam)
	t1, _ := CreateUser("team1", "t1@example.com", "h", "#fff", "Snakkos")
	t2, _ := CreateUser("team2", "t2@example.com", "h", "#fff", "Snakkos")
	perm, _ := CreateUser("perm", "perm@example.com", "h", "#fff", "")

	// Team listing includes codes and user counts
	teams, err := ListTeams()
	if err != nil {
		t.Fatalf("ListTeams failed: %v", err)
	}
	counts := map[string]int{}
	codes := map[string]string{}
	for _, row := range teams {
		counts[row.Name] = row.UserCount
		codes[row.Name] = row.Code
	}
	if counts[PublicTeam] != 1 || counts["Snakkos"] != 2 || codes["Snakkos"] != "LUNCHTIME" {
		t.Errorf("Team rows mismatch: counts=%v codes=%v", counts, codes)
	}

	// Rotating the code keeps members and team resolution intact
	if err := UpdateTeamCode("Snakkos", "NEWCODE-7"); err != nil {
		t.Fatalf("UpdateTeamCode failed: %v", err)
	}
	if _, err := GetTeamNameByCode("LUNCHTIME"); err == nil {
		t.Errorf("Old code should no longer resolve")
	}
	if name, _ := GetTeamNameByCode("NEWCODE-7"); name != "Snakkos" {
		t.Errorf("New code should resolve to Snakkos, got %q", name)
	}
	if err := UpdateTeamCode("NoSuchTeam", "WHATEVER"); err == nil {
		t.Errorf("Expected error rotating code of missing team")
	}

	// Team visibility by name
	team, err := ListTeamUsers("Snakkos")
	if err != nil {
		t.Fatalf("ListTeamUsers failed: %v", err)
	}
	if len(team) != 2 || team[0].ID != t1.ID || team[1].ID != t2.ID {
		t.Errorf("Expected team1+team2 on Snakkos, got %d users", len(team))
	}

	// GetUserTeam round-trips
	if tm, _ := GetUserTeam(pub.ID); tm != PublicTeam {
		t.Errorf("Expected PUBLIC team for trial user, got %q", tm)
	}
	if tm, _ := GetUserTeam(perm.ID); tm != "" {
		t.Errorf("Expected empty team for perm user, got %q", tm)
	}

	// Deleting a team keeps its users (and their membership string)
	if err := DeleteTeam("Snakkos"); err != nil {
		t.Fatalf("DeleteTeam failed: %v", err)
	}
	if _, err := GetUserByID(t1.ID); err != nil {
		t.Errorf("Team user should survive team deletion: %v", err)
	}
	if tm, _ := GetUserTeam(t1.ID); tm != "Snakkos" {
		t.Errorf("User should keep team name after deletion, got %q", tm)
	}
	if err := DeleteTeam("Snakkos"); err == nil {
		t.Errorf("Expected error deleting a missing team")
	}
}

func TestCleanExpiredTrialUsers(t *testing.T) {
	if err := InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	expired, _ := CreateUser("oldtrial", "old@example.com", "h", "#fff", PublicTeam)
	fresh, _ := CreateUser("newtrial", "new@example.com", "h", "#fff", PublicTeam)
	perm, _ := CreateUser("keeper", "keep@example.com", "h", "#fff", "")

	// The expired trial user owns a board with the fresh user as a member
	board, err := CreateBoard("Trial Board", "", expired.ID)
	if err != nil {
		t.Fatalf("CreateBoard failed: %v", err)
	}
	if err := AddBoardMember(board.ID, fresh.ID, "member"); err != nil {
		t.Fatalf("AddBoardMember failed: %v", err)
	}

	// Backdate the expired user past the trial window; backdate perm too to
	// prove only PUBLIC accounts are wiped
	backdated := time.Now().Add(-TrialDuration - time.Hour)
	for _, id := range []string{expired.ID, perm.ID} {
		if _, err := DB.Exec(`UPDATE users SET created_at = ? WHERE id = ?`, backdated, id); err != nil {
			t.Fatalf("Failed to backdate user: %v", err)
		}
	}

	removed, err := CleanExpiredTrialUsers()
	if err != nil {
		t.Fatalf("CleanExpiredTrialUsers failed: %v", err)
	}
	if removed != 1 {
		t.Errorf("Expected 1 user removed, got %d", removed)
	}

	if _, err := GetUserByID(expired.ID); err == nil {
		t.Errorf("Expected expired trial user to be deleted")
	}
	if _, err := GetUserByID(fresh.ID); err != nil {
		t.Errorf("Fresh trial user should survive: %v", err)
	}
	if _, err := GetUserByID(perm.ID); err != nil {
		t.Errorf("Permanent user should survive even when old: %v", err)
	}
	if _, err := GetBoard(board.ID); err == nil {
		t.Errorf("Expected expired user's board to be deleted")
	}
}

func TestUpdateUserPasswordAndDeleteSessions(t *testing.T) {
	if err := InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize in-memory DB: %v", err)
	}
	defer DB.Close()

	user, err := CreateUser("pwuser", "pw@example.com", "old_hash", "#6366f1", "")
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
