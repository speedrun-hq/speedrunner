package fulfiller

import (
	"context"
	"errors"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/speedrun-hq/speedrunner/pkg/models"
	"github.com/stretchr/testify/assert"
)

// MockJobQueue is a test implementation of a job queue
type MockJobQueue struct {
	mu     sync.Mutex
	jobs   []models.Intent
	closed bool
}

func NewMockJobQueue() *MockJobQueue {
	return &MockJobQueue{
		jobs:   make([]models.Intent, 0),
		closed: false,
	}
}

func (m *MockJobQueue) Push(intent models.Intent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("queue is closed")
	}

	m.jobs = append(m.jobs, intent)
	return nil
}

func (m *MockJobQueue) Pop() (models.Intent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return models.Intent{}, errors.New("queue is closed")
	}

	if len(m.jobs) == 0 {
		return models.Intent{}, errors.New("no jobs available")
	}

	intent := m.jobs[0]
	m.jobs = m.jobs[1:]
	return intent, nil
}

func (m *MockJobQueue) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// JobQueue defines the interface for a job queue
type JobQueue interface {
	Push(intent models.Intent) error
	Pop() (models.Intent, error)
	Close() error
}

// FulfillFunc is a function type for intent fulfillment
type FulfillFunc func(intent models.Intent) error

// worker processes intents from the queue and calls the fulfillment function
func worker(ctx context.Context, queue JobQueue, fulfillFunc FulfillFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			intent, err := queue.Pop()
			if err != nil {
				// No jobs available or queue closed
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Process the intent
			err = fulfillFunc(intent)
			if err != nil {
				// Handle error - in a real implementation, we would add to retry queue
				// For this test, we simply log the error to the console
				log.Printf("Error processing intent %s: %v", intent.ID, err)
			}
		}
	}
}

// MockService is a simplified test double for the Service
type MockService struct {
	mu               sync.Mutex
	fulfilledIntents []models.Intent
	failedIntents    []models.Intent
	shouldFail       bool
}

func (m *MockService) fulfillIntent(intent models.Intent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		m.failedIntents = append(m.failedIntents, intent)
		return errors.New("mock fulfillment failure")
	}

	m.fulfilledIntents = append(m.fulfilledIntents, intent)
	return nil
}

// TestWorkerProcessesJobs tests that the worker correctly processes jobs from the queue
func TestWorkerProcessesJobs(t *testing.T) {
	// Skip in short mode - this is a longer running test
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create test context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock queue and service
	queue := NewMockJobQueue()
	mockService := &MockService{}

	// Add some test intents to the queue
	testIntents := []models.Intent{
		{
			ID:               "intent1",
			SourceChain:      1,
			DestinationChain: 2,
			Amount:           "1000",
			Recipient:        "0x1111111111111111111111111111111111111111",
		},
		{
			ID:               "intent2",
			SourceChain:      1,
			DestinationChain: 2,
			Amount:           "2000",
			Recipient:        "0x2222222222222222222222222222222222222222",
		},
	}

	for _, intent := range testIntents {
		err := queue.Push(intent)
		assert.NoError(t, err)
	}

	// Start worker in a goroutine
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Create a worker that calls our mock service's fulfillIntent method
		worker(ctx, queue, func(intent models.Intent) error {
			return mockService.fulfillIntent(intent)
		})
	}()

	// Allow time for worker to process jobs
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop worker
	cancel()

	// Wait for worker to complete
	wg.Wait()

	// Verify intents were processed
	mockService.mu.Lock()
	defer mockService.mu.Unlock()
	assert.Equal(t, len(testIntents), len(mockService.fulfilledIntents),
		"Worker should have processed all intents")

	// Verify the correct intents were processed
	intentMap := make(map[string]bool)
	for _, intent := range mockService.fulfilledIntents {
		intentMap[intent.ID] = true
	}
	for _, intent := range testIntents {
		assert.True(t, intentMap[intent.ID], "Intent %s should have been processed", intent.ID)
	}
}

// TestWorkerHandlesErrors tests that the worker correctly handles fulfillment errors
func TestWorkerHandlesErrors(t *testing.T) {
	// Skip in short mode - this is a longer running test
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create test context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create mock queue and service
	queue := NewMockJobQueue()
	mockService := &MockService{shouldFail: true}

	// Add a test intent to the queue
	testIntent := models.Intent{
		ID:               "intent1",
		SourceChain:      1,
		DestinationChain: 2,
		Amount:           "1000",
		Recipient:        "0x1111111111111111111111111111111111111111",
	}

	err := queue.Push(testIntent)
	assert.NoError(t, err)

	// Start worker in a goroutine
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Create a worker that calls our mock service's fulfillIntent method
		worker(ctx, queue, func(intent models.Intent) error {
			return mockService.fulfillIntent(intent)
		})
	}()

	// Allow time for worker to process jobs
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop worker
	cancel()

	// Wait for worker to complete
	wg.Wait()

	// Verify intent was marked as failed
	mockService.mu.Lock()
	defer mockService.mu.Unlock()
	assert.Equal(t, 1, len(mockService.failedIntents),
		"Worker should have marked the intent as failed")
	assert.Equal(t, testIntent.ID, mockService.failedIntents[0].ID,
		"The correct intent should be marked as failed")
}
