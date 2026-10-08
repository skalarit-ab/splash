// Command preview plays the "Made by Skalarit" intro in a window, as an
// app shows it, over a plain first screen. A tap, a click or a key on
// that screen plays it again, and -loop plays it again by itself.
//
//	go run ./cmd/preview
//	go run ./cmd/preview -background dark -size 390x800
//	go run ./cmd/preview -shot intro.png -at 1s
//	go run ./cmd/preview -frames /tmp/frames -fps 30
//
// -shot holds the intro at the moment -at and writes the window to a
// PNG file; -frames writes the whole intro, frame by frame, as PNG
// files, for pictures or a GIF.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
	"github.com/skalarit-ab/splash"
)

// options are the command's flags.
type options struct {
	size       geom.Size
	background color.NRGBA
	silent     bool
	loop       time.Duration
	runFor     time.Duration
	shot       string
	at         time.Duration
	frames     string
	fps        int
}

func main() {
	var o options
	size := flag.String("size", "900x600", "the window's size, as 390x800 for one shaped like a phone")
	bg := flag.String("background", "light", "light, dark, or a colour as #rrggbb")
	flag.BoolVar(&o.silent, "silent", false, "play the intro without its sound")
	flag.DurationVar(&o.loop, "loop", 0, "play the intro again this long after each end; zero waits for a tap or a key")
	flag.DurationVar(&o.runFor, "for", 0, "quit after this long; zero runs until the window closes")
	flag.StringVar(&o.shot, "shot", "", "hold the intro at -at, write the window to this PNG file, and quit")
	flag.DurationVar(&o.at, "at", time.Second, "the moment of the intro -shot holds")
	flag.StringVar(&o.frames, "frames", "", "write the intro, frame by frame, as PNG files to this folder, and quit")
	flag.IntVar(&o.fps, "fps", 30, "how many frames a second -frames writes")
	flag.Parse()
	var w, h float32
	if _, err := fmt.Sscanf(*size, "%gx%g", &w, &h); err != nil || w <= 0 || h <= 0 {
		log.Fatalf("preview: -size %q: want a width and a height, as 390x800", *size)
	}
	o.size = geom.Sz(w, h)
	c, err := parseBackground(*bg)
	if err != nil {
		log.Fatalf("preview: -background: %v", err)
	}
	o.background = c
	if o.fps <= 0 {
		log.Fatal("preview: -fps must be above zero")
	}
	if err := run(o); err != nil {
		log.Fatal(err)
	}
}

// parseBackground reads light, dark or #rrggbb.
func parseBackground(s string) (color.NRGBA, error) {
	switch s {
	case "light":
		return splash.Light, nil
	case "dark":
		return splash.Dark, nil
	}
	var c color.NRGBA
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &c.R, &c.G, &c.B); err != nil || len(s) != 7 || s[0] != '#' {
		return c, fmt.Errorf("%q: want light, dark or #rrggbb", s)
	}
	c.A = 0xff
	return c, nil
}

// replay asks for the intro again; holdAt holds it at a moment.
type (
	replay struct{}
	holdAt struct{ At time.Duration }
)

func run(o options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if o.runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.runFor)
		defer cancel()
	}
	pictures := o.shot != "" || o.frames != ""
	// Pictures are taken silent.
	var mix *audio.Mixer
	if !pictures {
		mix = audio.NewMixer()
		if _, err := speaker.Open(mix, speaker.Options{Name: "Skalarit intro"}); err != nil {
			log.Printf("preview: no sound: %v", err)
			mix = nil
		}
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "Made by Skalarit", Size: o.size, Instant: pictures})
		if err != nil {
			return fmt.Errorf("preview: %w", err)
		}
		splash.Register(w, mix)
		gunim.RegisterPatch(w, splash.View, func(s *splash.Splash, h holdAt, _ *gunim.UI) { s.Hold(h.At) })
		gunim.RegisterView(w, "home", func(color.NRGBA) *home { return newHome(o.background) }, nil)
		c := w.Client()
		if pictures {
			return takePictures(ctx, c, o)
		}
		return serve(ctx, c, o)
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// serve shows the intro over the home screen, as an app does, and
// again on a tap or a key there, or after -loop.
func serve(ctx context.Context, c gunim.Client, o options) error {
	intro := splash.Intro{Background: o.background, Silent: o.silent}
	if err := c.Mount(gunim.Root, "home", "home", o.background); err != nil {
		return err
	}
	_ = c.Focus("home")
	show := func() error { return c.Mount(gunim.Root, "intro", splash.View, intro) }
	if err := show(); err != nil {
		return err
	}
	var again <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-again:
			again = nil
			if err := show(); err != nil {
				return err
			}
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch in := ev.Intent.(type) {
			case splash.Done:
				log.Printf("preview: done, skipped %v", in.Skipped)
				if err := c.Unmount("intro"); err != nil {
					return err
				}
				if o.loop > 0 {
					again = time.After(o.loop)
				}
			case replay:
				if err := show(); err != nil {
					return err
				}
			}
		}
	}
}

// takePictures writes the -shot or the -frames, and closes the window.
func takePictures(ctx context.Context, c gunim.Client, o options) error {
	defer c.Close()
	if err := c.Mount(gunim.Root, "home", "home", o.background); err != nil {
		return err
	}
	if err := c.Mount(gunim.Root, "intro", splash.View, splash.Intro{Background: o.background}); err != nil {
		return err
	}
	// The window settles onto the screen first.
	select {
	case <-time.After(500 * time.Millisecond):
	case <-ctx.Done():
		return ctx.Err()
	}
	if o.shot != "" {
		return shotAt(ctx, c, o.at, o.shot)
	}
	if err := os.MkdirAll(o.frames, 0o755); err != nil {
		return err
	}
	step := time.Second / time.Duration(o.fps)
	for i := 0; time.Duration(i)*step <= splash.Length; i++ {
		path := filepath.Join(o.frames, fmt.Sprintf("frame-%03d.png", i))
		if err := shotAt(ctx, c, time.Duration(i)*step, path); err != nil {
			return err
		}
	}
	return nil
}

// shotAt holds the intro at the moment at and writes the window to a
// PNG file.
func shotAt(ctx context.Context, c gunim.Client, at time.Duration, path string) error {
	if err := c.Patch("intro", holdAt{At: at}); err != nil {
		return err
	}
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}

// home is the preview's first screen: the background, and a line
// saying how to play the intro again.
type home struct {
	bg   color.NRGBA
	hint *widget.Label
}

// hintInk is the hint's colour.
var hintInk = theme.Foreground("preview.hint", color.NRGBA{R: 0x80, G: 0x86, B: 0x90, A: 0xff})

func newHome(bg color.NRGBA) *home {
	l := widget.NewLabel("Tap, click or press a key to play the intro again")
	l.Color = hintInk
	return &home{bg: bg, hint: l}
}

// Children implements [gunim.Composite].
func (h *home) Children() []gunim.Node { return []gunim.Node{h.hint} }

// Layout implements [gunim.Node].
func (h *home) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	for kid := range kids.All {
		s := kid.Layout(gunim.Loose(geom.Sz(size.W*0.9, size.H)))
		kid.Place(geom.Pt((size.W-s.W)/2, size.H*0.8-s.H/2))
	}
	return size
}

// Paint implements [gunim.Node].
func (h *home) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(h.bg))
	for kid := range kids.All {
		kid.Paint(p)
	}
}

// Handle implements [gunim.Handler]: a press or a key plays the intro
// again.
func (h *home) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		u.Send(h, replay{})
		return true
	case input.KeyPress:
		if !e.Repeat {
			u.Send(h, replay{})
		}
		return true
	}
	return false
}

// Focusable implements [gunim.Focusable], so keys reach the home screen.
func (h *home) Focusable() bool { return true }
