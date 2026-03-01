package db

import (
	"log"
	"time"
)

// maxRetries is the number of attempts for a transient-error retry.
const maxRetries = 3

// retry re-runs fn up to maxRetries times when it returns a transient DB error.
// Application-level errors (ErrNotFound, ErrDuplicate) are returned immediately
// without retrying, because they are not caused by connection issues.
func retry[T any](label string, fn func() (T, error)) (T, error) {
	var result T
	var err error
	for i := 0; i < maxRetries; i++ {
		result, err = fn()
		// Success or application-level error → stop immediately
		if err == nil || err == ErrNotFound || err == ErrDuplicate {
			return result, err
		}
		// Transient error → log and retry after a short backoff
		if i < maxRetries-1 {
			log.Printf("[db] %s transient error (attempt %d/%d): %v", label, i+1, maxRetries, err)
			time.Sleep(time.Duration(150*(i+1)) * time.Millisecond)
		}
	}
	log.Printf("[db] %s failed after %d attempts: %v", label, maxRetries, err)
	return result, err
}
