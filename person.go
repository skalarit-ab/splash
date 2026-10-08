package splash

import (
	"image/color"
	"math"
	"sync"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/shape"
)

// person is the little person at a laptop who sits on top of the T: a
// round head, a body, the back of the laptop's lid in the diamond's
// blue with a small diamond on it, the screen's light on their face,
// and two hands that type. Its paths are in the logo's units, with the
// seat, the middle of the T's top, at the origin.
type person struct {
	head, body, lid, base, mark *shape.Path
	hands                       [2]*shape.Path
}

// The person's parts, in the logo's units from the seat, before
// personSize: the body is 12 wide and 11 high, the head 9.2 across
// above it, and the lid 15 wide in front of the body.
const (
	bodyPath = "M -6 0 L -6 -6 C -6 -9.6 -3.3 -11 0 -11 C 3.3 -11 6 -9.6 6 -6 L 6 0 Z"
	headPath = "M -4.6 -16.5 A 4.6 4.6 0 1 0 4.6 -16.5 A 4.6 4.6 0 1 0 -4.6 -16.5 Z"
	lidPath  = "M -6.5 -9.5 L 6.5 -9.5 Q 7.5 -9.5 7.5 -8.5 L 7.5 -1 L -7.5 -1 L -7.5 -8.5 Q -7.5 -9.5 -6.5 -9.5 Z"
	basePath = "M -8.5 -1 L 8.5 -1 L 8.5 -0.2 Q 8.5 0 8.3 0 L -8.3 0 Q -8.5 0 -8.5 -0.2 Z"
	markPath = "M 0 -7 L 1.2 -5.2 L 0 -3.4 L -1.2 -5.2 Z"
	leftHand = "M -9.3 -4 A 1.5 1.5 0 1 0 -6.3 -4 A 1.5 1.5 0 1 0 -9.3 -4 Z"
	right    = "M 6.3 -4 A 1.5 1.5 0 1 0 9.3 -4 A 1.5 1.5 0 1 0 6.3 -4 Z"
)

// The person's colours, besides the letters' ink: the lid in the
// diamond's blue, its base and the screen's light in the right wedge's
// teal, and the mark on the lid in the glow's pale blue.
var (
	lidBlue   = color.NRGBA{R: 0x36, G: 0x92, B: 0xb6, A: 0xff}
	teal      = color.NRGBA{R: 0x4d, G: 0xb7, B: 0xca, A: 0xff}
	paleBlue  = color.NRGBA{R: 0xe6, G: 0xf0, B: 0xf9, A: 0xff}
	lightTeal = color.NRGBA{R: 0x4d, G: 0xb7, B: 0xca, A: 0x9c}
)

// thePerson reads the person's paths the first time it is asked for.
var thePerson = sync.OnceValues(func() (*person, error) {
	var p person
	for _, part := range []struct {
		to **shape.Path
		d  string
	}{
		{&p.head, headPath}, {&p.body, bodyPath}, {&p.lid, lidPath},
		{&p.base, basePath}, {&p.mark, markPath}, {&p.hands[0], leftHand}, {&p.hands[1], right},
	} {
		path, err := shape.NewPath(part.d)
		if err != nil {
			return nil, err
		}
		*part.to = path
	}
	return &p, nil
})

// pose is how the person is at a moment: sat is how far they have
// popped in, from 0 to 1; look how far they look down, from 0 to 1, as
// they stop typing; fall how far they have fallen, from 0 to 1; alpha
// how much they show; and typing how far up each hand is.
type pose struct {
	sat, look, fall, alpha float32
	typing                 [2]float32
}

// personSize scales the person from their paths: 50 of the logo's
// units high, about a letter, which is 53. Their head reaches 15 units
// above the logo's view box; the logo is at most half as high as the
// room it is centred in, so it leaves at least 63 of its units above
// it, and the head stays in the window.
const personSize = 2.4

// How the person falls: fallDrop down and fallDrift across, in the
// logo's units, turning by fallTurn radians, and with the hands thrown
// up by fallHands. lookDip is how far the head dips as they look down.
const (
	fallDrop  = 100
	fallDrift = 20
	fallTurn  = -2.6
	fallHands = 5
	lookDip   = 1.8
)

// paint draws the person in the pose ps, seated at seat, where the
// logo's view box vb is drawn into r, the body in ink.
func (pn *person) paint(p *paint.Painter, vb, r geom.Rect, seat geom.Point, ps pose, ink color.NRGBA) {
	if ps.sat <= 0 || ps.alpha <= 0 {
		return
	}
	k := r.Size().W / vb.Size().W
	// box is the view box in the person's units, so their paths map
	// into r as the logo's do.
	box := geom.Rect{Min: vb.Min.Sub(seat), Max: vb.Max.Sub(seat)}
	at := func(q geom.Point) geom.Point { return r.Min.Add(q.Sub(box.Min).Mul(k)) }
	sp := at(geom.Point{})
	// Falling, they drop as a weight does, drift off the T's edge and
	// tumble back, turning about their middle.
	drop := geom.Pt(ps.fall*fallDrift*k, ps.fall*fallDrop*k)
	defer p.Push(paint.Translate(drop).
		Mul(paint.Rotate(ps.fall*fallTurn, at(geom.Pt(0, -9*personSize)))).
		Mul(paint.Scale(ps.sat*personSize, sp)))()
	a := ps.alpha
	mask := func(path *shape.Path, c color.NRGBA) {
		f := path.Fill()
		p.Mask(f, f.In(box, r), faded(c, a))
	}
	// The screen lights them from below, brightest just over the lid.
	g := paint.Gradient{
		From: at(geom.Pt(0, -9.5)), To: at(geom.Pt(0, -18)),
		Start: faded(lightTeal, a), End: faded(color.NRGBA{R: teal.R, G: teal.G, B: teal.B}, a),
	}
	for _, path := range []*shape.Path{pn.body, pn.head} {
		// Looking down, the head dips toward the screen.
		pop := func() {}
		if path == pn.head {
			pop = p.Push(paint.Translate(geom.Pt(0, ps.look*lookDip*k)))
		}
		mask(path, ink)
		f := path.Fill()
		p.MaskFill(f, f.In(box, r), paint.Fill{Gradient: &g})
		pop()
	}
	mask(pn.lid, lidBlue)
	mask(pn.base, teal)
	mask(pn.mark, paleBlue)
	for i, hand := range pn.hands {
		// Looking down, they stop typing; falling, they throw up their
		// hands.
		up := ps.typing[i]*(1-clamp01(ps.look)) + ps.fall*fallHands
		pop := p.Push(paint.Translate(geom.Pt(0, -up*k)))
		mask(hand, ink)
		pop()
	}
}

// typing is how far up each hand is at time t, in the logo's units, as
// the person types: each hand taps seven times a second, in turn.
func typing(t float32) [2]float32 {
	tap := func(phase float64) float32 {
		return float32(1.1 * max(0, math.Sin(2*math.Pi*7*float64(t)+phase)))
	}
	return [2]float32{tap(0), tap(math.Pi)}
}
