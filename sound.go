package splash

import (
	"math"
	"sync"

	"github.com/marrasen/gunim/audio"
)

// The sting is made in code, in E major, in step with the logo: a soft
// pop as the diamond lands, rising into B4, a mallet note from each side as the wedges
// open, and two bells as the glow swells. It is 0.9 seconds long, and
// its loudest moment peaks at stingPeak of full scale.
const (
	stingLength = 0.9
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
)

// The notes, in hertz: B4 for the diamond, E5 and G#5 for the wedges,
// and B5 and E6 for the glow.
const (
	noteB4  = 493.88
	noteE5  = 659.26
	noteGs5 = 830.61
	noteB5  = 987.77
	noteE6  = 1318.51
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
		// The last 60 ms fade, so nothing clicks off.
		fade := min(1, float64(n-i)/(0.06*hz))
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
	// The diamond: a pop that rises into its note, in the middle.
	mid := pop(t - popAt)
	l, r = mid, mid
	// The wedges, each from its own side.
	addPanned(&l, &r, 0.45*mallet(t-leftAt, noteE5), -0.55)
	addPanned(&l, &r, 0.4*mallet(t-rightAt, noteGs5), 0.55)
	// The glow: two bells, a touch apart in each ear, so they shimmer
	// wide.
	l += 0.22*bell(t-bellsAt, noteB5) + 0.14*bell(t-bellsAt-0.03, noteE6*1.0015)
	r += 0.22*bell(t-bellsAt, noteB5*1.0015) + 0.14*bell(t-bellsAt-0.03, noteE6)
	return l, r
}

// addPanned adds v to l and r, placed at pan, from -1 at the left to 1
// at the right, as loud wherever it is.
func addPanned(l, r *float64, v, pan float64) {
	a := (pan + 1) * math.Pi / 4
	*l += v * math.Cos(a) * math.Sqrt2
	*r += v * math.Sin(a) * math.Sqrt2
}

// pop is a soft, round pop t seconds after it starts: a sine that
// rises from low into B4, with a touch of the octave above, and rings
// a moment.
func pop(t float64) float64 {
	if t < 0 {
		return 0
	}
	// The pitch glides from 0.55 of the note up to it over about 30 ms;
	// the phase is the glide's integral.
	const glide = 0.03
	f0 := noteB4 * 0.55
	ph := 2 * math.Pi * (noteB4*t + (noteB4-f0)*glide*math.Exp(-t/glide))
	att := min(t/0.004, 1)
	return att * (math.Sin(ph) + 0.15*math.Sin(2*ph)) * (0.6*math.Exp(-t/0.05) + 0.4*math.Exp(-t/0.25))
}

// mallet is a note struck on a soft wooden bar, as a marimba's, t
// seconds after it is struck: its tone, and the bar's fourth partial,
// which dies sooner.
func mallet(t, f float64) float64 {
	if t < 0 {
		return 0
	}
	w := 2 * math.Pi * f * t
	att := min(t/0.004, 1)
	return att * (math.Sin(w)*math.Exp(-t/0.28) + 0.22*math.Sin(3.93*w)*math.Exp(-t/0.04))
}

// bell is a small, soft bell, t seconds after it starts: a sine with a
// bright partial, swelling in over 25 ms.
func bell(t, f float64) float64 {
	if t < 0 {
		return 0
	}
	w := 2 * math.Pi * f * t
	att := min(t/0.025, 1)
	return att * (math.Sin(w)*math.Exp(-t/0.3) + 0.25*math.Sin(2.76*w)*math.Exp(-t/0.09))
}
