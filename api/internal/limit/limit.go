package limit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"sync"
	"time"
)

var (
	ErrClientLimit = errors.New("daily limit for this client reached")
	ErrGlobalLimit = errors.New("global daily limit reached")
)

type Limiter struct {
	secret    []byte
	perClient int
	global    int

	mu     sync.Mutex
	day    string
	total  int
	counts map[string]int
}

func New(secret []byte, perClient, global int) *Limiter {
	return &Limiter{secret: secret, perClient: perClient, global: global, counts: map[string]int{}}
}

func (l *Limiter) Consume(ip string, now time.Time) error {
	day := now.UTC().Format(time.DateOnly)
	key := l.key(day, bucket(ip))

	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollover(day)
	if l.counts[key] >= l.perClient {
		return ErrClientLimit
	}
	if l.total >= l.global {
		return ErrGlobalLimit
	}
	l.counts[key]++
	l.total++
	return nil
}

func (l *Limiter) Sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollover(now.UTC().Format(time.DateOnly))
}

func (l *Limiter) rollover(day string) {
	if day != l.day {
		l.day, l.total, l.counts = day, 0, map[string]int{}
	}
}

func (l *Limiter) key(day, bucket string) string {
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(day + "|" + bucket))
	return hex.EncodeToString(mac.Sum(nil))
}

func bucket(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		addrPort, portErr := netip.ParseAddrPort(ip)
		if portErr != nil {
			return "unparsable"
		}
		addr = addrPort.Addr()
	}
	addr = addr.Unmap()
	if addr.Is6() {
		prefix, _ := addr.Prefix(64)
		return prefix.String()
	}
	return addr.String()
}
