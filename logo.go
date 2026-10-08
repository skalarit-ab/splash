package splash

import (
	_ "embed"
	"fmt"
	"sync"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/shape"
)

// logoSVG is Skalarit's logo. Its parts come in this order: the left
// wedge, the right wedge, the diamond, then the letters S, K, A, L, A,
// R, I and T. The letters are black in the file; the intro colours them
// for its background.
//
//go:embed logo/skalarit.svg
var logoSVG []byte

// The logo's parts, by their place in the file.
const (
	partLeft = iota
	partRight
	partDiamond
	partLetters
	letterCount = 8
)

// logo is the logo read into parts, each a figure of its own on the
// logo's whole view box, so each one moves alone and all line up at
// rest.
type logo struct {
	box geom.Rect
	// mark is the left wedge, the right wedge and the diamond.
	mark [3]*shape.Figure
	// letters are SKALARIT, left to right.
	letters [letterCount]*shape.Figure
	// centre is the middle of the diamond, which it pops from; marks are
	// the middles of the wedges; tMiddle is the T's middle, and seat the
	// middle of its top, where the little person sits: all in the view
	// box's units.
	centre, tMiddle, seat geom.Point
	marks                 [2]geom.Point
}

// theLogo reads the logo the first time it is asked for.
var theLogo = sync.OnceValues(func() (*logo, error) { return readLogo(logoSVG) })

// readLogo reads the logo from its SVG.
func readLogo(src []byte) (*logo, error) {
	f, err := shape.ParseSVG(src)
	if err != nil {
		return nil, fmt.Errorf("splash: the logo: %w", err)
	}
	if len(f.Parts) != partLetters+letterCount {
		return nil, fmt.Errorf("splash: the logo has %d parts, want %d", len(f.Parts), partLetters+letterCount)
	}
	l := &logo{box: f.ViewBox}
	one := func(i int) *shape.Figure { return &shape.Figure{ViewBox: f.ViewBox, Parts: f.Parts[i : i+1]} }
	for i := range l.mark {
		l.mark[i] = one(i)
	}
	for i := range l.letters {
		l.letters[i] = one(partLetters + i)
	}
	l.centre = f.Parts[partDiamond].Path.Bounds().Center()
	t := f.Parts[partLetters+letterCount-1].Path.Bounds()
	l.tMiddle, l.seat = t.Center(), geom.Pt(t.Center().X, t.Min.Y)
	l.marks = [2]geom.Point{f.Parts[partLeft].Path.Bounds().Center(), f.Parts[partRight].Path.Bounds().Center()}
	return l, nil
}
