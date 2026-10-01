package config

import (
	"sync"
	"testing"
)

// TestFilterSensitiveData_Concurrent exercises the lazy sensitive-data cache
// from many goroutines at once, as happens when two agent turns run in
// parallel. Before the fix the cache pointer was created without
// synchronization, so under -race this reported a data race and without
// -race a goroutine could see a nil replacer and panic.
func TestFilterSensitiveData_Concurrent(t *testing.T) {
	const secret = "sk-concurrent-secret-12345"
	for round := 0; round < 50; round++ {
		cfg := &Config{}
		cfg.ModelList = SecureModelList{
			&ModelConfig{ModelName: "m", APIKeys: SimpleSecureStrings(secret)},
		}
		cfg.Tools.FilterSensitiveData = true
		cfg.Tools.FilterMinLength = 8

		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan string, 16)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if got := cfg.FilterSensitiveData("key=" + secret); got != "key=[FILTERED]" {
					errs <- got
				}
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		for got := range errs {
			t.Fatalf("round %d: got %q, want key=[FILTERED]", round, got)
		}
	}
}
