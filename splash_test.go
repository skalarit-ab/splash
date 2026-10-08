package splash

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The window sizes the intro is tried in: a desktop window, a phone
// held upright and sideways, and a tablet.
var sizes = []geom.Size{geom.Sz(900, 600), geom.Sz(390, 800), geom.Sz(800, 360), geom.Sz(1024, 1366)}

// run plays frames until the stage has run until, checking each frame
// against the one before.
func (s *stage) run(until time.Duration, prev *look) {
	s.t.Helper()
	for s.at+s.dt/2 < until {
		s.frame()
		cur := s.look()
		inside(s.t, s.at, cur, s.size)
		lettersInOrder(s.t, s.at, cur)
		smooth(s.t, s.at, *prev, cur, s.dt, s.size)
		*prev = cur
	}
}

// lettersInOrder fails the test where a letter shows more than the one
// before it: the letters come in from left to right.
func lettersInOrder(t *testing.T, when time.Duration, lk look) {
	t.Helper()
	for i := 1; i < letterCount; i++ {
		a, b := lk.parts[partLetters+i-1], lk.parts[partLetters+i]
		if b.alpha > a.alpha+0.001 {
			t.Fatalf("at %v %s shows %.2f, more than %s's %.2f", when, partName(partLetters+i), b.alpha, partName(partLetters+i-1), a.alpha)
		}
	}
}

// Played through, the intro moves smoothly on every frame, keeps to the
// window, brings in its letters from left to right, comes to rest with
// the logo whole in the middle, sends Done once at 1.3 seconds, and has
// faded away by Length.
func TestTheIntroPlaysThroughSmoothly(t *testing.T) {
	for _, hz := range []int{60, 144} {
		for _, size := range sizes {
			for _, bg := range []string{"light", "dark"} {
				t.Run(fmt.Sprintf("%dHz/%vx%v/%s", hz, size.W, size.H, bg), func(t *testing.T) {
					in := Intro{}
					if bg == "dark" {
						in.Background = Dark
					}
					s := newStage(t, size, hz, in, nil)
					first := s.look()
					if len(first.parts) != 0 || first.bg != 1 {
						t.Fatalf("the first frame shows %d parts on a background %.2f opaque, want only the background", len(first.parts), first.bg)
					}
					prev := first
					s.run(time.Second, &prev)
					// At rest: every part whole, where the logo puts it.
					want := rest(t, size)
					for id, r := range want {
						got, ok := prev.parts[id]
						if !ok || got.alpha < 0.999 || !near(got.rect, r, 0.5) {
							t.Fatalf("at rest %s is %+v, want %v whole", partName(id), got, r)
						}
					}
					s.run(Length+2*s.dt, &prev)
					if len(s.dones) != 1 || s.dones[0].Skipped {
						t.Fatalf("the app heard %+v, want one Done, unskipped", s.dones)
					}
					if d := s.doneAts[0] - doneAt; d < 0 || d > s.dt {
						t.Fatalf("Done came at %v, want %v", s.doneAts[0], doneAt)
					}
					if len(prev.parts) != 0 || prev.bg >= 0 {
						t.Fatalf("at %v the intro still draws %d parts and a background %.2f opaque", s.at, len(prev.parts), prev.bg)
					}
				})
			}
		}
	}
}

// The intro moves the same at any frame rate: each moment looks the
// same at 30, 60, 90 and 144 frames a second. The moments are sixths of
// a second, which a frame at each rate falls on.
func TestTheIntroMovesTheSameAtAnyRate(t *testing.T) {
	size := geom.Sz(900, 600)
	for sixths := 1; sixths <= 9; sixths++ {
		at := time.Duration(sixths) * time.Second / 6
		looks := make([]look, 0, 4)
		for _, hz := range []int{30, 60, 90, 144} {
			s := newStage(t, size, hz, Intro{}, nil)
			// The first frame is the intro's moment zero.
			for s.at+s.dt/2 < at {
				s.frame()
			}
			looks = append(looks, s.look())
		}
		for i, lk := range looks[1:] {
			for id, p := range looks[0].parts {
				q := lk.parts[id]
				if !near(p.rect, q.rect, 0.5) || abs(p.alpha-q.alpha) > 0.01 {
					t.Fatalf("at %v %s is %+v at 30 Hz and %+v at the rate %d", at, partName(id), p, q, i+1)
				}
			}
		}
	}
}

// Hold shows the moment the intro shows when played to it.
func TestHoldShowsTheMomentPlayed(t *testing.T) {
	size := geom.Sz(900, 600)
	for _, at := range []time.Duration{0, 150 * time.Millisecond, 450 * time.Millisecond, 1450 * time.Millisecond} {
		played := newStage(t, size, 120, Intro{}, nil)
		for played.at+played.dt/2 < at {
			played.frame()
		}
		held := New(Intro{}, nil)
		held.Hold(at)
		got := lookAt(t, paintOf(held, size))
		want := played.look()
		if len(got.parts) != len(want.parts) {
			t.Fatalf("at %v Hold draws %d parts, and playing %d", at, len(got.parts), len(want.parts))
		}
		for id, p := range want.parts {
			if q := got.parts[id]; !near(p.rect, q.rect, 0.5) || abs(p.alpha-q.alpha) > 0.01 {
				t.Fatalf("at %v %s is %+v played and %+v held", at, partName(id), p, q)
			}
		}
	}
}

// A press, a tap or a key in the middle skips the intro: the app hears
// Done, skipped, at once, the intro fades away smoothly within 0.2
// seconds, and nothing more comes of further presses.
func TestASkipFadesAtOnceAndSendsDoneOnce(t *testing.T) {
	skips := map[string]func(w *gunim.Window){
		"click": func(w *gunim.Window) {
			w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
			w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary, Time: time.Now()})
		},
		"key": func(w *gunim.Window) { w.Input(input.KeyPress{Key: input.KeySpace}) },
	}
	for name, skip := range skips {
		// At once, as the diamond pops, as the letters come in, and as
		// the intro fades on its own.
		for _, at := range []time.Duration{0, 50 * time.Millisecond, 380 * time.Millisecond, 1350 * time.Millisecond} {
			t.Run(fmt.Sprintf("%s/%v", name, at), func(t *testing.T) {
				s := newStage(t, geom.Sz(900, 600), 144, Intro{}, nil)
				prev := s.look()
				s.run(at, &prev)
				skip(s.w)
				s.drain()
				natural := at >= doneAt
				if !natural && (len(s.dones) != 1 || !s.dones[0].Skipped) {
					t.Fatalf("after the %s the app heard %+v, want one Done, skipped", name, s.dones)
				}
				s.run(at+skipFade.Duration+2*s.dt, &prev)
				if natural {
					// Fading already, it fades on as it was.
					s.run(Length+2*s.dt, &prev)
				}
				if len(prev.parts) != 0 || prev.bg >= 0 {
					t.Fatalf("at %v, after the %s, the intro still draws %d parts", s.at, name, len(prev.parts))
				}
				skip(s.w)
				skip(s.w)
				s.run(s.at+time.Second, &prev)
				if len(s.dones) != 1 {
					t.Fatalf("the app heard %d Dones, want one", len(s.dones))
				}
			})
		}
	}
}

// The intro holds the keyboard while it shows: the key that skips it
// goes no further, and once the intro has gone the app has the
// keyboard back.
func TestTheIntroHoldsTheKeyboardWhileItShows(t *testing.T) {
	s := newStage(t, geom.Sz(900, 600), 60, Intro{}, nil)
	prev := s.look()
	s.run(300*time.Millisecond, &prev)
	s.w.Input(input.KeyPress{Key: input.KeyA})
	if s.app.keys != 0 {
		t.Fatal("the app heard the key that skipped the intro")
	}
	s.run(time.Second, &prev)
	s.w.Input(input.KeyPress{Key: input.KeyA})
	if s.app.keys != 1 {
		t.Fatalf("once the intro had gone the app heard %d keys, want 1", s.app.keys)
	}
}

// A window resized in the middle has the intro carry on where it was,
// the logo laid out afresh in the middle of the new size.
func TestAResizeInTheMiddleCarriesOn(t *testing.T) {
	for _, to := range []geom.Size{geom.Sz(390, 800), geom.Sz(1600, 900), geom.Sz(120, 60)} {
		t.Run(fmt.Sprintf("%vx%v", to.W, to.H), func(t *testing.T) {
			s := newStage(t, geom.Sz(900, 600), 60, Intro{}, nil)
			prev := s.look()
			s.run(500*time.Millisecond, &prev)
			before := prev
			s.w.Offscreen().Resize(to)
			s.size = to
			s.frame()
			after := s.look()
			inside(t, s.at, after, to)
			// The same parts show, as much as before or more: the intro
			// went on rather than starting again.
			for id, p := range before.parts {
				if q, ok := after.parts[id]; !ok || q.alpha < p.alpha-0.001 {
					t.Fatalf("after the resize %s is %+v, and was %+v", partName(id), q, p)
				}
			}
			prev = after
			s.run(time.Second, &prev)
			for id, r := range rest(t, to) {
				if got := prev.parts[id]; !near(got.rect, r, 0.5) {
					t.Fatalf("at rest after the resize %s is at %v, want %v", partName(id), got.rect, r)
				}
			}
			s.run(Length+2*s.dt, &prev)
			if len(s.dones) != 1 || s.dones[0].Skipped || s.doneAts[0]-doneAt > s.dt {
				t.Fatalf("the app heard %+v at %v, want one Done at %v", s.dones, s.doneAts, doneAt)
			}
		})
	}
}

// The window hidden in the middle, as a phone's app sent to the
// background, skips the intro: the app hears Done at once, while the
// window draws nothing, and shown again the intro fades from where it
// held.
func TestHiddenInTheMiddleSkips(t *testing.T) {
	mix := audio.NewMixer()
	s := newStage(t, geom.Sz(390, 800), 60, Intro{}, mix)
	prev := s.look()
	s.run(400*time.Millisecond, &prev)
	s.w.Input(driver.WindowShown{Shown: false})
	s.drain()
	if len(s.dones) != 1 || !s.dones[0].Skipped {
		t.Fatalf("hidden, the app heard %+v, want one Done, skipped", s.dones)
	}
	// The sting fades out while the window is away.
	mix.Mix(make([]float32, 2*int(mix.Frames(time.Second))))
	if n := mix.Playing(); n != 0 {
		t.Fatalf("hidden, the mixer still plays %d voices", n)
	}
	// A real window draws no frames while hidden, so none run here.
	s.w.Input(driver.WindowShown{Shown: true})
	s.frame()
	back := s.look()
	smooth(t, s.at, prev, back, s.dt, s.size)
	prev = back
	s.run(s.at+skipFade.Duration+2*s.dt, &prev)
	if len(prev.parts) != 0 || prev.bg >= 0 {
		t.Fatalf("shown again, the intro still draws %d parts", len(prev.parts))
	}
	if len(s.dones) != 1 {
		t.Fatalf("the app heard %d Dones, want one", len(s.dones))
	}
}

// A tiny window, down to none at all, plays the intro through, keeps
// the logo inside it, and sends Done once.
func TestATinyWindowPlaysThrough(t *testing.T) {
	for _, size := range []geom.Size{geom.Sz(0, 0), geom.Sz(1, 1), geom.Sz(24, 24), geom.Sz(200, 30), geom.Sz(30, 200)} {
		t.Run(fmt.Sprintf("%vx%v", size.W, size.H), func(t *testing.T) {
			s := newStage(t, size, 60, Intro{}, nil)
			prev := s.look()
			s.run(Length+2*s.dt, &prev)
			if len(s.dones) != 1 || s.dones[0].Skipped {
				t.Fatalf("the app heard %+v, want one Done, unskipped", s.dones)
			}
			if len(prev.parts) != 0 {
				t.Fatalf("the intro still draws %d parts", len(prev.parts))
			}
		})
	}
}

// The logo keeps its proportions in every window, sits in the middle,
// and fits within 64% of the width, half the height and 520 pixels.
func TestTheLogoKeepsItsShapeAndFits(t *testing.T) {
	l, err := theLogo()
	if err != nil {
		t.Fatal(err)
	}
	aspect := l.box.Size().W / l.box.Size().H
	for _, size := range append(sizes, geom.Sz(3840, 2160), geom.Sz(320, 480), geom.Sz(50, 500)) {
		r := logoRect(l.box, geom.Rect{Max: size.Point()})
		if got := r.Size().W / r.Size().H; abs(got-aspect) > 0.001*aspect {
			t.Fatalf("in %v the logo is %v, out of its proportions", size, r)
		}
		if abs(r.Center().X-size.W/2) > 0.01 || abs(r.Center().Y-size.H/2) > 0.01 {
			t.Fatalf("in %v the logo is at %v, off the middle", size, r)
		}
		if r.Size().W > 0.64*size.W+0.01 || r.Size().H > 0.5*size.H+0.01 || r.Size().W > 520.01 {
			t.Fatalf("in %v the logo is %v, too large", size, r)
		}
	}
}

// The sound plays once, from the first frame, through the app's mixer;
// an app with its sound off, or with no mixer, gets silence; and sound
// turned off in the middle fades the sting out.
func TestSoundPlaysOnlyWhenTheAppWantsIt(t *testing.T) {
	t.Run("on", func(t *testing.T) {
		mix := audio.NewMixer()
		s := newStage(t, geom.Sz(900, 600), 60, Intro{}, mix)
		if mix.Playing() != 1 {
			t.Fatalf("at the first frame the mixer plays %d voices, want the sting", mix.Playing())
		}
		prev := s.look()
		s.run(Length+2*s.dt, &prev)
		if mix.Playing() != 1 {
			t.Fatalf("the mixer plays %d voices, want the sting alone", mix.Playing())
		}
		out := make([]float32, 2*int(mix.Frames(time.Second)))
		mix.Mix(out)
		if loudest(out) == 0 {
			t.Fatal("the sting is silent")
		}
		if mix.Playing() != 0 {
			t.Fatal("the sting plays on past a second")
		}
	})
	t.Run("silent", func(t *testing.T) {
		mix := audio.NewMixer()
		s := newStage(t, geom.Sz(900, 600), 60, Intro{Silent: true}, mix)
		prev := s.look()
		s.run(Length+2*s.dt, &prev)
		if mix.Playing() != 0 {
			t.Fatalf("silent, the mixer plays %d voices", mix.Playing())
		}
	})
	t.Run("no mixer", func(t *testing.T) {
		s := newStage(t, geom.Sz(900, 600), 60, Intro{}, nil)
		prev := s.look()
		s.run(Length+2*s.dt, &prev)
		if len(s.dones) != 1 {
			t.Fatalf("the app heard %d Dones, want one", len(s.dones))
		}
	})
	t.Run("off in the middle", func(t *testing.T) {
		mix := audio.NewMixer()
		s := newStage(t, geom.Sz(900, 600), 60, Intro{}, mix)
		prev := s.look()
		s.run(200*time.Millisecond, &prev)
		if err := s.c.Update("intro", Intro{Silent: true}); err != nil {
			t.Fatal(err)
		}
		s.frame()
		mix.Mix(make([]float32, 2*int(mix.Frames(soundStop+10*time.Millisecond))))
		if mix.Playing() != 0 {
			t.Fatal("sound turned off, the sting plays on")
		}
	})
}

// Taken away before it ends, the intro fades out and sends no Done: the
// app knows already.
func TestUnmountedEarlyItFadesWithNoDone(t *testing.T) {
	s := newStage(t, geom.Sz(900, 600), 144, Intro{}, nil)
	prev := s.look()
	s.run(300*time.Millisecond, &prev)
	if err := s.c.Unmount("intro"); err != nil {
		t.Fatal(err)
	}
	s.run(time.Second, &prev)
	if len(s.dones) != 0 {
		t.Fatalf("the app heard %+v, want nothing", s.dones)
	}
	if len(prev.parts) != 0 || prev.bg >= 0 {
		t.Fatalf("the intro still draws %d parts", len(prev.parts))
	}
}

// An app that leaves the intro in after Done sees it gone all the same,
// and hears no more, however it is pressed or hidden, while it fades
// and after.
func TestLeftInAfterDoneItStaysGone(t *testing.T) {
	s := newStage(t, geom.Sz(900, 600), 60, Intro{}, nil)
	s.unmount = false
	prev := s.look()
	poke := func() {
		s.w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
		s.w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Button: input.ButtonPrimary, Time: time.Now()})
		s.w.Input(input.KeyPress{Key: input.KeySpace})
		s.w.Input(driver.WindowShown{Shown: false})
		s.w.Input(driver.WindowShown{Shown: true})
		s.drain()
	}
	s.run(doneAt+50*time.Millisecond, &prev)
	poke()
	s.run(Length+time.Second, &prev)
	poke()
	s.run(Length+2*time.Second, &prev)
	if len(s.dones) != 1 {
		t.Fatalf("the app heard %d Dones, want one", len(s.dones))
	}
	if len(prev.parts) != 0 || prev.bg > 0 {
		t.Fatalf("the intro still shows %d parts on a background %.2f opaque", len(prev.parts), prev.bg)
	}
}

// The letters are black on a light background and white on a dark one.
func TestTheLettersSuitTheBackground(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Intro
		ink  uint8
	}{{"zero", Intro{}, 0}, {"light", Intro{Background: Light}, 0}, {"dark", Intro{Background: Dark}, 0xff}} {
		if got := inkOn(background(c.in)); got.R != c.ink || got.A != 0xff {
			t.Fatalf("on %s the letters are %v", c.name, got)
		}
	}
}

// The state and the intent survive a socket.
func TestWire(t *testing.T) {
	if err := gunim.CheckWire(Intro{Background: Dark, Silent: true}, Done{Skipped: true}); err != nil {
		t.Fatal(err)
	}
}

// The sting is under a second, gentle, in both ears, and ends in
// silence with no click.
func TestTheStingIsShortAndGentle(t *testing.T) {
	c := stingAt(audio.SampleRate)
	if d := audio.Duration(c.Len()); d >= time.Second {
		t.Fatalf("the sting is %v long", d)
	}
	s := c.Samples()
	if l := loudest(s); math.Abs(float64(l-stingPeak)) > 1e-3 {
		t.Fatalf("the sting peaks at %v, want %v", l, stingPeak)
	}
	var left, right float64
	for i := 0; i+1 < len(s); i += 2 {
		left += math.Abs(float64(s[i]))
		right += math.Abs(float64(s[i+1]))
	}
	if left == 0 || right == 0 || left/right < 0.7 || left/right > 1.4 {
		t.Fatalf("the sting is out of balance: left %v, right %v", left, right)
	}
	if end := loudest(s[len(s)-2:]); end > 1e-3 {
		t.Fatalf("the sting ends at %v, not in silence", end)
	}
}

func TestTheLogoReads(t *testing.T) {
	l, err := theLogo()
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range l.mark {
		if f.Parts[0].FillGradient == nil {
			t.Fatalf("%s has no gradient", partName(i))
		}
	}
	if g := l.mark[partDiamond].Parts[0].FillGradient; g.Aspect <= 1 {
		t.Fatalf("the diamond's glow is round, aspect %v, want an oval", g.Aspect)
	}
}

func loudest(s []float32) float32 {
	var m float32
	for _, v := range s {
		m = max(m, abs(v))
	}
	return m
}

// On a phone the logo sits in the middle of the part the system's bars
// leave clear.
func TestTheLogoKeepsClearOfThePhonesBars(t *testing.T) {
	size := geom.Sz(390, 800)
	safe := geom.Insets{Top: 48, Bottom: 34}
	s := newStage(t, size, 60, Intro{}, nil)
	s.w.Offscreen().SetSafeArea(safe)
	prev := s.look()
	s.run(time.Second, &prev)
	for id, r := range restIn(t, size, safe) {
		if got := prev.parts[id]; !near(got.rect, r, 0.5) {
			t.Fatalf("at rest %s is at %v, want %v", partName(id), got.rect, r)
		}
	}
}
