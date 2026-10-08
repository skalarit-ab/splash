package splash

import (
	"math"
	"sync"

	"github.com/marrasen/gunim/audio"
)

// The sting is made in code, in E major, in step with the logo. Under
// it all an E major chord starts at the first frame, quiet, and grows
// fuller and brighter as the logo comes together, to its peak as the
// logo settles, then rings out: a small lift. Over it come the chimes:
// a soft pop as the diamond lands, rising into B4, a mallet note from
// each side as the wedges open, two bells as the glow swells, and a
// light run of plucks up through the chord as the letters rise. It is
// 1.5 seconds long, its last stingFade fading to silence, so it has
// gone before the intro has. Its loudest moment peaks at stingPeak of
// full scale.
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
	// peakAt is when the chord is at its fullest, as the logo settles;
	// chordRing is how long it then takes to fall to a third.
	peakAt    = 0.66
	chordRing = 0.32
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
	noteE3  = 164.81
	noteB2  = 123.47
)

// arpeggio is the run of plucks up through E major, E4 to B5; chord is
// the chord, E4, G#4, B4 and E5, and low its tones an octave and more
// below, E3 and B2.
var (
	arpeggio = [...]float64{noteE4, noteGs4, noteB4, noteE5, noteGs5, noteB5}
	chord    = [...]float64{noteE4, noteGs4, noteB4, noteE5}
	low      = [...]float64{noteE3, noteB2}
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
	// The plucks, from left to right as the letters come in.
	for i, f := range arpeggio {
		pan := -0.45 + 0.9*float64(i)/float64(len(arpeggio)-1)
		addPanned(&l, &r, 0.12*pluck(t, arpAt+float64(i)*arpGap, f), pan)
	}
	// The chord, growing from the first frame, wide and in the middle:
	// each note a hair sharp in one ear and flat in the other. A hair
	// only: a phone's one speaker adds the ears together, and further
	// apart they would beat, a note fading out and back as they drift.
	for i, f := range chord {
		d := 1 + chordSpread*float64(i%2*2-1)
		l += 0.27 * chordTone(t, f, d)
		r += 0.27 * chordTone(t, f, 1/d)
	}
	for i, f := range low {
		d := 1 + chordSpread*float64(i%2*2-1)
		l += 0.34 * lowTone(t, f, d)
		r += 0.34 * lowTone(t, f, 1/d)
	}
	return l, r
}

// chordSpread is how far apart in the two ears the chord's notes are
// tuned: by the peak, E5's two drift less than a third of a turn apart.
const chordSpread = 0.0003

// grow is how loud the chord is at time t: a quiet start at the first
// frame, growing faster and faster to its fullest at peakAt, then
// ringing out.
func grow(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t < peakAt {
		p := t / peakAt
		// A 10 ms rise into its first level, so it starts with no click.
		return min(t/0.01, 1) * (0.2 + 0.8*p*p)
	}
	return math.Exp(-(t - peakAt) / chordRing)
}

// bright is how rich the chord is at time t, from 0 at the first frame
// to 1 at peakAt: how much of its upper harmonics sound.
func bright(t float64) float64 { return min(max(t/peakAt, 0), 1) }

// chordTone is a note of the chord at time t, tuned by detune: a warm
// tone that grows with the chord, its octave and twelfth coming in as
// it grows richer.
func chordTone(t, f, detune float64) float64 {
	w, b := phase(t, 0, f, detune), bright(t)
	return grow(t) * (math.Sin(w) + (0.08+0.3*b)*math.Sin(2*w) + (0.02+0.1*b)*math.Sin(3*w))
}

// lowTone is a low tone under the chord at time t, tuned by detune. Its
// own pitch is soft and its octave strong, so a phone's small speaker,
// which plays little of the pitch itself, still gives the low tone by
// its octave. Its double octave comes in as the chord grows richer.
func lowTone(t, f, detune float64) float64 {
	w, b := phase(t, 0, f, detune), bright(t)
	return grow(t) * (0.35*math.Sin(w) + 0.8*math.Sin(2*w) + (0.08+0.2*b)*math.Sin(4*w))
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

// The boing the little person makes as they fall: a springy, wobbling
// twang, its pitch bouncing up and down quickly as it falls from C#5 to
// E4 and dies away, as a cartoon spring's does. It is boingLength
// seconds long and peaks at boingPeak, as loud as the sting is at its
// fullest.
const (
	boingLength = 0.34
	boingPeak   = 0.24
	// boingFrom and boingTo are where the pitch falls from and to, in
	// hertz, over about boingGlide seconds.
	boingFrom  = 554.37
	boingTo    = noteE4
	boingGlide = 0.15
	// boingRate is how many times a second the pitch bounces, and
	// boingDepth how far, as a share of the pitch, at first; the bounce
	// dies away over about boingSettle seconds.
	boingRate   = 15
	boingDepth  = 0.3
	boingSettle = 0.2
)

var boings = map[int]*audio.Clip{}

// boingAt returns the boing made for a mixer running at rate frames a
// second, making it the first time.
func boingAt(rate int) *audio.Clip {
	stingMu.Lock()
	defer stingMu.Unlock()
	if c := boings[rate]; c != nil {
		return c
	}
	c := audio.NewClip(boingSamples(rate))
	boings[rate] = c
	return c
}

// boingSamples renders the boing at rate frames a second, as stereo
// samples, left then right, the same in both.
func boingSamples(rate int) []float32 {
	hz := float64(rate)
	n := int(boingLength * hz)
	s := make([]float32, 2*n)
	var ph, loudest float64
	for i := range n {
		t := float64(i) / hz
		ph += 2 * math.Pi * boingPitch(t) / hz
		// A twang: the tone and its next two harmonics, struck quickly and
		// dying away, the last 40 ms fading to silence.
		v := math.Sin(ph) + 0.35*math.Sin(2*ph) + 0.12*math.Sin(3*ph)
		v *= min(t/0.005, 1) * math.Exp(-t/0.16) * min(1, float64(n-i)/(0.04*hz))
		s[2*i], s[2*i+1] = float32(v), float32(v)
		loudest = max(loudest, math.Abs(v))
	}
	if loudest > 0 {
		k := float32(boingPeak / loudest)
		for i := range s {
			s[i] *= k
		}
	}
	return s
}

// boingPitch is the boing's pitch t seconds in, in hertz.
func boingPitch(t float64) float64 {
	glide := boingTo + (boingFrom-boingTo)*math.Exp(-t/boingGlide)
	return glide * (1 + boingDepth*math.Exp(-t/boingSettle)*math.Sin(2*math.Pi*boingRate*t))
}
