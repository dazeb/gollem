package openai

import (
	"path/filepath"
	"sync"
	"testing"
)

// TestSaveCredentials_AtomicUnderConcurrency: concurrent savers and
// readers must never observe an invalid (empty/partial) credentials
// file. The previous os.WriteFile implementation truncated in place;
// two processes refreshing tokens concurrently produced a 0-byte
// auth.json in production.
func TestSaveCredentials_AtomicUnderConcurrency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	creds := &Credentials{AccessToken: "tok", RefreshToken: "ref"}
	if err := SaveCredentialsTo(creds, path); err != nil {
		t.Fatal(err)
	}

	var writers sync.WaitGroup
	stop := make(chan struct{})
	errs := make(chan error, 64)

	for range 4 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for range 200 {
				if err := SaveCredentialsTo(creds, path); err != nil {
					errs <- err
					return
				}
			}
		}()
	}

	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := LoadCredentialsFrom(path)
			if err != nil {
				errs <- err
				return
			}
			if got.AccessToken != "tok" {
				errs <- nil
				return
			}
		}
	}()

	writers.Wait()
	close(stop)
	<-readerDone

	select {
	case err := <-errs:
		t.Fatalf("invalid credentials observed under concurrency: %v", err)
	default:
	}
}
