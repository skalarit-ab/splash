package splash

import (
	"fmt"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/shape"
)

// stage is an offscreen window showing the intro over an app's first
// screen, as an app shows it, stepped a frame at a time.
type stage struct {
	t    *testing.T
	w    *gunim.Window
	c    gunim.Client
	size geom.Size
	// dt is the time between frames.
	dt time.Duration
	// at is the moment of the intro the last frame drew: zero at the
	// first frame, and dt more at each one after.
	at time.Duration
	// frames counts the frames drawn.
	frames int
	// dones are the Done intents the app has heard, and when.
	dones   []Done
	doneAts []time.Duration
	app     *app
	// unmount says the app unmounts the intro on Done, as an app should.
	unmount bool
}

// app is the app's first screen: it takes the keyboard, and notes the
// keys it hears.
type app struct{ keys int }

func (a *app) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }
func (a *app) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children)          {}
func (a *app) Focusable() bool                                                       { return true }
func (a *app) Handle(e input.Event, _ *gunim.UI) bool {
	if _, ok := e.(input.KeyPress); ok {
		a.keys++
		return true
	}
	return false
}

// newStage shows in over an app in a window of size, at hz frames a
// second, with its sound through mix, and runs the first frame.
func newStage(t *testing.T, size geom.Size, hz int, in Intro, mix *audio.Mixer) *stage {
	t.Helper()
	s := &stage{t: t, size: size, dt: time.Second / time.Duration(hz), app: &app{}, unmount: true}
	s.w = gunimtest.New(t, size, nil)
	s.c = s.w.Client()
	Register(s.w, mix)
	gunim.RegisterView(s.w, "app", func(struct{}) *app { return s.app }, nil)
	for _, err := range []error{
		s.c.Mount(gunim.Root, "app", "app", nil),
		s.c.Focus("app"),
		s.c.Mount(gunim.Root, "intro", View, in),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	s.frame()
	return s
}

// frame runs one frame, and hears the intents it sent.
func (s *stage) frame() {
	s.t.Helper()
	s.w.Frame(s.dt)
	s.frames++
	s.at = time.Duration(s.frames-1) * s.dt
	s.drain()
}

// drain hears the intents sent so far.
func (s *stage) drain() {
	s.t.Helper()
	for {
		select {
		case ev := <-s.c.Intents():
			switch in := ev.Intent.(type) {
			case Done:
				s.dones = append(s.dones, in)
				s.doneAts = append(s.doneAts, s.at)
				if s.unmount {
					if err := s.c.Unmount("intro"); err != nil {
						s.t.Fatal(err)
					}
				}
			case gunim.CommandFailed:
				s.t.Fatalf("a command failed: %+v", in)
			}
		default:
			return
		}
	}
}

// part is a piece of the logo as a frame drew it: where, in window
// space, and how opaque.
type part struct {
	rect  geom.Rect
	alpha float32
	// color is a solid part's colour.
	color color.NRGBA
}

// The person's parts, as look names them, after the logo's.
const (
	personBody = 100 + iota
	personHead
	personLid
	personBase
	personMark
	personLeftHand
	personRightHand
)

// personParts are the person's parts by the name look gives them.
func personParts(t *testing.T) map[*shape.Path]int {
	t.Helper()
	pn, err := thePerson()
	if err != nil {
		t.Fatal(err)
	}
	return map[*shape.Path]int{
		pn.body: personBody, pn.head: personHead, pn.lid: personLid, pn.base: personBase,
		pn.mark: personMark, pn.hands[0]: personLeftHand, pn.hands[1]: personRightHand,
	}
}

// look is what a frame drew of the intro.
type look struct {
	// bg is the background's opacity, or -1 where none was drawn.
	bg float32
	// parts are the logo's pieces drawn, by their place in the logo:
	// the left wedge, the right wedge, the diamond and the letters.
	parts map[int]part
}

// person returns the person's parts in lk.
func (lk look) person() map[int]part {
	out := map[int]part{}
	for id, p := range lk.parts {
		if id >= personBody {
			out[id] = p
		}
	}
	return out
}

// look reads what the last frame drew.
func (s *stage) look() look {
	s.t.Helper()
	return lookAt(s.t, s.w.Offscreen().Ops())
}

func lookAt(t *testing.T, ops []paint.Op) look {
	t.Helper()
	l, err := theLogo()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[*shape.Path]int{}
	for i, f := range l.mark {
		ids[f.Parts[0].Path] = i
	}
	for i, f := range l.letters {
		ids[f.Parts[0].Path] = partLetters + i
	}
	for path, id := range personParts(t) {
		ids[path] = id
	}
	lk := look{bg: -1, parts: map[int]part{}}
	opacity := []float32{1}
	for _, op := range ops {
		switch op := op.(type) {
		case *paint.LayerOp:
			opacity = append(opacity, opacity[len(opacity)-1]*op.Opts.Opacity)
		case *paint.LayerEndOp:
			opacity = opacity[:len(opacity)-1]
		case *paint.RRectOp:
			if lk.bg < 0 {
				lk.bg = float32(op.Fill.Solid.A) / 255 * opacity[len(opacity)-1]
			}
		case *paint.MaskOp:
			fill, ok := op.Shape.(shape.Fill)
			if !ok {
				continue
			}
			id, ok := ids[fill.Path]
			if !ok {
				continue
			}
			if _, seen := lk.parts[id]; seen {
				// The screen's light on the person's head and body, over them.
				continue
			}
			a := opacity[len(opacity)-1]
			if g := op.Gradient; g != nil {
				// A gradient shows as much as its most opaque colour.
				top := max(g.Start.A, g.End.A)
				for _, st := range g.Stops {
					top = max(top, st.Color.A)
				}
				a *= float32(top) / 255
			} else {
				a *= float32(op.Color.A) / 255
			}
			lk.parts[id] = part{rect: moved(op.Transform, op.Rect), alpha: a, color: op.Color}
		}
	}
	return lk
}

// moved is the box round r once t moves it.
func moved(t paint.Transform, r geom.Rect) geom.Rect {
	ps := []geom.Point{t.Apply(r.Min), t.Apply(r.Max), t.Apply(geom.Pt(r.Min.X, r.Max.Y)), t.Apply(geom.Pt(r.Max.X, r.Min.Y))}
	out := geom.Rect{Min: ps[0], Max: ps[0]}
	for _, p := range ps[1:] {
		out.Min = geom.Pt(min(out.Min.X, p.X), min(out.Min.Y, p.Y))
		out.Max = geom.Pt(max(out.Max.X, p.X), max(out.Max.Y, p.Y))
	}
	return out
}

// partName names a part for a failure.
func partName(id int) string {
	switch id {
	case partLeft:
		return "the left wedge"
	case partRight:
		return "the right wedge"
	case partDiamond:
		return "the diamond"
	case personBody:
		return "the person's body"
	case personHead:
		return "the person's head"
	case personLid:
		return "the person's laptop"
	case personBase:
		return "the person's laptop's base"
	case personMark:
		return "the mark on the laptop"
	case personLeftHand:
		return "the person's left hand"
	case personRightHand:
		return "the person's right hand"
	}
	return fmt.Sprintf("letter %c", "SKALARIT"[id-partLetters])
}

// rest is where each part sits once the intro has settled, in a window
// of size.
func rest(t *testing.T, size geom.Size) map[int]geom.Rect { return restIn(t, size, geom.Insets{}) }

// restIn is rest, with the system's bars over the window at safe.
func restIn(t *testing.T, size geom.Size, safe geom.Insets) map[int]geom.Rect {
	t.Helper()
	l, err := theLogo()
	if err != nil {
		t.Fatal(err)
	}
	r := logoRect(l.box, geom.Rect{Max: size.Point()}.Inset(safe))
	out := map[int]geom.Rect{}
	for i, f := range l.mark {
		fill := f.Parts[0].Path.Fill()
		out[i] = fill.In(l.box, r)
	}
	for i, f := range l.letters {
		fill := f.Parts[0].Path.Fill()
		out[partLetters+i] = fill.In(l.box, r)
	}
	// The person, seated, their hands at rest, scaled about the seat.
	k := r.Size().W / l.box.Size().W
	box := geom.Rect{Min: l.box.Min.Sub(l.seat), Max: l.box.Max.Sub(l.seat)}
	seat := r.Min.Add(l.seat.Sub(l.box.Min).Mul(k))
	for path, id := range personParts(t) {
		fill := path.Fill()
		out[id] = moved(paint.Scale(personSize, seat), fill.In(box, r))
	}
	return out
}

// near reports whether a and b are within d of each other at every
// edge.
func near(a, b geom.Rect, d float32) bool {
	return abs(a.Min.X-b.Min.X) <= d && abs(a.Min.Y-b.Min.Y) <= d && abs(a.Max.X-b.Max.X) <= d && abs(a.Max.Y-b.Max.Y) <= d
}

func abs(v float32) float32 { return float32(math.Abs(float64(v))) }

// smooth fails the test where a part jumped between two frames dt
// apart: it moved, grew or faded faster than any of the intro's motions
// go, or came in or left at once. size is the window's size, which the
// logo's motions scale with.
func smooth(t *testing.T, when time.Duration, prev, cur look, dt time.Duration, size geom.Size) {
	t.Helper()
	secs := float32(dt.Seconds())
	// The logo is at most 520 wide; its parts never travel faster than
	// four of its widths a second, and never fade faster than a full
	// fade in a thirtieth of a second.
	reach := 4 * min(0.64*size.W, 520) * secs
	fade := 30 * secs
	for id, c := range cur.parts {
		p, was := prev.parts[id]
		if !was {
			// A part arrives faint, or small.
			if c.alpha > 0.4 && c.rect.Size().W > 0.4*rest(t, size)[id].Size().W {
				t.Fatalf("at %v %s came in at once, %.2f opaque and %v", when, partName(id), c.alpha, c.rect)
			}
			continue
		}
		if !near(p.rect, c.rect, reach+1) {
			t.Fatalf("at %v %s jumped from %v to %v", when, partName(id), p.rect, c.rect)
		}
		if abs(p.alpha-c.alpha) > fade+0.01 {
			t.Fatalf("at %v %s jumped from %.2f to %.2f opaque", when, partName(id), p.alpha, c.alpha)
		}
	}
	for id, p := range prev.parts {
		if _, is := cur.parts[id]; !is && p.alpha > 0.15 && p.rect.Size().W > 0.15*rest(t, size)[id].Size().W {
			t.Fatalf("at %v %s went at once, from %.2f opaque", when, partName(id), p.alpha)
		}
	}
	if prev.bg >= 0 && cur.bg >= 0 && abs(prev.bg-cur.bg) > fade+0.01 {
		t.Fatalf("at %v the background jumped from %.2f to %.2f opaque", when, prev.bg, cur.bg)
	}
	if prev.bg > 0.15 && cur.bg < 0 {
		t.Fatalf("at %v the background went at once, from %.2f opaque", when, prev.bg)
	}
}

// inside fails the test for a part drawn outside the window.
func inside(t *testing.T, when time.Duration, lk look, size geom.Size) {
	t.Helper()
	box := geom.Rect{Max: size.Point()}
	for id, p := range lk.parts {
		r := p.rect
		if r.Min.X < box.Min.X-0.5 || r.Min.Y < box.Min.Y-0.5 || r.Max.X > box.Max.X+0.5 || r.Max.Y > box.Max.Y+0.5 {
			t.Fatalf("at %v %s is drawn at %v, outside the window's %v", when, partName(id), r, box)
		}
	}
}

// paintOf returns what s paints in a box of size, outside any window.
func paintOf(s *Splash, size geom.Size) []paint.Op {
	var p paint.Painter
	f := gunim.Frame{Scale: 1}
	s.Layout(gunim.Tight(size), f, gunim.Children{})
	s.Paint(&p, f, size, gunim.Children{})
	return p.Ops()
}
