package limit

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var day1 = time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

func consumeN(t *testing.T, l *Limiter, ip string, n int, now time.Time) {
	t.Helper()
	for i := range n {
		if err := l.Consume(ip, now); err != nil {
			t.Fatalf("request %d from %s: %v", i+1, ip, err)
		}
	}
}

func TestClientCapPerDay(t *testing.T) {
	l := New([]byte("secret"), 3, 100)
	consumeN(t, l, "203.0.113.7", 3, day1)
	if err := l.Consume("203.0.113.7", day1); !errors.Is(err, ErrClientLimit) {
		t.Fatalf("got %v, want ErrClientLimit", err)
	}
	if err := l.Consume("203.0.113.8", day1); err != nil {
		t.Fatalf("other client: %v", err)
	}
}

func TestGlobalCapAcrossClients(t *testing.T) {
	l := New([]byte("secret"), 10, 4)
	consumeN(t, l, "203.0.113.1", 2, day1)
	consumeN(t, l, "203.0.113.2", 2, day1)
	if err := l.Consume("203.0.113.3", day1); !errors.Is(err, ErrGlobalLimit) {
		t.Fatalf("got %v, want ErrGlobalLimit", err)
	}
}

func TestDeniedRequestsDoNotUseGlobalBudget(t *testing.T) {
	l := New([]byte("secret"), 1, 2)
	consumeN(t, l, "203.0.113.1", 1, day1)
	for range 5 {
		_ = l.Consume("203.0.113.1", day1)
	}
	if err := l.Consume("203.0.113.2", day1); err != nil {
		t.Fatalf("got %v, want the second global slot", err)
	}
}

func TestNewUTCDayResetsCounts(t *testing.T) {
	l := New([]byte("secret"), 1, 1)
	consumeN(t, l, "203.0.113.7", 1, day1)
	lateSameDay := time.Date(2026, 11, 10, 23, 59, 0, 0, time.UTC)
	if err := l.Consume("203.0.113.7", lateSameDay); err == nil {
		t.Fatal("expected a limit later the same UTC day")
	}
	nextDay := time.Date(2026, 11, 11, 0, 0, 1, 0, time.UTC)
	if err := l.Consume("203.0.113.7", nextDay); err != nil {
		t.Fatalf("next day: %v", err)
	}
}

func TestIPv6AddressesShareTheirSlash64(t *testing.T) {
	l := New([]byte("secret"), 2, 100)
	consumeN(t, l, "2001:db8:1:2::1", 1, day1)
	consumeN(t, l, "2001:db8:1:2:ffff::9", 1, day1)
	if err := l.Consume("2001:db8:1:2:abcd::1", day1); !errors.Is(err, ErrClientLimit) {
		t.Fatalf("got %v, want ErrClientLimit for the same /64", err)
	}
	if err := l.Consume("2001:db8:1:3::1", day1); err != nil {
		t.Fatalf("other /64: %v", err)
	}
}

func TestUnparsableAddressesShareOneBucket(t *testing.T) {
	l := New([]byte("secret"), 1, 100)
	consumeN(t, l, "garbage", 1, day1)
	if err := l.Consume("", day1); !errors.Is(err, ErrClientLimit) {
		t.Fatalf("got %v, want ErrClientLimit", err)
	}
}

func TestRawAddressesAreNotKept(t *testing.T) {
	l := New([]byte("secret"), 5, 100)
	consumeN(t, l, "203.0.113.7", 1, day1)
	for key := range l.counts {
		if strings.Contains(key, "203.0.113.7") {
			t.Fatalf("raw address kept in key %q", key)
		}
	}
}

func TestConcurrentConsumeNeverExceedsGlobalCap(t *testing.T) {
	l := New([]byte("secret"), 1000, 50)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Go(func() {
			ip := "203.0.113." + string(rune('0'+i%10))
			if l.Consume(ip, day1) == nil {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 50 {
		t.Fatalf("allowed %d, want exactly 50", allowed.Load())
	}
}

func TestAddressesWithPortsKeepTheirOwnBucket(t *testing.T) {
	l := New([]byte("secret"), 1, 100)
	consumeN(t, l, "203.0.113.7:5678", 1, day1)
	if err := l.Consume("203.0.113.8:5678", day1); err != nil {
		t.Fatalf("other client with port: %v", err)
	}
	if err := l.Consume("203.0.113.7", day1); !errors.Is(err, ErrClientLimit) {
		t.Fatalf("got %v, want the same bucket with and without port", err)
	}
	if err := l.Consume("[2001:db8:1:2::1]:443", day1); err != nil {
		t.Fatalf("bracketed IPv6 with port: %v", err)
	}
}

func TestSweepForgetsYesterdayWithoutNewRequests(t *testing.T) {
	l := New([]byte("secret"), 5, 100)
	consumeN(t, l, "203.0.113.7", 1, day1)
	l.Sweep(day1.Add(time.Hour))
	if len(l.counts) != 1 {
		t.Fatalf("swept during the same day: %d keys", len(l.counts))
	}
	l.Sweep(time.Date(2026, 11, 11, 0, 5, 0, 0, time.UTC))
	if len(l.counts) != 0 {
		t.Fatalf("kept %d keys after the day ended", len(l.counts))
	}
}
