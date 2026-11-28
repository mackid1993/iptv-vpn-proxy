package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A stand-in provider that acts the way real ones were measured to act on a live system, not the way the code
// assumes they do: the playlist states one target duration and moves at another pace (stated 4 s, moving every
// 6.2 to 6.7 s; stated 11 s, moving every 11 to 13.3 s), the whole channel stands still now and then (10 to 16 s
// on a busy channel), every ask at the channel's own address opens a new session and the newest wins (an older
// session's playlist and pieces answer 404 from then on), and any answer can be slow, refused or go quiet without
// closing. Times are in seconds after the provider started; tests scale them where the rules they exercise can be
// scaled (unchangedWait), and keep real seconds where they meet a fixed limit (pieceWait, renewGap).

type realCfg struct {
	target float64    // the target duration its playlists state
	gaps   []float64  // seconds between moves of the playlist, cycled (the provider's real pace)
	still  [2]float64 // the whole channel stands still from still[0] to still[1], then carries on where it was
	size   int        // bytes in a piece (default 2000 MPEG-TS packets)
	// What the provider does instead of its usual answer (true: answered). n counts asks from 1: at the
	// channel's own address, for playlists of all sessions, for this one piece.
	front    func(n int, w http.ResponseWriter, r *http.Request) bool
	playlist func(tok, n int, w http.ResponseWriter, r *http.Request) bool
	piece    func(tok, n int, w http.ResponseWriter, r *http.Request) bool
}

type realProv struct {
	cfg       realCfg
	cdn, prov *fakeProv
	base      string
	began     time.Time
	opens     atomic.Int32 // asks at the channel's own address (each opens a session at the provider)
	latest    atomic.Int32 // the session that wins
	plAsks    atomic.Int32 // playlist asks, all sessions
	mu        sync.Mutex
	stuck     map[int]int64  // a session stuck at the provider: the MEDIA-SEQUENCE it stands still at
	asked     map[string]int // asks of each piece
	data      []byte
}

func newRealProv(t *testing.T, cfg realCfg) *realProv {
	p := &realProv{cfg: cfg, began: time.Now(), stuck: map[int]int64{}, asked: map[string]int{}}
	if cfg.size == 0 {
		cfg.size = 2000 * 188
	}
	p.data = tsFill(cfg.size / 188)
	p.cdn = newFakeProv(t, true, func(fp *fakeProv, w http.ResponseWriter, r *http.Request) bool {
		var tok int
		if _, err := fmt.Sscanf(r.URL.Path, "/hlsr/tok%d/", &tok); err != nil {
			return false
		}
		if int32(tok) != p.latest.Load() { // newest wins: an older session is gone
			http.NotFound(w, r)
			return true
		}
		if strings.HasSuffix(r.URL.Path, ".m3u8") {
			n := int(p.plAsks.Add(1))
			if cfg.playlist != nil && cfg.playlist(tok, n, w, r) {
				return true
			}
			seq := p.seqAt(time.Since(p.began).Seconds())
			p.mu.Lock()
			if s, ok := p.stuck[tok]; ok {
				seq = s
			}
			p.mu.Unlock()
			fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%g\n#EXT-X-MEDIA-SEQUENCE:%d\n", cfg.target, seq)
			for i := int64(0); i < 3; i++ {
				fmt.Fprintf(w, "#EXTINF:%g,\np%d.ts\n", cfg.gaps[0], seq+i)
			}
			return true
		}
		p.mu.Lock()
		p.asked[r.URL.Path]++
		n := p.asked[r.URL.Path]
		p.mu.Unlock()
		if cfg.piece != nil && cfg.piece(tok, n, w, r) {
			return true
		}
		w.Header().Set("Content-Type", "video/mp2t")
		http.ServeContent(w, r, "p.ts", time.Time{}, bytes.NewReader(p.data)) // (Range answered as servers do)
		return true
	})
	p.prov = newFakeProv(t, true, func(fp *fakeProv, w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/live/realU/realP/44.m3u8" {
			return false
		}
		n := int(p.opens.Add(1))
		if cfg.front != nil && cfg.front(n, w, r) {
			return true
		}
		p.latest.Store(int32(n))
		http.Redirect(w, r, fmt.Sprintf("%s/hlsr/tok%d/index.m3u8", p.cdn.srv.URL, n), 302)
		return true
	})
	_, p.base = limFront(t, p.prov, "hls", "", advanced{})
	return p
}

// seqAt: the channel's MEDIA-SEQUENCE el seconds after it started, at its own pace.
func (p *realProv) seqAt(el float64) int64 {
	if from, to := p.cfg.still[0], p.cfg.still[1]; to > from && el >= from {
		el = max(from, el-(to-from))
	}
	n, at := int64(0), 0.0
	for i := 0; ; i++ {
		if at += p.cfg.gaps[i%len(p.cfg.gaps)]; at > el {
			return 1000 + n
		}
		n++
	}
}

// stick: session tok stands still at the provider from now on (its address still answers 200, the same
// playlist every time), while the channel itself carries on.
func (p *realProv) stick(tok int) {
	s := p.seqAt(time.Since(p.began).Seconds())
	p.mu.Lock()
	p.stuck[tok] = s
	p.mu.Unlock()
}

// most: the most asks any one piece had.
func (p *realProv) most() (string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	path, n := "", 0
	for k, v := range p.asked {
		if v > n {
			path, n = k, v
		}
	}
	return path, n
}

// at: sleep until el seconds after the provider started.
func (p *realProv) at(el float64) { time.Sleep(time.Until(p.began.Add(secs(el)))) }

func tsFill(packets int) []byte {
	return bytes.Repeat(append([]byte{0x47}, bytes.Repeat([]byte{0xa5}, 187)...), packets)
}

// waitOr: the server waits d before it answers; false if the asker hung up first.
func waitOr(r *http.Request, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-r.Context().Done():
		return false
	}
}

// trickle: the server sends its answer's heading at once and the body in lumps over `over` (a busy server on a
// slow route: 3 to 8 MB pieces took up to 8.7 s to arrive).
func trickle(w http.ResponseWriter, r *http.Request, data []byte, over time.Duration) {
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.WriteHeader(200)
	const lumps = 87
	step := (len(data)/lumps + 187) / 188 * 188
	for from := 0; from < len(data); from += step {
		if _, err := w.Write(data[from:min(from+step, len(data))]); err != nil {
			return
		}
		w.(http.Flusher).Flush()
		if !waitOr(r, over/lumps) {
			return
		}
	}
}

const (
	ownAddr  = "/live/tv/easy/44.m3u8"
	listAddr = "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
)

func tuneIn(t *testing.T, p *realProv, dev string) []byte {
	t.Helper()
	st, data, err := devGet(p.base, dev, ownAddr, 20*time.Second)
	if err != nil || st != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
		t.Fatalf("%s tunes: status %d, %v, %q", dev, st, err, data)
	}
	return data
}

// A busy video server takes 5 s to answer one playlist reload (at peak time pieces took up to 8.7 s). That is
// slow, not quiet: the reload is answered from its one ask, on the same session, and is not asked again on a new
// connection (which a server that is merely busy only gets slower from).
func TestRealisticSlowReloadIsAskedOnce(t *testing.T) {
	var slow atomic.Int32
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.3}, playlist: func(tok, n int, w http.ResponseWriter, r *http.Request) bool {
		return int32(n) == slow.Load() && !waitOr(r, 5*time.Second)
	}})
	tuneIn(t, p, "192.0.2.1")
	before := p.plAsks.Load()
	slow.Store(before + 1)
	st, data, err := devGet(p.base, "192.0.2.1", listAddr, 20*time.Second)
	if err != nil || st != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
		t.Fatalf("the slow reload: status %d, %v, %q", st, err, data)
	}
	if n := p.plAsks.Load() - before; n != 1 {
		t.Errorf("a reload the server answered after 5 s was asked for %d times, want 1 (slow is not quiet)", n)
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("the channel was opened at the provider %d times, want 1", n)
	}
}

// The provider's own address is slow to send a channel on at peak time (5 s for the redirect). A player tuning
// in waits for it, as it would straight at the provider; the channel is opened once, not again on a new
// connection while the first open is still on its way (a second open ends the first session: newest wins).
func TestRealisticSlowTuneOpensOnce(t *testing.T) {
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.3}, front: func(n int, w http.ResponseWriter, r *http.Request) bool {
		return n == 1 && !waitOr(r, 5*time.Second)
	}})
	began := time.Now()
	tuneIn(t, p, "192.0.2.1")
	if took := time.Since(began); took > 8*time.Second {
		t.Errorf("tuning took %.1f s; the provider answered after 5 s", took.Seconds())
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a tune the provider answered after 5 s opened the channel %d times, want 1", n)
	}
}

// A second device tunes the same channel while the first device's open is still on its way (a slow provider,
// 3 s). Both share the one session: straight at the provider a second open would end the first device's
// session, and the first device's next piece would answer 404.
func TestRealisticSecondDeviceJoiningDuringASlowTuneSharesIt(t *testing.T) {
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.3}, front: func(n int, w http.ResponseWriter, r *http.Request) bool {
		return n == 1 && !waitOr(r, 3*time.Second)
	}})
	var wg sync.WaitGroup
	lists := make([][]byte, 2)
	for i, dev := range []string{"192.0.2.1", "192.0.2.2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i) * time.Second)
			st, data, err := devGet(p.base, dev, ownAddr, 30*time.Second)
			if err != nil || st != 200 {
				t.Errorf("%s tunes: status %d, %v", dev, st, err)
			}
			lists[i] = data
		}()
	}
	wg.Wait()
	if n := p.opens.Load(); n != 1 {
		t.Errorf("two devices tuning one channel while its open was on its way opened it %d times, want 1", n)
	}
	for i, l := range lists { // (each device's piece is still there: nobody ended its session)
		if bytes.Contains(l, []byte("#EXTINF")) {
			if st, _, err := devGet(p.base, fmt.Sprintf("192.0.2.%d", i+1), pieceOf(t, l), 20*time.Second); err != nil || st != 200 {
				t.Errorf("device %d's piece: status %d, %v", i+1, st, err)
			}
		}
	}
}

// A 24/7 channel states 11 s pieces and moves every 13 s (healthy: up to 13.3 s was seen). A reload 3.5 s after
// a move goes quiet on its kept connection and is asked again after pieceWait on a new one, which answers at once
// with the same playlist: 11.5 s since the move, past the 11 s a device alone may see. The standstill is counted
// to the start of the reload, so the quiet connection is not taken for the provider standing still and the
// channel is not opened again (that would end the session, and some providers refuse a reopen for minutes).
func TestRealisticQuietReloadIsNotAStandstill(t *testing.T) {
	var quiet atomic.Int32
	p := newRealProv(t, realCfg{target: 11, gaps: []float64{13}, playlist: func(tok, n int, w http.ResponseWriter, r *http.Request) bool {
		if int32(n) == quiet.Load() {
			<-r.Context().Done()
			return true
		}
		return false
	}})
	tuneIn(t, p, "192.0.2.1")
	p.at(3.5)
	quiet.Store(p.plAsks.Load() + 1)
	st, data, err := devGet(p.base, "192.0.2.1", listAddr, 20*time.Second)
	if err != nil || st != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
		t.Fatalf("the reload asked again on a new connection: status %d, %v, %q", st, err, data)
	}
	if el := time.Since(p.began).Seconds(); el < 11.2 || el > 12.9 {
		t.Fatalf("the reload ended %.1f s after the move; the test needs it past 11 s and before the next move at 13 s", el)
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a reload that went quiet opened the channel again (%d opens at the provider, want 1): the wait on the quiet connection was counted as the provider standing still", n)
	}
}

// The whole channel stands still at the provider for 16 s (a busy channel at peak time). Nothing is wrong with
// the session: the channel is not opened again, for a player alone on it or anyone else. (For a while it was,
// once past a limit: the new session was no further on, as every one on a live system was, and the old one was
// ended for it.) (One tenth of the time: 4 s pieces moving every 4 s, a reload every 0.5 s.)
func TestRealisticWholeChannelStandstillOpensNothing(t *testing.T) {
	p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.4}, still: [2]float64{1.21, 2.41}}) // (last move 1.2, next 2.8)
	tuneIn(t, p, "192.0.2.1")
	for time.Since(p.began).Seconds() < 3.4 {
		if st, _, err := devGet(p.base, "192.0.2.1", listAddr, 10*time.Second); err != nil || st != 200 {
			t.Fatalf("reload: status %d, %v", st, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a 16 s standstill of the whole channel opened it %d times at the provider, want 1 (the tune)", n)
	}
}

// Two devices watch one channel; the provider stands still 12 s on it (a busy channel). Neither device's reloads
// take the session from the other: a new session's pieces have new addresses, and newest wins, so the other
// device's next piece would answer 404. (One tenth of the time.)
func TestRealisticTwoDevicesRideOutABusyStandstill(t *testing.T) {
	p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.4}, still: [2]float64{0.81, 1.61}}) // (last move 0.8, next 2.0)
	tuneIn(t, p, "192.0.2.1")
	tuneIn(t, p, "192.0.2.2")
	var wg sync.WaitGroup
	var last [2]atomic.Value
	for i, every := range []time.Duration{150 * time.Millisecond, 200 * time.Millisecond} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dev := fmt.Sprintf("192.0.2.%d", i+1)
			for time.Since(p.began).Seconds() < 2.6 {
				st, data, err := devGet(p.base, dev, listAddr, 10*time.Second)
				if err != nil || st != 200 {
					t.Errorf("%s reloads: status %d, %v", dev, st, err)
					return
				}
				last[i].Store(data)
				time.Sleep(every)
			}
		}()
	}
	wg.Wait()
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a 12 s standstill with two devices on the session opened the channel %d times at the provider, want 1", n)
	}
	if l, _ := last[0].Load().([]byte); l != nil {
		if st, _, err := devGet(p.base, "192.0.2.1", pieceOf(t, l), 20*time.Second); err != nil || st != 200 {
			t.Errorf("the first device's newest piece after the standstill: status %d, %v", st, err)
		}
	}
}
