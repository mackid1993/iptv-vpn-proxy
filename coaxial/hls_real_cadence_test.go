package main

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cadenceHLS: a provider whose live playlists state target and move every cadence seconds (time-scaled from a
// live evening: one tenth). stuckAfter > 0: the first session stops moving after that many seconds.
func cadenceHLS(t *testing.T, target, cadence, stuckAfter float64) *newestHLS {
	h, _ := cadenceHLSSeen(t, target, cadence, stuckAfter)
	return h
}

// cadenceHLSSeen: cadenceHLS, and the time a player was first handed the last playlist of the session that
// stalls (the last move the player saw).
func cadenceHLSSeen(t *testing.T, target, cadence, stuckAfter float64) (*newestHLS, *atomic.Int64) {
	h := newNewestHLS(t)
	began := time.Now()
	var seen atomic.Int64
	served := map[string]int{}
	var mu sync.Mutex
	var stuckTok atomic.Value // (the session in use when the stall begins: that one stops, a new one moves on)
	h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
		el := time.Since(began).Seconds()
		if stuckAfter > 0 && el > stuckAfter && stuckTok.Load() == nil {
			stuckTok.Store(fmt.Sprintf("/tok%d/", h.latest.Load()))
		}
		seq := 1000 + int(el/cadence)
		if tok, _ := stuckTok.Load().(string); tok != "" && strings.Contains(r.URL.Path, tok) {
			seq = 1000 + int(stuckAfter/cadence)
		}
		tok := tokRe.FindString(r.URL.Path)
		mu.Lock()
		if seq > served[tok] {
			served[tok] = seq
			if st, _ := stuckTok.Load().(string); st == "" || st == tok {
				seen.Store(time.Now().UnixNano())
			}
		}
		mu.Unlock()
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:%g\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:%g,\np%d.ts\n", target, seq, cadence, seq)
		return true
	}
	return h, &seen
}

var tokRe = regexp.MustCompile(`/tok[0-9]+/`)

// play: device dev tunes and reloads every every seconds for d seconds.
func play(t *testing.T, h *newestHLS, dev string, every, d float64) {
	const own, list = "/live/tv/easy/44.m3u8", "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
	if st, _, err := devGet(h.base, dev, own, 10*time.Second); err != nil || st != 200 {
		t.Fatalf("tune: %d %v", st, err)
	}
	for end := time.Now().Add(time.Duration(d * float64(time.Second))); time.Now().Before(end); {
		devGet(h.base, dev, list, 10*time.Second)
		time.Sleep(time.Duration(every * float64(time.Second)))
	}
}

// Healthy playback as one provider sends it: it states 4 s pieces and its playlist moves every 6.3 s, the player
// reloads every 1.5 s. Nothing is stuck, so the channel is opened at the provider once.
func TestHLSStated4sMoving6sOpensOnce(t *testing.T) {
	h := cadenceHLS(t, 0.4, 0.63, 0)
	play(t, h, "192.0.2.1", 0.15, 12)
	if n := h.opens.Load(); n != 1 {
		t.Errorf("healthy playback (4 s stated, a move every 6.3 s) opened the channel %d times at the provider, want 1 (the tune)", n)
	}
}

// A 24/7 channel stating 11 s pieces whose playlist moves every 11.5 s: opened once.
func TestHLSStated11sMoving11sOpensOnce(t *testing.T) {
	h := cadenceHLS(t, 1.1, 1.15, 0)
	play(t, h, "192.0.2.1", 0.2, 8)
	if n := h.opens.Load(); n != 1 {
		t.Errorf("healthy playback (11 s stated, a move every 11.5 s) opened the channel %d times at the provider, want 1 (the tune)", n)
	}
}

// A session that stops moving for good while the channel carries on at the provider (5 s pieces, and 9 s
// pieces): its playlist still answers 200, so it is passed on as it is and the channel is not opened again
// while the player only reloads. (A player that gives up and tunes the channel again gets a new session:
// TestHLSTunedAgainAfterPlaylistStoodStillGetsANewSession.)
func TestHLSSessionThatStopsIsNotReplacedBehindThePlayer(t *testing.T) {
	for _, c := range []struct {
		name                   string
		target, cadence, every float64
	}{
		{"5 s pieces", 0.5, 0.5, 0.2}, {"9 s pieces", 0.9, 0.85, 0.2},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := cadenceHLS(t, c.target, c.cadence, 4*c.cadence)
			play(t, h, "192.0.2.1", c.every, 4*c.cadence+3)
			if n := h.opens.Load(); n != 1 {
				t.Errorf("a session that stopped (%s) had its channel opened %d times at the provider, want 1 (the tune)", c.name, n)
			}
		})
	}
}

// The real pace of a channel on a live evening, replayed (one tenth of the time): the seconds between moves of
// its playlist, as Coaxial logged them, with a lone player reloading. Normal playback: the channel is opened once.
func TestHLSRealPaceReplayedOpensOnce(t *testing.T) {
	for _, c := range []struct {
		name   string
		target float64
		gaps   []float64
	}{
		{"stated 4 s", 0.4, []float64{0.66, 0.64, 0.42, 0.64, 0.64, 0.65, 0.63, 0.65, 0.44, 0.65, 0.65, 0.63, 0.65, 0.43, 0.65, 0.64, 0.65, 0.63, 0.65, 0.43, 0.65, 0.63, 0.65, 0.63, 0.65, 0.42, 0.64, 0.64, 0.65, 0.64, 0.44, 0.63, 0.65, 0.64, 0.65, 0.64, 0.44, 0.80, 0.64, 0.66}},
		{"stated 11 s", 1.1, []float64{1.26, 0.72, 1.26, 1.27, 0.77, 1.30, 0.75, 1.31, 0.81, 1.33, 1.11, 1.10, 1.10, 1.10, 1.10, 1.10, 1.10, 1.10, 1.10}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newNewestHLS(t)
			began := time.Now()
			at := []float64{0}
			for _, g := range c.gaps {
				at = append(at, at[len(at)-1]+g)
			}
			h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
				el, seq := time.Since(began).Seconds(), 0
				for seq < len(at)-1 && at[seq+1] <= el {
					seq++
				}
				fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:%g\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:%g,\np%d.ts\n", c.target, 1000+seq, c.target, seq)
				return true
			}
			play(t, h, "192.0.2.1", 0.2, at[len(at)-1])
			if n := h.opens.Load(); n != 1 {
				t.Errorf("the real pace of a channel (%s) opened it %d times at the provider, want 1", c.name, n)
			}
		})
	}
}

// The other way in: a player that asks for the channel's own address again every few seconds (some do at each
// refresh) while its playlist moves at a provider's healthy pace: the channel is opened once.
func TestHLSRetunesAtRealPaceOpenOnce(t *testing.T) {
	for _, c := range []struct {
		name            string
		target, cadence float64
	}{{"stated 11 s, moving every 12 s", 1.1, 1.2}, {"stated 4 s, moving every 6.6 s", 0.4, 0.66}} {
		t.Run(c.name, func(t *testing.T) {
			h := cadenceHLS(t, c.target, c.cadence, 0)
			const own, list = "/live/tv/easy/44.m3u8", "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
			devGet(h.base, "192.0.2.1", own, 10*time.Second)
			for i, end := 0, time.Now().Add(24*time.Second); time.Now().Before(end); i++ {
				p := list
				if i%3 == 2 { // (about every 0.5 s: its own address again)
					p = own
				}
				devGet(h.base, "192.0.2.1", p, 10*time.Second)
				time.Sleep(200 * time.Millisecond)
			}
			if n := h.opens.Load(); n != 1 {
				t.Errorf("a player asking its channel's own address again at a healthy pace (%s) had it opened %d times at the provider in 4 minutes (x10), want 1", c.name, n)
			}
		})
	}
}

// A channel stating 5 s pieces whose playlist moves every 6.3 s and now and then stands still 10.3 to 11.1 s in
// healthy playback (the real pace of one, one tenth of the time). It was opened again at each long pause for a
// while; it is opened once.
func TestHLSShortPieceChannelWithLongPausesOpensOnce(t *testing.T) {
	h := newNewestHLS(t)
	gaps := []float64{0.64, 0.63, 0.65, 0.64, 1.03, 0.63, 0.65, 0.64, 0.66, 1.05, 0.63, 0.64, 0.65, 0.63, 1.11,
		0.64, 0.63, 0.65, 0.64, 1.06, 0.63, 0.65, 0.64, 0.63, 1.08, 0.64, 0.65, 0.63}
	at := []float64{0}
	for _, g := range gaps {
		at = append(at, at[len(at)-1]+g)
	}
	began := time.Now()
	h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
		el, seq := time.Since(began).Seconds(), 0
		for seq < len(at)-1 && at[seq+1] <= el {
			seq++
		}
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:0.5\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:0.5,\np%d.ts\n", 1000+seq, seq)
		return true
	}
	play(t, h, "192.0.2.1", 0.2, at[len(at)-1])
	if n := h.opens.Load(); n != 1 {
		t.Errorf("a 5 s channel whose healthy pauses reach 11 s was opened %d times at the provider, want 1 (the tune)", n)
	}
}
