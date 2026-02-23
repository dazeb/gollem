package team

import (
	"sync"
	"testing"
)

func TestTaskBoard_Create(t *testing.T) {
	tb := NewTaskBoard()

	id1 := tb.Create("Task 1", "Description 1")
	id2 := tb.Create("Task 2", "Description 2")

	if id1 == id2 {
		t.Error("expected unique IDs")
	}

	task, err := tb.Get(id1)
	if err != nil {
		t.Fatal(err)
	}
	if task.Subject != "Task 1" {
		t.Errorf("expected 'Task 1', got %q", task.Subject)
	}
	if task.Status != TaskPending {
		t.Errorf("expected pending, got %q", task.Status)
	}
}

func TestTaskBoard_Update(t *testing.T) {
	tb := NewTaskBoard()
	id := tb.Create("Task", "Desc")

	err := tb.Update(id, WithStatus(TaskInProgress), WithOwner("alice"))
	if err != nil {
		t.Fatal(err)
	}

	task, _ := tb.Get(id)
	if task.Status != TaskInProgress {
		t.Errorf("expected in_progress, got %q", task.Status)
	}
	if task.Owner != "alice" {
		t.Errorf("expected owner 'alice', got %q", task.Owner)
	}
}

func TestTaskBoard_UpdateNotFound(t *testing.T) {
	tb := NewTaskBoard()
	err := tb.Update("999", WithStatus(TaskCompleted))
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestTaskBoard_List(t *testing.T) {
	tb := NewTaskBoard()
	tb.Create("Task 1", "")
	tb.Create("Task 2", "")
	tb.Create("Task 3", "")

	tasks := tb.List()
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestTaskBoard_Claim(t *testing.T) {
	tb := NewTaskBoard()
	id := tb.Create("Task", "Desc")

	if err := tb.Claim(id, "bob"); err != nil {
		t.Fatal(err)
	}

	task, _ := tb.Get(id)
	if task.Owner != "bob" {
		t.Errorf("expected owner 'bob', got %q", task.Owner)
	}
	if task.Status != TaskInProgress {
		t.Errorf("expected in_progress, got %q", task.Status)
	}

	// Claiming again should fail.
	if err := tb.Claim(id, "alice"); err == nil {
		t.Error("expected error claiming already-owned task")
	}
}

func TestTaskBoard_ClaimBlocked(t *testing.T) {
	tb := NewTaskBoard()
	id1 := tb.Create("Blocker", "")
	id2 := tb.Create("Blocked", "")

	tb.Update(id2, WithAddBlockedBy(id1))

	if err := tb.Claim(id2, "bob"); err == nil {
		t.Error("expected error claiming blocked task")
	}

	// Complete the blocker.
	tb.Update(id1, WithStatus(TaskCompleted))

	// Now should succeed.
	if err := tb.Claim(id2, "bob"); err != nil {
		t.Fatalf("expected claim to succeed after blocker completed: %v", err)
	}
}

func TestTaskBoard_Available(t *testing.T) {
	tb := NewTaskBoard()
	id1 := tb.Create("Available", "")
	id2 := tb.Create("Owned", "")
	id3 := tb.Create("Blocked", "")

	tb.Update(id2, WithOwner("alice"))
	tb.Update(id3, WithAddBlockedBy(id1))

	avail := tb.Available()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available task, got %d", len(avail))
	}
	if avail[0].ID != id1 {
		t.Errorf("expected task %q to be available, got %q", id1, avail[0].ID)
	}
}

func TestTaskBoard_Delete(t *testing.T) {
	tb := NewTaskBoard()
	id1 := tb.Create("Task 1", "")
	id2 := tb.Create("Task 2", "")

	tb.Update(id2, WithAddBlockedBy(id1))

	if err := tb.Delete(id1); err != nil {
		t.Fatal(err)
	}

	// Deleted task should not be found.
	if _, err := tb.Get(id1); err == nil {
		t.Error("expected error for deleted task")
	}

	// Blocked-by reference should be cleaned up.
	task2, _ := tb.Get(id2)
	if len(task2.BlockedBy) != 0 {
		t.Errorf("expected BlockedBy to be empty after deleting blocker, got %v", task2.BlockedBy)
	}
}

func TestTaskBoard_Metadata(t *testing.T) {
	tb := NewTaskBoard()
	id := tb.Create("Task", "")

	tb.Update(id, WithMetadata(map[string]any{"key": "value", "num": 42}))

	task, _ := tb.Get(id)
	if task.Metadata["key"] != "value" {
		t.Errorf("expected metadata key='value', got %v", task.Metadata["key"])
	}

	// Delete a key.
	tb.Update(id, WithMetadata(map[string]any{"key": nil}))
	task, _ = tb.Get(id)
	if _, ok := task.Metadata["key"]; ok {
		t.Error("expected 'key' to be deleted")
	}
	if task.Metadata["num"] != 42 {
		t.Errorf("expected num=42, got %v", task.Metadata["num"])
	}
}

func TestTaskBoard_GetReturnsCopy(t *testing.T) {
	tb := NewTaskBoard()
	id := tb.Create("Task", "Desc")

	task1, _ := tb.Get(id)
	task1.Subject = "Modified"

	task2, _ := tb.Get(id)
	if task2.Subject != "Task" {
		t.Errorf("Get should return copies; original was modified")
	}
}

// TestTaskBoard_AddBlocksReciprocal verifies that WithAddBlocks on task A
// automatically adds A to the blocked task's BlockedBy list.
func TestTaskBoard_AddBlocksReciprocal(t *testing.T) {
	tb := NewTaskBoard()
	id1 := tb.Create("Blocker", "")
	id2 := tb.Create("Blocked", "")

	// Use WithAddBlocks on the blocker to indicate it blocks the other task.
	tb.Update(id1, WithAddBlocks(id2))

	// Task 2 should now have id1 in its BlockedBy.
	task2, _ := tb.Get(id2)
	if len(task2.BlockedBy) != 1 || task2.BlockedBy[0] != id1 {
		t.Fatalf("expected task2.BlockedBy=[%s], got %v", id1, task2.BlockedBy)
	}

	// Task 2 should not be available (it's blocked).
	avail := tb.Available()
	for _, a := range avail {
		if a.ID == id2 {
			t.Error("blocked task should not appear in Available()")
		}
	}

	// Claiming task 2 should fail.
	if err := tb.Claim(id2, "bob"); err == nil {
		t.Error("expected error claiming blocked task")
	}

	// Complete the blocker.
	tb.Update(id1, WithStatus(TaskCompleted))

	// Now task 2 should be available and claimable.
	if err := tb.Claim(id2, "bob"); err != nil {
		t.Fatalf("expected claim to succeed after blocker completed: %v", err)
	}
}

// TestTaskBoard_AddBlockedByReciprocal verifies that WithAddBlockedBy on task B
// automatically adds B to the blocking task's Blocks list.
func TestTaskBoard_AddBlockedByReciprocal(t *testing.T) {
	tb := NewTaskBoard()
	id1 := tb.Create("Blocker", "")
	id2 := tb.Create("Blocked", "")

	// Use WithAddBlockedBy on the blocked task.
	tb.Update(id2, WithAddBlockedBy(id1))

	// Task 1 should now have id2 in its Blocks.
	task1, _ := tb.Get(id1)
	if len(task1.Blocks) != 1 || task1.Blocks[0] != id2 {
		t.Fatalf("expected task1.Blocks=[%s], got %v", id2, task1.Blocks)
	}
}

func TestTaskBoard_ConcurrentAccess(t *testing.T) {
	tb := NewTaskBoard()

	var wg sync.WaitGroup
	// Create tasks concurrently.
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tb.Create("Task", "Desc")
		}()
	}
	wg.Wait()

	tasks := tb.List()
	if len(tasks) != 100 {
		t.Errorf("expected 100 tasks, got %d", len(tasks))
	}

	// Claim tasks concurrently — only one should succeed per task.
	avail := tb.Available()
	if len(avail) == 0 {
		t.Fatal("no available tasks")
	}
	target := avail[0].ID

	var claimWg sync.WaitGroup
	successes := 0
	var mu sync.Mutex
	for i := range 10 {
		claimWg.Add(1)
		go func(owner string) {
			defer claimWg.Done()
			if err := tb.Claim(target, owner); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(string(rune('a' + i)))
	}
	claimWg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 successful claim, got %d", successes)
	}
}
