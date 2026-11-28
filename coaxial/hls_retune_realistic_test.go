package main

import (
	"testing"
	"time"
)

// Some players ask for the channel's own address at every playlist refresh (they follow the redirect each time
// instead of keeping the address it leads to; see "once a minute" in playlistRequest). On a provider that states
// 4 s pieces while its playlist moves every 6.3 s, playback is healthy and nothing is stuck: the channel is
// opened at the provider once. Each further open would end the session (newest wins), its pieces would answer
// 404, and some providers refuse reopening for minutes. (One tenth of the time.)
func TestRealisticPlayerAskingTheChannelAtEveryRefreshOpensItOnce(t *testing.T) {
	p := newRealProv(t, realCfg{target: 0.4, gaps: []float64{0.63}})
	for time.Since(p.began).Seconds() < 12 {
		tuneIn(t, p, "192.0.2.1") // the channel's own address, then the playlist it leads to
		time.Sleep(150 * time.Millisecond)
	}
	if n := p.opens.Load(); n != 1 {
		t.Errorf("healthy playback (4 s stated, a move every 6.3 s) by a player that asks for the channel at every refresh opened it %d times at the provider, want 1", n)
	}
}
