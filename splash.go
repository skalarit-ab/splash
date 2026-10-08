// Package splash is the "Made by Skalarit" intro that Skalarit AB's
// apps open with: the logo, animated, with a short sting of sound.
//
// The diamond pops in on a spring with a little turn, and the wedges
// open out from behind it to either side. The glow in the diamond
// swells once and settles, and the letters rise into place one after
// another, from left to right. Just after the T, a little person pops
// up on top of it, sat at a laptop, typing. A moment later the intro
// sends [Done] and fades away over what lies under it. The T goes
// first, the person loses their seat and falls, tumbling, with a
// springy boing, and fades last. All of it takes [Length], 1.72
// seconds. A tap, a click or any key skips it before Done: Done goes at
// once, and the intro, the person and the sound fade in 0.2 seconds.
//
// An app shows the intro over its own first screen, which loads behind
// it meanwhile. It registers the view with the window, mounts its first
// screen, mounts the intro over it, and unmounts the intro on Done:
//
//	splash.Register(w, mix) // mix may be nil, for silence
//	c.Mount(gunim.Root, "home", "home", home)
//	c.Mount(gunim.Root, "intro", splash.View, splash.Intro{Background: splash.Dark})
//	...
//	case splash.Done:
//		c.Unmount("intro")
//
// The intro holds the keyboard while it shows, as a dialog does, and
// gives it back to what had it once it leaves.
package splash

import (
	"cmp"
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/shape"
)

// View is the name [Register] gives the intro's view, for
// [gunim.Client.Mount].
const View = "skalarit.splash"

// Intro is the intro's state: what an app mounts [View] with.
type Intro struct {
	// Background is the colour behind the logo, as the app's first
	// screen's, so the intro flows into it. The zero colour is [Light].
	// The letters are black on a light background and white on a dark
	// one.
	Background color.NRGBA
	// Silent plays the intro without its sound, for an app with its
	// sound turned off.
	Silent bool
}

// Done is the intent the intro's view sends as it hands over to the
// app: as it starts to fade, at the end or on a skip. It comes once.
// Skipped says a tap, a click, a key or the window hidden cut the intro
// short.
type Done struct{ Skipped bool }

func init() {
	gunim.RegisterType[Intro]("skalarit.splash.intro")
	gunim.RegisterType[Done]("skalarit.splash.done")
}

// The backgrounds to choose from.
var (
	// Light is white.
	Light = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	// Dark is a deep blue grey.
	Dark = color.NRGBA{R: 0x0e, G: 0x11, B: 0x16, A: 0xff}
)

// The choreography, as when each part starts from the intro's first
// frame.
const (
	diamondAt = 0
	wedgesAt  = 120 * time.Millisecond
	glowAt    = 200 * time.Millisecond
	// glowSwell is how long the glow takes to swell.
	glowSwell = 220 * time.Millisecond
	// settleAt is when the glow, at its fullest, sinks back.
	settleAt  = glowAt + glowSwell
	lettersAt = 300 * time.Millisecond
	// letterGap is how long after one letter the next starts.
	letterGap = 45 * time.Millisecond
	// sitAt is when the little person pops in on top of the T, just
	// after the T has risen into place.
	sitAt = lettersAt + (letterCount-1)*letterGap + 85*time.Millisecond
	// doneAt is when the intro sends Done and starts to fade, the T
	// first.
	doneAt = 1300 * time.Millisecond
	// fallAt is when the person, the T gone from under them, falls, and
	// awayAt when they start to fade.
	fallAt = doneAt + 80*time.Millisecond
	awayAt = doneAt + 220*time.Millisecond
)

// The motions.
var (
	// diamondSpring pops the diamond in, past its size and back.
	diamondSpring = anim.Spring{Response: 0.42, Damping: 0.52}
	// wedgeSpring opens the wedges, with a small overshoot.
	wedgeSpring = anim.Spring{Response: 0.4, Damping: 0.68}
	wedgeFade   = anim.Tween{Duration: 120 * time.Millisecond, Ease: anim.EaseOut}
	// The glow sinks back from its swell on glowSettle.
	glowSettle = anim.Spring{Response: 0.5, Damping: 1}
	// Each letter rises on letterSpring and fades in on letterFade.
	letterSpring = anim.Spring{Response: 0.36, Damping: 0.78}
	letterFade   = anim.Tween{Duration: 180 * time.Millisecond, Ease: anim.EaseOut}
	// fadeAway fades the intro at its end, and skipFade on a skip.
	fadeAway = anim.Tween{Duration: 320 * time.Millisecond, Ease: anim.EaseInOut}
	skipFade = anim.Tween{Duration: 200 * time.Millisecond, Ease: anim.EaseOut}
	// sitSpring pops the person in, past their size and back.
	sitSpring = anim.Spring{Response: 0.38, Damping: 0.5}
	// tAway takes the T away from under the person at the end, quickly,
	// so it has all but gone as they fall.
	tAway = anim.Tween{Duration: 120 * time.Millisecond, Ease: anim.EaseOut}
	// fallMotion drops the person, quicker and quicker, as a weight
	// falls, and personAway fades them.
	fallMotion = anim.Tween{Duration: 340 * time.Millisecond, Ease: easeIn}
	personAway = anim.Tween{Duration: 200 * time.Millisecond, Ease: easeIn}
	// soundStop is how quickly the sound fades on a skip: as quickly
	// as the picture.
	soundStop = 200 * time.Millisecond
)

// Length is how long the intro plays when nothing skips it, from its
// first frame until it has faded away, the falling person last. It
// sends Done 0.42 seconds before the end.
const Length = awayAt + 200*time.Millisecond

// easeIn starts slow and speeds up, as a weight falling does.
func easeIn(t float32) float32 { return t * t }

// How far the parts travel, in the logo's own units, where the logo is
// 417 wide and 127 high.
const (
	// wedgeTravel is how far in, behind the diamond, the wedges start.
	wedgeTravel = 24
	// letterRise is how far below its place each letter starts.
	letterRise = 16
	// turn is how far the diamond starts turned, in radians.
	turn = -0.6
)

// The glow's size, as a share of its size in the logo: small as the
// diamond lands, at its fullest as it swells, and as drawn at rest.
const (
	glowSmall = 0.55
	glowFull  = 1.35
)

// maxStep is the longest step the intro takes in one frame, as anim's
// motions take: a stalled frame slows the intro rather than skipping
// part of it.
const maxStep = 100 * time.Millisecond

// Splash is the intro, as a node. [Register] wraps it as a view, which
// is how an app usually shows it; an app that builds its own tree can
// put a Splash in it and hear of the end through OnDone.
//
// It plays from the first frame it is drawn in. It fills its box with
// its background, with the logo in the middle, as large as fits within
// 64% of the box's width, half its height and 520 logical pixels across.
// On a phone the logo keeps to the part of the box the system's bars
// leave clear, [gunim.Frame.Safe].
//
// It takes a press of the pointer and every key, as it shows over the
// app, and holds the keyboard as a dialog does.
type Splash struct {
	anim.Group
	// OnDone runs as the intro hands over to the app: as it starts to
	// fade, at the end or on a skip. skipped says it was cut short. It
	// runs once, and never after the intro has started to leave the tree.
	OnDone func(skipped bool, u *gunim.UI) gunim.Intent

	in  Intro
	mix *audio.Mixer
	// voice is the sting playing, and boing the person's fall, or nil.
	voice, boing *audio.Voice

	// clock is how far the intro has played; next is the cue to run
	// next.
	clock time.Duration
	next  int
	cues  []cue
	// started says the first frame has come; held that Hold froze the
	// intro.
	started, held bool
	// leaving says the intro has started to fade, and ending that it
	// fades at its end rather than on a skip, the person falling. owed
	// says Done is due at the next layout, and done that it has gone.
	leaving, ending, owed, done bool

	bg *anim.Color
	// diamond is the diamond's size, from 0 to 1; glow the glow's.
	diamond, glow *anim.Float
	// open is how far in the wedges still are, from 1 to 0, and wedges
	// how much they show.
	open, wedges *anim.Float
	// rise is how far below its place each letter is, from 1 to 0, and
	// shown how much it shows.
	rise, shown [letterCount]*anim.Float
	// gone is how far the intro has faded away, from 0 to 1, and tGone
	// how far the T has, which goes first.
	gone, tGone *anim.Float
	// sat is how far the person has popped in, from 0 to 1; fall how far
	// they have fallen, from 0 to 1; and shownPerson how much they show.
	sat, fall, shownPerson *anim.Float
}

// cue starts motions at a moment of the intro, and returns the ones it
// started.
type cue struct {
	at  time.Duration
	run func() []*anim.Float
}

// New returns the intro, showing in, which plays its sound through mix.
// A nil mix plays it silent.
func New(in Intro, mix *audio.Mixer) *Splash {
	s := &Splash{
		in: in, mix: mix,
		bg:      anim.NewColor(background(in)),
		diamond: anim.NewFloat(0), glow: anim.NewFloat(glowSmall),
		open: anim.NewFloat(1), wedges: anim.NewFloat(0),
		gone: anim.NewFloat(0), tGone: anim.NewFloat(0),
		sat: anim.NewFloat(0), fall: anim.NewFloat(0), shownPerson: anim.NewFloat(1),
	}
	s.Add(s.bg, s.diamond, s.glow, s.open, s.wedges, s.gone, s.tGone, s.sat, s.fall, s.shownPerson)
	for i := range letterCount {
		s.rise[i], s.shown[i] = anim.NewFloat(1), anim.NewFloat(0)
		s.Add(s.rise[i], s.shown[i])
	}
	s.cues = s.choreography()
	if mix != nil && !in.Silent {
		// Made now, so the first frame has it ready.
		stingAt(mix.Rate())
	}
	return s
}

// Register registers the intro's view, [View], with w. Its state is an
// [Intro], and it sends [Done] as it hands over to the app. The intro
// plays its sound through mix, which may be nil for none.
func Register(w *gunim.Window, mix *audio.Mixer) {
	gunim.RegisterView(w, View,
		func(in Intro) *Splash {
			s := New(in, mix)
			s.OnDone = func(skipped bool, _ *gunim.UI) gunim.Intent { return Done{Skipped: skipped} }
			return s
		},
		func(s *Splash, in Intro, u *gunim.UI) { s.Set(in, u) })
}

// Set takes new state. A new background glides in once the intro has
// started. Turning the sound off fades out the sting playing.
func (s *Splash) Set(in Intro, _ *gunim.UI) {
	s.in = in
	if s.started {
		s.bg.Animate(background(in), anim.Gentle)
	} else {
		s.bg.Jump(background(in))
	}
	if in.Silent {
		s.quiet()
	}
}

// quiet fades out the sound playing.
func (s *Splash) quiet() {
	for _, v := range []**audio.Voice{&s.voice, &s.boing} {
		if *v != nil {
			(*v).Stop(soundStop)
			*v = nil
		}
	}
}

// background is the colour the intro shows behind the logo for in.
func background(in Intro) color.NRGBA {
	if in.Background.A == 0 {
		return Light
	}
	return in.Background
}

// choreography is the intro's cues, in order.
func (s *Splash) choreography() []cue {
	cues := []cue{
		{diamondAt, func() []*anim.Float {
			s.diamond.Animate(1, diamondSpring)
			return []*anim.Float{s.diamond}
		}},
		{wedgesAt, func() []*anim.Float {
			s.open.Animate(0, wedgeSpring)
			s.wedges.Animate(1, wedgeFade)
			return []*anim.Float{s.open, s.wedges}
		}},
		{glowAt, func() []*anim.Float {
			s.glow.Animate(glowFull, anim.Tween{Duration: glowSwell, Ease: anim.EaseOut})
			return []*anim.Float{s.glow}
		}},
		{settleAt, func() []*anim.Float {
			s.glow.Animate(1, glowSettle)
			return []*anim.Float{s.glow}
		}},
	}
	for i := range letterCount {
		cues = append(cues, cue{lettersAt + time.Duration(i)*letterGap, func() []*anim.Float {
			s.rise[i].Animate(0, letterSpring)
			s.shown[i].Animate(1, letterFade)
			return []*anim.Float{s.rise[i], s.shown[i]}
		}})
	}
	cues = append(cues,
		cue{sitAt, func() []*anim.Float {
			s.sat.Animate(1, sitSpring)
			return []*anim.Float{s.sat}
		}},
		cue{doneAt, func() []*anim.Float {
			if s.leaving {
				return nil
			}
			s.leaving, s.ending, s.owed = true, true, true
			s.gone.Animate(1, fadeAway)
			s.tGone.Animate(1, tAway)
			return []*anim.Float{s.gone, s.tGone}
		}},
		// With the T gone from under them, the person falls, with a boing.
		cue{fallAt, func() []*anim.Float {
			if !s.ending {
				return nil
			}
			s.fall.Animate(1, fallMotion)
			s.playBoing()
			return []*anim.Float{s.fall}
		}},
		cue{awayAt, func() []*anim.Float {
			if !s.ending {
				return nil
			}
			s.shownPerson.Animate(0, personAway)
			return []*anim.Float{s.shownPerson}
		}})
	// advance runs them in the order they come due.
	slices.SortStableFunc(cues, func(a, b cue) int { return cmp.Compare(a.at, b.at) })
	return cues
}

// Step implements [gunim.Animator]. The intro's clock starts at the
// first frame, and runs on the frames' time.
func (s *Splash) Step(dt time.Duration) bool {
	if s.held {
		return false
	}
	if !s.started {
		// The first frame is the intro's moment zero.
		s.started = true
		s.play()
		s.advance(0)
		return true
	}
	moving := s.advance(min(dt, maxStep))
	// The clock runs on to the end's last cue, past moments when nothing
	// moves.
	return moving || s.clock < doneAt || s.ending && s.next < len(s.cues)
}

// advance moves the intro on by dt: the motions under way, then the
// cues that come due, each stepped on by the time since it was due, so
// the intro moves the same at any frame rate.
func (s *Splash) advance(dt time.Duration) (moving bool) {
	moving = s.Group.Step(dt)
	s.clock += dt
	for s.next < len(s.cues) && s.cues[s.next].at <= s.clock {
		c := s.cues[s.next]
		s.next++
		for _, v := range c.run() {
			if v.Step(s.clock - c.at) {
				moving = true
			}
		}
	}
	return moving
}

// play starts the sting, unless the intro is silent.
func (s *Splash) play() {
	if s.mix == nil || s.in.Silent {
		return
	}
	s.voice = s.mix.Play(stingAt(s.mix.Rate()).Source(), audio.Options{})
}

// playBoing plays the person's boing as they fall, unless the intro is
// silent or held.
func (s *Splash) playBoing() {
	if s.mix == nil || s.in.Silent || s.held {
		return
	}
	s.boing = s.mix.Play(boingAt(s.mix.Rate()).Source(), audio.Options{})
}

// Hold stops the intro at, from its first frame, and keeps it there,
// silent, for a picture of that moment. It sends no Done.
func (s *Splash) Hold(at time.Duration) {
	s.quiet()
	s.reset()
	s.started, s.held = true, true
	s.advance(0)
	const step = time.Second / 240
	for s.clock+step <= at {
		s.advance(step)
	}
	s.advance(at - s.clock)
	s.owed = false
}

// reset puts every part back where the intro starts.
func (s *Splash) reset() {
	s.bg.Jump(background(s.in))
	s.diamond.Jump(0)
	s.glow.Jump(glowSmall)
	s.open.Jump(1)
	s.wedges.Jump(0)
	s.gone.Jump(0)
	s.tGone.Jump(0)
	s.sat.Jump(0)
	s.fall.Jump(0)
	s.shownPerson.Jump(1)
	for i := range letterCount {
		s.rise[i].Jump(1)
		s.shown[i].Jump(0)
	}
	s.clock, s.next = 0, 0
	s.leaving, s.ending, s.owed = false, false, false
}

// Skip cuts the intro short: it sends Done, if it has not yet, and
// fades away quickly with its sound, the person too, falling or not.
func (s *Splash) Skip(u *gunim.UI) {
	s.quiet()
	if !s.leaving {
		s.leaving = true
		s.gone.Animate(1, skipFade)
	}
	s.shownPerson.Animate(0, skipFade)
	s.owed = false
	s.finish(true, u)
}

// finish runs OnDone and sends what it returns, once.
func (s *Splash) finish(skipped bool, u *gunim.UI) {
	if s.done || s.held {
		return
	}
	s.done = true
	if s.OnDone == nil {
		return
	}
	if in := s.OnDone(skipped, u); in != nil && u != nil {
		u.Send(s, in)
	}
}

// Handle implements [gunim.Handler]: a press of the pointer or any key
// skips the intro, and so does the window being hidden, as a phone's
// app going to the background.
func (s *Splash) Handle(e input.Event, u *gunim.UI) bool {
	switch e.(type) {
	case input.PointerDown, input.KeyPress:
		s.Skip(u)
		return true
	case input.WindowHidden:
		s.Skip(u)
	}
	return false
}

// Transition implements [gunim.Transitioner]. The intro is all there
// as it arrives, and plays on its own clock. Taken away, it fades as a
// skip does, with no Done.
func (s *Splash) Transition(p gunim.Presence, _ gunim.Frame) bool {
	if p != gunim.Exiting {
		return true
	}
	if !s.leaving {
		s.done = true
		s.Skip(nil)
	}
	return s.faded()
}

// faded reports whether the intro has faded away, the person too.
func (s *Splash) faded() bool {
	return !s.gone.Active() && s.gone.Value() >= 1 && !s.shownPerson.Active() && s.shownPerson.Value() <= 0
}

// Modal implements [gunim.Modal]: the intro holds the keyboard while it
// shows.
func (s *Splash) Modal() bool { return true }

// Focusable implements [gunim.Focusable].
func (s *Splash) Focusable() bool { return true }

// Access implements [gunim.Accessible].
func (s *Splash) Access() access.Info {
	return access.Info{Role: access.RoleImage, Name: "Made by Skalarit"}
}

// Layout implements [gunim.Node]. The intro fills its box, and sends
// Done as it starts to fade.
func (s *Splash) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	if s.owed {
		s.owed = false
		s.finish(false, f.UI())
	}
	return c.Max
}

// Paint implements [gunim.Node].
func (s *Splash) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	gone := s.gone.Value()
	if s.faded() || gone >= 1 && s.sat.Value() <= 0 {
		// Faded away: nothing left to draw.
		return
	}
	bg := s.bg.Value()
	// The background lingers a little behind the logo as both fade, so
	// the logo leaves first.
	if a := 1 - anim.EaseInOut(clamp01((gone-0.15)/0.85)); a > 0 {
		p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(faded(bg, a)))
	}
	l, err := theLogo()
	if err != nil {
		return
	}
	// The logo keeps clear of what the system draws over the window,
	// as a phone's status bar, and the background runs under it.
	r := logoRect(l.box, geom.Rect{Max: box.Point()}.Inset(f.Safe))
	if r.Empty() {
		return
	}
	k := r.Size().W / l.box.Size().W
	at := func(q geom.Point) geom.Point { return r.Min.Add(q.Sub(l.box.Min).Mul(k)) }
	// The logo grows a little as it fades away.
	defer p.Push(paint.Scale(1+0.05*gone, r.Center()))()
	ink := inkOn(bg)
	if gone < 1 {
		s.paintLogo(p, l, r, k, at, gone, ink)
	}
	// The person fades on their own, after the logo, as they fall.
	if pn, err := thePerson(); err == nil {
		ps := pose{sat: s.sat.Value(), fall: s.fall.Value(), alpha: clamp01(s.shownPerson.Value())}
		ps.typing = typing(float32(s.clock.Seconds()))
		pn.paint(p, l.box, r, l.seat, ps, ink)
	}
}

// paintLogo draws the logo, gone faded away, its letters in ink.
func (s *Splash) paintLogo(p *paint.Painter, l *logo, r geom.Rect, k float32, at func(geom.Point) geom.Point, gone float32, ink color.NRGBA) {
	if gone > 0 {
		defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(-r.Size().W)), Opacity: 1 - gone})()
	}
	s.paintWedges(p, l, r, k, at)
	s.paintDiamond(p, l, r, at)
	for i, letter := range l.letters {
		a := clamp01(s.shown[i].Value())
		if a <= 0 {
			continue
		}
		t := paint.Translate(geom.Pt(0, s.rise[i].Value()*letterRise*k))
		if i == letterCount-1 {
			// The T goes first at the end, shrinking away from under the
			// person.
			tg := clamp01(s.tGone.Value())
			a *= 1 - tg
			t = t.Mul(paint.Scale(1-0.6*tg, at(l.tMiddle)))
		}
		if a <= 0 {
			continue
		}
		pop := p.Push(t)
		paintPart(p, letter, r, func(pt *shape.Part) { pt.Fill = faded(ink, a) })
		pop()
	}
}

// paintWedges draws the wedges, sliding out from behind the diamond.
func (s *Splash) paintWedges(p *paint.Painter, l *logo, r geom.Rect, k float32, at func(geom.Point) geom.Point) {
	a := clamp01(s.wedges.Value())
	if a <= 0 {
		return
	}
	if a < 1 {
		defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: a})()
	}
	in := s.open.Value()
	for i, side := range []float32{1, -1} {
		// Each wedge starts in toward the diamond, and a little small.
		pop := p.Push(paint.Translate(geom.Pt(side*in*wedgeTravel*k, 0)).Mul(paint.Scale(1-0.2*clamp01(in), at(l.marks[i]))))
		paintPart(p, l.mark[i], r, nil)
		pop()
	}
}

// paintDiamond draws the diamond, popping in with a turn, its glow
// swelling.
func (s *Splash) paintDiamond(p *paint.Painter, l *logo, r geom.Rect, at func(geom.Point) geom.Point) {
	d := s.diamond.Value()
	if d <= 0 {
		return
	}
	c := at(l.centre)
	defer p.Push(paint.Scale(d, c).Mul(paint.Rotate((1-d)*turn, c)))()
	glow := s.glow.Value()
	paintPart(p, l.mark[partDiamond], r, func(pt *shape.Part) {
		if pt.FillGradient == nil {
			return
		}
		g := *pt.FillGradient
		g.To = g.From.Add(g.To.Sub(g.From).Mul(glow))
		pt.FillGradient = &g
	})
}

// paintPart paints the one part of fig into r, changed first by change
// where it is not nil.
func paintPart(p *paint.Painter, fig *shape.Figure, r geom.Rect, change func(*shape.Part)) {
	if change == nil {
		fig.Paint(p, r)
		return
	}
	pt := fig.Parts[0]
	change(&pt)
	one := shape.Figure{ViewBox: fig.ViewBox, Parts: []shape.Part{pt}}
	one.Paint(p, r)
}

// logoRect is where the logo of view box vb sits at rest in area: in
// the middle, as large as fits within 64% of its width, half its height
// and 520 logical pixels across.
func logoRect(vb, area geom.Rect) geom.Rect {
	size := area.Size()
	aspect := vb.Size().W / vb.Size().H
	w := min(0.64*size.W, 0.5*size.H*aspect, 520)
	if w <= 0 {
		return geom.Rect{}
	}
	h := w / aspect
	return geom.Rc(area.Min.X+(size.W-w)/2, area.Min.Y+(size.H-h)/2, w, h)
}

// inkOn is the letters' colour on bg: black on a light background and
// white on a dark one, as in the logo's two files.
func inkOn(bg color.NRGBA) color.NRGBA {
	// Relative luminance, from the colour's sRGB values made linear.
	lin := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	y := 0.2126*lin(bg.R) + 0.7152*lin(bg.G) + 0.0722*lin(bg.B)
	if y < 0.18 {
		return color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	return color.NRGBA{A: 0xff}
}

// faded is c at alpha a, from 0 to 1.
func faded(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A)*clamp01(a) + 0.5)
	return c
}

func clamp01(v float32) float32 { return min(max(v, 0), 1) }
