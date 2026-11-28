package main

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The tests of this package replay providers at one tenth of real time, so the wait before an unchanged live
// playlist is looked at again (unchangedWait, 1 s) is a tenth too, unless a test sets its own.
func init() { unchangedWait = 0.1 }

func setUnchangedWait(t *testing.T, s float64) {
	old := unchangedWait
	unchangedWait = s
	t.Cleanup(func() { unchangedWait = old })
}

var ucSeqRe = regexp.MustCompile(`#EXT-X-MEDIA-SEQUENCE:(\d+)`)

func ucSeq(t *testing.T, data []byte) string {
	t.Helper()
	m := ucSeqRe.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no MEDIA-SEQUENCE in %q", data)
	}
	return string(m[1])
}

// countingPlayer: a player that gives a live stream up the way one widely used engine does. It reloads the
// playlist every `every` after an answer that brought nothing new, at once after one that did, and after
// `limit` answers in a row that brought nothing new it gives the stream up. Returns the most such answers it
// counted in a row and whether it gave up, playing until `until` seconds after the provider started.
func countingPlayer(t *testing.T, p *realProv, dev string, every time.Duration, limit int, until float64) (most int, gaveUp bool) {
	t.Helper()
	last, run := ucSeq(t, tuneIn(t, p, dev)), 0
	for time.Since(p.began).Seconds() < until {
		st, data, err := devGet(p.base, dev, listAddr, 10*time.Second)
		if err != nil || st != 200 {
			t.Fatalf("reload: status %d, %v", st, err)
		}
		if seq := ucSeq(t, data); seq != last {
			last, run = seq, 0
			continue
		}
		if run++; run > most {
			most = run
		}
		if run >= limit {
			return most, true
		}
		time.Sleep(every)
	}
	return most, false
}

// The freeze this guards against. A provider's live playlist moves in lumps and now and then stands still for
// 12 to 15 s on 4 s pieces: nothing is wrong, the next lump is late. A player that counts reloads bringing
// nothing new (asked 2 s apart) and gives up at the seventh did so inside such a pause, opened the channel
// again, and where the stream had changed meanwhile (the sound going from 5.1 to stereo at a break) it took
// the reopened stream for another one and stopped for good. Every answer it had been given was correct, and
// answered at once, which is what ran its count out. With the unchanged playlist looked at once more before it
// is answered, the same player rides every such pause out. (One tenth of the time. Proven to bite: with
// unchangedWait 0 this player gives up in the 14 s and 15.4 s pauses.)
func TestCountingPlayerRidesOutAnOrdinaryPause(t *testing.T) {
	for _, pause := range []float64{1.24, 1.32, 1.4, 1.54} {
		t.Run(fmt.Sprintf("%.1f s", pause*10), func(t *testing.T) {
			p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.62}, still: [2]float64{1.25, 1.25 + pause - 0.62}})
			most, gaveUp := countingPlayer(t, p, "192.0.2.31", 200*time.Millisecond, 7, 1.25+pause+1.3)
			t.Logf("a %.1f s pause: at most %d reloads in a row brought nothing new", pause*10, most)
			if gaveUp {
				t.Errorf("a player that gives up after 7 reloads with nothing new gave up in an ordinary %.1f s pause", pause*10)
			}
			if most > 5 {
				t.Errorf("%d reloads in a row brought nothing new in a %.1f s pause; the player gives up at 7, and 5 leaves it room", most, pause*10)
			}
			if n := p.opens.Load(); n != 1 {
				t.Errorf("an ordinary pause opened the channel %d times at the provider, want 1 (the tune)", n)
			}
		})
	}
}

// The other direction: a channel that has really stopped is not hidden from the player. Its count runs out,
// later than it did, and the player gives up as it would straight at the provider.
func TestCountingPlayerStillGivesUpOnAChannelThatStopped(t *testing.T) {
	p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.62}, still: [2]float64{1.25, 60}})
	began := time.Now()
	_, gaveUp := countingPlayer(t, p, "192.0.2.31", 200*time.Millisecond, 7, 8)
	if !gaveUp {
		t.Fatalf("a player was kept reloading a channel that had stopped for good")
	}
	if took := time.Since(began).Seconds() - 1.25; took > 3.2 {
		t.Errorf("the player's count ran out %.1f s (x10) into a channel that stopped; want within about 19 s (7 reloads, each a wait longer)", took*10)
	}
}

// What must never change: a playlist that has something new for the device is answered at once, and so is a
// device's first look at a playlist, whoever else was handed it before. Real seconds here: the wait is the real
// one (1 s), and no answer below may come anywhere near it.
func TestChangedPlaylistIsAnsweredAtOnce(t *testing.T) {
	setUnchangedWait(t, 1.0)
	h := newNewestHLS(t)
	var seq atomic.Int64
	seq.Store(100)
	h.playHook = func(n int, w http.ResponseWriter, r *http.Request) bool {
		s := seq.Load()
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:%d\n#EXTINF:4,\np%d.ts\n#EXTINF:4,\np%d.ts\n", s, s, s+1)
		return true
	}
	const own, list = "/live/tv/easy/44.m3u8", "/__ch/live/tv/easy/44.m3u8/__index.m3u8"
	ask := func(dev, addr string) (string, time.Duration) {
		t.Helper()
		began := time.Now()
		st, data, err := devGet(h.base, dev, addr, 10*time.Second)
		if err != nil || st != 200 {
			t.Fatalf("%s asks %s: status %d, %v", dev, addr, st, err)
		}
		return ucSeq(t, data), time.Since(began)
	}
	if _, took := ask("192.0.2.1", own); took > 500*time.Millisecond {
		t.Errorf("the tune took %.2f s", took.Seconds())
	}
	for i := 0; i < 5; i++ { // the playlist has moved at every reload: none waits
		seq.Add(1)
		if got, took := ask("192.0.2.1", list); took > 400*time.Millisecond || got != fmt.Sprint(seq.Load()) {
			t.Errorf("a reload of a playlist that had moved took %.2f s and brought %s (want %d, at once)", took.Seconds(), got, seq.Load())
		}
	}
	// A second device joins: its first look at the playlist the first device already has is answered at once.
	if _, took := ask("192.0.2.2", own); took > 500*time.Millisecond {
		t.Errorf("a second device's tune took %.2f s: it waited on what another device had been handed", took.Seconds())
	}
	// The first device reloads with nothing new: that one waits, and brings a move made meanwhile.
	go func() { time.Sleep(300 * time.Millisecond); seq.Add(1) }()
	before := h.plAsks.Load()
	got, took := ask("192.0.2.1", list)
	if took < 900*time.Millisecond || took > 1600*time.Millisecond {
		t.Errorf("a reload with nothing new was answered after %.2f s, want about 1 s (unchangedWait)", took.Seconds())
	}
	if got != fmt.Sprint(seq.Load()) {
		t.Errorf("the second look brought %s; the playlist had moved to %d meanwhile", got, seq.Load())
	}
	if n := h.plAsks.Load() - before; n != 2 {
		t.Errorf("a reload with nothing new asked the provider for the playlist %d times, want 2 (the look and one more)", n)
	}
	if n := h.opens.Load(); n != 1 {
		t.Errorf("the channel was opened %d times at the provider, want 1", n)
	}
}

// A live playlist changes only by pieces added at its end and dropped from its start (RFC 8216, 6.2.1): the
// address of a piece a player was given never changes while its number is listed, through a standstill, a
// reload past any limit, or anything else done here. (For a while a reload past a limit opened a new session
// behind the player: the same numbers, other addresses.)
func TestPieceAddressesNeverChangeUnderAReloadingPlayer(t *testing.T) {
	p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.62}, still: [2]float64{1.25, 4.5}})
	lineRe := regexp.MustCompile(`(?m)^[^#\s][^\r\n]*$`)
	addr := map[string]string{} // piece number (its file name) -> the address it was first given at
	data := tuneIn(t, p, "192.0.2.31")
	for time.Since(p.began).Seconds() < 5.5 {
		for _, line := range lineRe.FindAllString(string(data), -1) {
			name := line[strings.LastIndex(line, "/")+1:]
			if was, ok := addr[name]; ok && was != line {
				t.Fatalf("piece %s was first given at\n  %s\nand later at\n  %s", name, was, line)
			}
			addr[name] = line
		}
		st, d, err := devGet(p.base, "192.0.2.31", listAddr, 10*time.Second)
		if err != nil || st != 200 {
			t.Fatalf("reload: status %d, %v", st, err)
		}
		data = d
		time.Sleep(150 * time.Millisecond)
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a 32 s standstill under a player that only reloads opened the channel %d times at the provider, want 1", n)
	}
}

// The provider is asked for the playlist at most twice for one reload by a player, and a player that hangs up
// during the wait is not asked for again at all.
func TestUnchangedReloadAsksTheProviderOnceMore(t *testing.T) {
	setUnchangedWait(t, 0.5)
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.2}, still: [2]float64{0.1, 60}})
	tuneIn(t, p, "192.0.2.31")
	before := p.plAsks.Load()
	const reloads = 4
	for i := 0; i < reloads; i++ {
		if st, _, err := devGet(p.base, "192.0.2.31", listAddr, 10*time.Second); err != nil || st != 200 {
			t.Fatalf("reload: status %d, %v", st, err)
		}
	}
	if n := p.plAsks.Load() - before; n != 2*reloads {
		t.Errorf("%d reloads with nothing new asked the provider for the playlist %d times, want %d (each once more)", reloads, n, 2*reloads)
	}
	before = p.plAsks.Load()
	if _, _, err := devGet(p.base, "192.0.2.31", listAddr, 150*time.Millisecond); err == nil {
		t.Fatalf("the player that hangs up after 150 ms was answered: the wait did not happen")
	}
	time.Sleep(700 * time.Millisecond)
	if n := p.plAsks.Load() - before; n != 1 {
		t.Errorf("a player that hung up during the wait had the provider asked %d times, want 1 (nothing after it left)", n)
	}
	if st, data, err := devGet(p.base, "192.0.2.31", listAddr, 10*time.Second); err != nil || st != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
		t.Errorf("the reload after a hang-up: status %d, %v, %q", st, err, data)
	}
}

// The second look brings an error (the server is busy for a moment): the first answer stands. The wait never
// turns a playlist the player could have had into a failure.
func TestUnchangedReloadWhoseSecondLookFailsKeepsTheFirst(t *testing.T) {
	var fail atomic.Bool
	p := newRealProv(t, realCfg{target: 4, gaps: []float64{6.2}, still: [2]float64{0.1, 60}, playlist: func(tok, n int, w http.ResponseWriter, r *http.Request) bool {
		if fail.Load() && n%2 == 1 { // (every second ask from here on: the second look of each reload)
			http.Error(w, "busy", 503)
			return true
		}
		return false
	}})
	tuneIn(t, p, "192.0.2.31")
	if p.plAsks.Load()%2 != 0 { // (the next ask is odd, the one after it even)
		devGet(p.base, "192.0.2.99", listAddr, 10*time.Second)
	}
	p.plAsks.Add(1) // (so the first look of the next reload is even, its second look odd)
	fail.Store(true)
	for i := 0; i < 3; i++ {
		st, data, err := devGet(p.base, "192.0.2.31", listAddr, 10*time.Second)
		if err != nil || st != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
			t.Fatalf("reload %d, whose second look answered 503: status %d, %v, %q; want the playlist of the first look", i+1, st, err, data)
		}
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("a second look that failed opened the channel %d times at the provider, want 1", n)
	}
}

// HLS that Coaxial cuts from a provider's MPEG-TS (a ts provider) stands still the same way: MPEG-TS comes in
// lumps, and no piece can be cut while nothing comes. A playlist the device already has waits for the next
// piece, up to unchangedWait; a new piece ends the wait at once, and a device's first look never waits. Real
// seconds.
func TestHLSFromTSUnchangedPlaylistWaitsForTheNextPiece(t *testing.T) {
	setUnchangedWait(t, 1.0)
	_, base := newTSHLSProv(t, 40, 14)
	const dev = "192.0.2.71"
	status, data, _, at := tsHLSGet(t, dev, base+"/live/tv/easy/7.m3u8")
	if status != 200 || !bytes.Contains(data, []byte("#EXTINF")) {
		t.Fatalf("the channel asked for as HLS: %d %q", status, data)
	}
	newest := func(d []byte) string {
		_, names := tsHLSList(string(d))
		if len(names) == 0 {
			t.Fatalf("a playlist with no pieces: %q", d)
		}
		return names[len(names)-1]
	}
	last, waited, early, moves := newest(data), 0, 0, 0
	for end := time.Now().Add(14 * time.Second); time.Now().Before(end); {
		began := time.Now()
		st, d, _, _ := tsHLSGet(t, dev, at)
		took := time.Since(began)
		if st != 200 {
			t.Fatalf("reload: status %d", st)
		}
		if n := newest(d); n != last {
			last, moves = n, moves+1
			if took < 900*time.Millisecond {
				early++ // (a piece was cut during the wait, or since the last look: answered then)
			}
			continue // (something new: a player reloads at once)
		}
		if took < 900*time.Millisecond || took > 1600*time.Millisecond {
			t.Errorf("a reload that brought nothing new was answered after %.2f s, want about 1 s", took.Seconds())
		}
		waited++
		time.Sleep(time.Second)
	}
	if moves < 2 || waited < 2 {
		t.Fatalf("the stand-in did not produce the case: %d moves, %d reloads with nothing new", moves, waited)
	}
	if early == 0 {
		t.Errorf("no new piece was handed over before the full wait in %d moves: a piece cut during the wait must end it", moves)
	}
	began := time.Now() // another device's first look at the same playlist: at once
	if st, _, _, _ := tsHLSGet(t, "192.0.2.72", base+"/live/tv/easy/7.m3u8"); st != 200 || time.Since(began) > 600*time.Millisecond {
		t.Errorf("a second device's first playlist: status %d after %.2f s, want 200 at once", st, time.Since(began).Seconds())
	}
}
