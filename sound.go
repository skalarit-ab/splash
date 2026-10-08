package splash

import (
	"math"
	"sync"

	"github.com/marrasen/gunim/audio"
)

// The sting is made in code, in E major, in step with the logo: a soft
// pop as the diamond lands, rising into B4, a mallet note from each side
// as the wedges open, and two bells as the glow swells. Then a quick
// arpeggio runs up through the chord, left to right with the letters,
// and lands on the chord as the logo settles, a small lift. It is 1.5
// seconds long, its last stingFade fading to silence, so it has gone
// before the intro has. Its loudest moment peaks at stingPeak of full
// scale.
const (
	stingLength = 1.5
	stingFade   = 0.35
	stingPeak   = 0.28
)

// When each sound starts, in seconds from the intro's first frame. They
// sit a little after the cues they go with, at the moment each motion
// is plainest: the diamond near its fullest, the wedges near their
// places.
const (
	popAt   = 0.07
	leftAt  = 0.19
	rightAt = 0.23
	bellsAt = 0.30
	// arpAt is when the arpeggio's first note starts, as the first
	// letter rises, and arpGap how long after one note the next starts.
	arpAt  = 0.36
	arpGap = 0.045
	// chordAt is when the chord lands, as the last letters settle.
	chordAt = arpAt + float64(len(arpeggio))*arpGap + 0.03
)

// The notes, in hertz: B4 for the diamond, E5 and G#5 for the wedges,
// and B5 and E6 for the glow.
const (
	noteB4  = 493.88
	noteE5  = 659.26
	noteGs5 = 830.61
	noteB5  = 987.77
	noteE6  = 1318.51
	noteE4  = 329.63
	noteGs4 = 415.30
)

// arpeggio is the run up through E major, E4 to B5; chord is the chord
// it lands on, E4, G#4, B4 and E5.
var (
	arpeggio = [...]float64{noteE4, noteGs4, noteB4, noteE5, noteGs5, noteB5}
	chord    = [...]float64{noteE4, noteGs4, noteB4, noteE5}
)

var (
	stingMu sync.Mutex
	stings  = map[int]*audio.Clip{}
)

// stingAt returns the sting made for a mixer running at rate frames a
// second, making it the first time.
func stingAt(rate int) *audio.Clip {
	stingMu.Lock()
	defer stingMu.Unlock()
	if c := stings[rate]; c != nil {
		return c
	}
	c := audio.NewClip(stingSamples(rate))
	stings[rate] = c
	return c
}

// stingSamples renders the sting at rate frames a second, as stereo
// samples, left then right.
func stingSamples(rate int) []float32 {
	hz := float64(rate)
	n := int(stingLength * hz)
	s := make([]float32, 2*n)
	loudest := 0.0
	for i := range n {
		t := float64(i) / hz
		l, r := stingAtTime(t)
		// The tail fades to silence, so nothing clicks off.
		fade := min(1, float64(n-i)/(stingFade*hz))
		fade *= fade
		l, r = l*fade, r*fade
		s[2*i], s[2*i+1] = float32(l), float32(r)
		loudest = max(loudest, math.Abs(l), math.Abs(r))
	}
	if loudest > 0 {
		k := float32(stingPeak / loudest)
		for i := range s {
			s[i] *= k
		}
	}
	return s
}

// stingAtTime is the sting t seconds in, left and right.
func stingAtTime(t float64) (l, r float64) {
	// Every tone's oscillator runs from the sting's start, whenever the
	// tone itself starts, so two tones of one pitch add up rather than
	// cancel: the arpeggio's E5 and G#5 sound over the wedges' still
	// ringing, and its B4 over the pop's.
	//
	// The diamond: a pop that rises into its note, in the middle.
	mid := pop(t, popAt)
	l, r = mid, mid
	// The wedges, each from its own side.
	addPanned(&l, &r, 0.45*mallet(t, leftAt, noteE5), -0.55)
	addPanned(&l, &r, 0.4*mallet(t, rightAt, noteGs5), 0.55)
	// The glow: two bells, a touch apart in each ear, so they shimmer
	// wide.
	l += 0.22*bell(t, bellsAt, noteB5, 1) + 0.14*bell(t, bellsAt+0.03, noteE6, wide)
	r += 0.22*bell(t, bellsAt, noteB5, wide) + 0.14*bell(t, bellsAt+0.03, noteE6, 1)
	// The lift: the arpeggio, from left to right as the letters come
	// in, and the chord it lands on, wide and in the middle.
	for i, f := range arpeggio {
		pan := -0.45 + 0.9*float64(i)/float64(len(arpeggio)-1)
		addPanned(&l, &r, 0.24*pluck(t, arpAt+float64(i)*arpGap, f), pan)
	}
	for i, f := range chord {
		// Each note a touch sharp in one ear and flat in the other, so the
		// chord is wide.
		d := 1 + 0.0012*float64(i%2*2-1)
		l += 0.21 * swell(t, chordAt, f, d)
		r += 0.21 * swell(t, chordAt, f, 1/d)
	}
	return l, r
}

// wide is how far a tone is tuned up in one ear, to sound wide.
const wide = 1.0015

// phase is the phase at time t of a tone of pitch f started at on and
// tuned by detune. It starts in step with the pitch's oscillator, which
// runs from the sting's start, and drifts from there by the detune
// alone.
func phase(t, on, f, detune float64) float64 {
	return 2 * math.Pi * (f*on + f*detune*(t-on))
}

// pluck is a short, soft plucked note struck at on, at time t: a sine
// and its octave, struck quickly and dying away.
func pluck(t, on, f float64) float64 {
	u := t - on
	if u < 0 {
		return 0
	}
	w := phase(t, on, f, 1)
	att := min(u/0.003, 1)
	return att * (math.Sin(w) + 0.3*math.Sin(2*w)*math.Exp(-u/0.06)) * math.Exp(-u/0.18)
}

// swell is a note of the chord landing at on, tuned by detune, at time
// t: a warm tone,
// a sine with its octave and fifth above, that swells in over 50 ms
// and dies away over half a second.
func swell(t, on, f, detune float64) float64 {
	u := t - on
	if u < 0 {
		return 0
	}
	w := phase(t, on, f, detune)
	att := 1 - math.Exp(-u/0.05)
	return att * (math.Sin(w) + 0.25*math.Sin(2*w) + 0.08*math.Sin(3*w)) * math.Exp(-u/0.45)
}

// addPanned adds v to l and r, placed at pan, from -1 at the left to 1
// at the right, as loud wherever it is.
func addPanned(l, r *float64, v, pan float64) {
	a := (pan + 1) * math.Pi / 4
	*l += v * math.Cos(a) * math.Sqrt2
	*r += v * math.Sin(a) * math.Sqrt2
}

// pop is a soft, round pop started at on, at time t: a sine that rises
// from low into B4, with a touch of the octave above, and rings a
// moment.
func pop(t, on float64) float64 {
	u := t - on
	if u < 0 {
		return 0
	}
	// The pitch glides from 0.55 of the note up to it over about 30 ms;
	// the phase is the glide's integral, and settles onto B4's from the
	// sting's start.
	const glide = 0.03
	f0 := noteB4 * 0.55
	ph := 2 * math.Pi * (noteB4*t + (noteB4-f0)*glide*math.Exp(-u/glide))
	att := min(u/0.004, 1)
	return att * (math.Sin(ph) + 0.15*math.Sin(2*ph)) * (0.6*math.Exp(-u/0.05) + 0.4*math.Exp(-u/0.25))
}

// mallet is a note struck on a soft wooden bar, as a marimba's, at on,
// at time t: its tone, and the bar's fourth partial, which dies sooner.
func mallet(t, on, f float64) float64 {
	u := t - on
	if u < 0 {
		return 0
	}
	w := phase(t, on, f, 1)
	att := min(u/0.004, 1)
	return att * (math.Sin(w)*math.Exp(-u/0.28) + 0.22*math.Sin(3.93*w)*math.Exp(-u/0.04))
}

// bell is a small, soft bell started at on, tuned by detune, at time t: a sine with a
// bright partial, swelling in over 25 ms.
func bell(t, on, f, detune float64) float64 {
	u := t - on
	if u < 0 {
		return 0
	}
	w := phase(t, on, f, detune)
	att := min(u/0.025, 1)
	return att * (math.Sin(w)*math.Exp(-u/0.3) + 0.25*math.Sin(2.76*w)*math.Exp(-u/0.09))
}
