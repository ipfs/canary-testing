package task

import (
	"container/heap"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestQueueSortsPriorityAndTime(t *testing.T) {
	earlier := time.Now()
	later := earlier.Add(time.Minute)

	// Add tasks to the queue with different priorities
	tq := make(taskQueue, 0)
	for i := 0; i <= 10; i++ {
		tsk := Task{
			Priority: i,
			Created:  earlier,
		}
		heap.Push(&tq, &tsk)
	}
	// Add a few more with a later timestamp
	for i := 0; i <= 10; i++ {
		tsk := Task{
			Priority: i,
			Created:  later,
		}
		heap.Push(&tq, &tsk)
	}

	// verify the sort is by piority (high->low) and time (oldest->newest)
	head := heap.Pop(&tq).(*Task)
	for len(tq) > 0 {
		next := heap.Pop(&tq).(*Task)
		t.Logf("priority %d > %d?", head.Priority, next.Priority)
		if head.Priority != next.Priority {
			assert.Greater(t, head.Priority, next.Priority, "should prefer higher priority tasks")
		} else {
			t.Logf("timestamp %s before %s?", head.Created, next.Created)
			assert.True(t, head.Created.Before(next.Created), "should prefer older tasks")
		}
		head = next
	}
}
