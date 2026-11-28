package main

import (
	"bytes"
	"testing"
	"time"
)

// A session's playlist stands still at the provider while a player alone on it keeps reloading. Its address
// still answers 200 with the same playlist, so nothing has failed: the player is handed that playlist, as it
// would be straight at the provider, and the channel is not opened again. (For a while it was: a new session
// in the old one's place, which ended the old one and changed the addresses of pieces already listed.)
func TestHLSLoneReloaderOfAPlaylistThatStandsStillKeepsItsSession(t *testing.T) {
	h, own, list := standstill(t)
	const dev = "192.0.2.1"
	if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
		t.Fatalf("tune: %d %v", st, err)
	}
	for i := 0; i < 3; i++ { // it plays: the playlist moves
		devGet(h.base, dev, list, 10*time.Second)
		time.Sleep(200 * time.Millisecond)
	}
	h.stuck.Store(true)
	for i := 0; i < 8; i++ { // the player only reloads, for 4 s and more (the playlist has 1 s pieces)
		st, data, err := devGet(h.base, dev, list, 10*time.Second)
		if err != nil || st != 200 || !bytes.Contains(data, []byte("MEDIA-SEQUENCE:500")) {
			t.Fatalf("reload %d of a playlist that stands still: status %d, %v, %q; want the session's own playlist", i+1, st, err, data)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if n := h.opens.Load(); n != 1 {
		t.Errorf("a player reloading a playlist that stood still had its channel opened %d times at the provider, want 1 (the tune)", n)
	}
}

// The same with a second device on the session.
func TestHLSReloadsOfASharedPlaylistThatStandsStillKeepTheSession(t *testing.T) {
	h, own, list := standstill(t)
	for _, dev := range []string{"192.0.2.1", "192.0.2.2"} {
		if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
			t.Fatalf("%s tunes: %d %v", dev, st, err)
		}
	}
	h.stuck.Store(true)
	for i := 0; i < 6; i++ {
		devGet(h.base, "192.0.2.1", list, 10*time.Second)
		devGet(h.base, "192.0.2.2", list, 10*time.Second)
		time.Sleep(400 * time.Millisecond)
	}
	if n := h.opens.Load(); n != 1 {
		t.Errorf("reloads with two devices on a playlist that stood still opened the channel again (%d opens)", n)
	}
}
