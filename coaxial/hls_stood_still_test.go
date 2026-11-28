package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A live HLS session's playlist stops moving at the provider (the session is stuck there; its address still
// answers 200 with the same playlist). The player waits, gives up and tunes the channel again. Straight at the
// provider that opens a new session, and the picture comes back. Through Coaxial the player was handed the
// remembered session again, with the same stopped playlist, at every new tune: it never recovered until it had
// stayed away for a minute. A fresh ask for a channel whose playlist has stood still opens it again; reloads of
// the playlist never do, however long it stands still.
func TestHLSTunedAgainAfterPlaylistStoodStillGetsANewSession(t *testing.T) {
	h, own, list := standstill(t)
	old := stoodStill
	stoodStill = 1.5 // (15 s, shortened; the stuck playlist has 1 s pieces, so three of them is 3 s)
	defer func() { stoodStill = old }()
	const dev = "192.0.2.1"
	if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
		t.Fatalf("tune: %d %v", st, err)
	}
	// A player asks twice as it tunes, and a second device may join: neither opens the channel again.
	if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 || h.opens.Load() != 1 {
		t.Fatalf("a second ask right after the tune: status %d, %v, %d opens at the provider, want 200 and 1", st, err, h.opens.Load())
	}
	h.stuck.Store(true)
	var had []byte
	for i := 0; i < 3; i++ { // the player reloads its playlist and it does not move
		st, data, _ := devGet(h.base, dev, list, 10*time.Second)
		if st != 200 || !bytes.Contains(data, []byte("MEDIA-SEQUENCE:500")) {
			t.Fatalf("reload %d of the playlist that stands still: status %d, playlist %q", i+1, st, data)
		}
		had = data
		time.Sleep(400 * time.Millisecond)
	}
	// A new tune inside the limit (an ordinary pause): the session it had, the same playlist byte for byte.
	if st, data, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 || !bytes.Equal(data, had) || h.opens.Load() != 1 {
		t.Fatalf("a new tune %d s into a standstill: status %d, %v, %d opens (want 1), playlist\n%q\nwant the one it had\n%q", 1, st, err, h.opens.Load(), data, had)
	}
	for i := 0; i < 6; i++ { // it goes on reloading, past the limit: reloads alone never open the channel again
		if st, data, _ := devGet(h.base, dev, list, 10*time.Second); st != 200 || !bytes.Contains(data, []byte("MEDIA-SEQUENCE:500")) {
			t.Fatalf("reload of the playlist that stands still: status %d, playlist %q", st, data)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if n := h.opens.Load(); n != 1 {
		t.Fatalf("reloads of a playlist that stood still opened the channel again (%d opens)", n)
	}
	// The player gives up and tunes the channel again, past the limit.
	st, data, err := devGet(h.base, dev, own, 10*time.Second)
	if err != nil || st != 200 {
		t.Fatalf("the new tune: %d %v", st, err)
	}
	if bytes.Contains(data, []byte("MEDIA-SEQUENCE:500")) || h.opens.Load() != 2 {
		t.Errorf("the player tuned again and was handed the stopped playlist back (%d open(s) at the provider, want 2):\n%s", h.opens.Load(), data)
	}
	// And the new session is the one its reloads read from now on.
	if st, data, _ := devGet(h.base, dev, list, 10*time.Second); st != 200 || bytes.Contains(data, []byte("MEDIA-SEQUENCE:500")) {
		t.Errorf("the reload after the new tune: status %d, playlist %q, want the new session's", st, data)
	}
}

// A playlist that keeps moving: a player that tunes a healthy channel again (or a second device that joins it)
// shares the one session, as before.
func TestHLSTunedAgainOnAMovingPlaylistKeepsItsSession(t *testing.T) {
	h := newNewestHLS(t)
	h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:1,\np%d.ts\n#EXTINF:1,\np%d.ts\n", 100+n, 100+n, 101+n)
		return true
	}
	const own, list = "/live/tv/easy/44.m3u8", "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
	if st, _, err := devGet(h.base, "192.0.2.1", own, 10*time.Second); err != nil || st != 200 {
		t.Fatalf("tune: %d %v", st, err)
	}
	for i := 0; i < 8; i++ {
		devGet(h.base, "192.0.2.1", list, 10*time.Second)
		time.Sleep(500 * time.Millisecond)
	}
	for _, dev := range []string{"192.0.2.1", "192.0.2.2"} { // the same device tunes again; another joins
		if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
			t.Fatalf("%s asks for the channel: %d %v", dev, st, err)
		}
	}
	if n := h.opens.Load(); n != 1 {
		t.Errorf("a channel whose playlist kept moving was opened %d times at the provider, want 1", n)
	}
}

// A standstill inside the limit with a second device on the session: one device's new start does not take the
// session from under the other.
func TestHLSStartingOverWithAnotherDeviceOnTheSessionKeepsIt(t *testing.T) {
	h, own, list := standstill(t)
	for _, dev := range []string{"192.0.2.1", "192.0.2.2"} {
		if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
			t.Fatalf("%s tunes: %d %v", dev, st, err)
		}
	}
	h.stuck.Store(true)
	for i := 0; i < 5; i++ {
		devGet(h.base, "192.0.2.1", list, 10*time.Second)
		devGet(h.base, "192.0.2.2", list, 10*time.Second)
		time.Sleep(400 * time.Millisecond)
	}
	if st, _, err := devGet(h.base, "192.0.2.1", own, 10*time.Second); err != nil || st != 200 {
		t.Fatalf("the new start: %d %v", st, err)
	}
	if n := h.opens.Load(); n != 1 {
		t.Errorf("one device's new start opened the channel again (%d opens) with another device on the session", n)
	}
}

type standstillHLS struct {
	*newestHLS
	stuck atomic.Bool
}

// standstill: a provider whose playlists have 1 s pieces and move at every ask, until stuck: then the first
// session's stands still.
func standstill(t *testing.T) (*standstillHLS, string, string) {
	h := &standstillHLS{newestHLS: newNewestHLS(t)}
	h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
		if h.stuck.Load() && strings.Contains(r.URL.Path, "/tok1/") {
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:500\n#EXTINF:1,\np500.ts\n#EXTINF:1,\np501.ts\n")
			return true
		}
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:1,\np%d.ts\n#EXTINF:1,\np%d.ts\n", 100+n, 100+n, 101+n)
		return true
	}
	return h, "/live/tv/easy/44.m3u8", "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
}
