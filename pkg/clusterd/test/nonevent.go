package test

import (
	"log"

	"github.com/rook/rook/pkg/clusterd"
)

func WaitForEvents(leader clusterd.ServiceLeader) {
	// add a placeholder event to the queue. When it is dequeued we know the rest of the events have completed.
	e := newNonEvent()
	leader.Events() <- e

	// wait for the Name() method to be called on the nonevent, which means it was dequeued
	log.Printf("waiting for event queue to empty")
	<-e.signaled
	log.Printf("event queue is empty")
}

// Empty event for testing
type nonEvent struct {
	signaled   chan bool
	nameCalled bool
}

func newNonEvent() *nonEvent {
	return &nonEvent{signaled: make(chan bool)}
}

func (e *nonEvent) Name() string {
	if !e.nameCalled {
		e.nameCalled = true
		e.signaled <- true
	}
	return "nonevent"
}
func (e *nonEvent) Context() *clusterd.Context {
	return nil
}
