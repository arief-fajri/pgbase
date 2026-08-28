package subscriptions

import "testing"

// TestDefaultClientSendNonBlockingWhenFull verifies RT-1: once the buffered
// channel is full, Send drops the message instead of blocking (slow-client
// isolation without per-message goroutines).
func TestDefaultClientSendNonBlockingWhenFull(t *testing.T) {
	client := NewDefaultClient()

	// fill the buffer exactly (channel cap - no reader)
	for i := 0; i < clientChannelBufferSize; i++ {
		client.Send(Message{Name: "sub"})
	}

	// the buffer is now full; further sends must NOT block and must drop
	client.Send(Message{Name: "dropped"})

	if got := len(client.Channel()); got != clientChannelBufferSize {
		t.Fatalf("expected buffer to stay at capacity (%d) after a drop, got %d", clientChannelBufferSize, got)
	}

	// a drained buffer accepts a new message again
	<-client.Channel()
	client.Send(Message{Name: "after-drain"})
	if got := len(client.Channel()); got != clientChannelBufferSize {
		t.Fatalf("expected the buffer to accept a message after draining, got length %d", got)
	}
}

// TestDefaultClientSendDiscardedIsNoop verifies Send on a discarded client does
// not panic and does not queue.
func TestDefaultClientSendDiscardedIsNoop(t *testing.T) {
	client := NewDefaultClient()
	client.Discard()
	client.Discard() // safe multiple times

	client.Send(Message{Name: "ignored"})
	if got := len(client.Channel()); got != 0 {
		t.Fatalf("expected no messages queued on a discarded client, got %d", got)
	}
}
