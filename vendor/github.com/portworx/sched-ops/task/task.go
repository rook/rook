package task

import (
	"context"
	"fmt"
	"log"
	"time"
)

//TODO: export the type: type Task func() (string, error)

// ErrTimedOut is returned when an operation times out
// Is this type used anywhere? If not we can get rid off it in favor context.DeadlineExceeded
type ErrTimedOut struct {
	// Reason is the reason for the timeout
	Reason string
}

func (e *ErrTimedOut) Error() string {
	errString := "timed out performing task."
	if len(e.Reason) > 0 {
		errString = fmt.Sprintf("%s, Error was: %s", errString, e.Reason)
	}

	return errString
}

// DoRetryWithTimeout performs given task with given timeout and timeBeforeRetry
// TODO(stgleb): In future I would like to add context as a first param to this function
// so calling code can cancel task.
func DoRetryWithTimeout(t func() (interface{}, bool, error), timeout, timeBeforeRetry time.Duration) (interface{}, error) {
	// Use context.Context as a standard go way of timeout and cancellation propagation amount goroutines.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultChan := make(chan interface{})
	errChan := make(chan error)

	go func() {
		for {
			select {
			case <-ctx.Done():

				if ctx.Err() != nil {
					errChan <- ctx.Err()
				}

				return
			default:
				out, retry, err := t()
				if err == nil {
					resultChan <- out
					return
				}

				if err != nil && !retry {
					errChan <- err
					return
				}

				log.Printf("%v Next retry in: %v", err, timeBeforeRetry)
				time.Sleep(timeBeforeRetry)
			}
		}
	}()

	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errChan:
		if err == context.DeadlineExceeded {
			return nil, &ErrTimedOut{
				Reason: err.Error(),
			}
		}

		return nil, err
	}
}
