package main

import (
	"net/http"
	"testing"
	"time"
)

// rbHangUp: the provider's own address takes the connection and closes it without an answer (a dropped
// connection on the way through the tunnel).
func rbHangUp(w http.ResponseWriter) {
	if c, _, err := w.(http.Hijacker).Hijack(); err == nil {
		c.Close()
	}
}

// A session the provider ended (its playlist answers 404: a newer one won) is opened again once, as designed;
// when that reopen gets no answer (the provider's address drops the connection while it restarts the channel),
// the next reloads do not each open the channel again: once per renewGap, as a player retrying its own playlist
// at most every 8 s would (hls.js maxRetryDelayMs).
func TestRealisticReopenWithNoAnswerIsNotTriedAtEveryReload(t *testing.T) {
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.3}, front: func(n int, w http.ResponseWriter, r *http.Request) bool {
		if n > 1 {
			rbHangUp(w)
			return true
		}
		return false
	}})
	tuneIn(t, p, "192.0.2.1")
	p.latest.Store(99) // (the session is gone at the provider)
	asks := 0
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(500 * time.Millisecond) {
		devGet(p.base, "192.0.2.1", listAddr, 10*time.Second)
		asks++
	}
	if n := p.opens.Load(); n > 2 {
		t.Errorf("%d reloads of a session that is gone, whose reopen got no answer, opened the channel %d times; want 2 (the tune and one reopen in renewGap)", asks, n)
	}
}
