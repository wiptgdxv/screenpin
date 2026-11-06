package scroll

import (
	"bytes"
	"image"
	"image/color"
	"math/rand"
	"testing"
)

var scrollDirectionLabels = []string{"向下", "向上", "向右", "向左"}

func scrollFixture() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 144, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 144; x++ {
			img.SetRGBA(x, y, color.RGBA{byte((x*13 + y*37 + (y/256)*97) % 256), byte((x*43 + y*y*3) % 256), byte((x*y*7 + y*23) % 256), 255})
		}
	}
	return img
}

func TestScrollStitchPreservesPixelsWithoutDuplicateRows(t *testing.T) {
	full := scrollFixture()
	composer := newDirectionComposer(full.SubImage(image.Rect(0, 0, 144, 200)), scrollDown)
	for _, offset := range []int{45, 110, 185, 265} {
		added, err := composer.add(cloneRGBA(full.SubImage(image.Rect(0, offset, 144, offset+200))))
		if err != nil || !added {
			t.Fatalf("offset %d: added=%v error=%v", offset, added, err)
		}
	}
	result := composer.Image()
	want := cloneRGBA(full.SubImage(image.Rect(0, 0, 144, 465)))
	if !bytes.Equal(result.Pix, want.Pix) || result.Bounds() != want.Bounds() {
		t.Fatal("stitched image has missing, duplicated or altered pixels")
	}
	if added, err := composer.add(composer.last); added || err != nil {
		t.Fatal("stationary frame appended")
	}
}

func TestHorizontalUndoAndRestitch(t *testing.T) {
	full := transposeRGBA(scrollFixture())
	frame := func(x int) *image.RGBA { return cloneRGBA(full.SubImage(image.Rect(x, 0, x+200, 144))) }
	composer := newDirectionComposer(frame(0), scrollRight)
	for _, x := range []int{45, 110} {
		if added, err := composer.add(composer.orient(frame(x))); !added || err != nil {
			t.Fatalf("offset %d: %v", x, err)
		}
	}
	if !composer.Undo() || composer.Size() != image.Pt(245, 144) {
		t.Fatalf("undo gave size %v", composer.Size())
	}
	if score := overlapError(composer.last, composer.orient(frame(45)), 0); score != 0 {
		t.Fatalf("undo restored wrong reference frame: %v", score)
	}
	if added, err := composer.add(composer.orient(frame(98))); !added || err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	want := cloneRGBA(full.SubImage(image.Rect(0, 0, 298, 144)))
	got := composer.Image()
	if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("horizontal stitch altered pixels")
	}
}

func TestDetectScrollWithNarrowOverlap(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 144, 400))
	rng := rand.New(rand.NewSource(15))
	_, _ = rng.Read(fixture.Pix)
	for i := 3; i < len(fixture.Pix); i += 4 {
		fixture.Pix[i] = 255
	}
	for _, direction := range []scrollDirection{scrollDown, scrollUp, scrollRight, scrollLeft} {
		t.Run(scrollDirectionLabels[direction], func(t *testing.T) {
			full := fixture
			region := func(start, end int) image.Rectangle { return image.Rect(0, start, 144, end) }
			if direction.horizontal() {
				full = transposeRGBA(full)
				region = func(start, end int) image.Rectangle { return image.Rect(start, 0, end, 144) }
			}
			first, next := 0, 174
			if direction.reversed() {
				first, next = next, first
			}
			frame := func(offset int) *image.RGBA { return cloneRGBA(full.SubImage(region(offset, offset+200))) }
			match, err := detectScroll(frame(first), frame(next))
			if err != nil || match == nil || match.direction != direction {
				t.Fatalf("direction %d: match=%v error=%v", direction, match != nil, err)
			}
			want := cloneRGBA(full.SubImage(region(0, 374)))
			got := match.Image()
			if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("fast scroll changed the pixels")
			}
		})
	}
}

func TestScrollExtendsBothEndsWithoutDuplicatingVisitedContent(t *testing.T) {
	for _, direction := range []scrollDirection{scrollUp, scrollLeft} {
		t.Run(scrollDirectionLabels[direction], func(t *testing.T) {
			full := scrollFixture()
			region := func(start, end int) image.Rectangle { return image.Rect(0, start, 144, end) }
			if direction.horizontal() {
				full = transposeRGBA(full)
				region = func(start, end int) image.Rectangle { return image.Rect(start, 0, end, 144) }
			}
			frame := func(offset int) *image.RGBA { return cloneRGBA(full.SubImage(region(offset, offset+200))) }
			composer, err := detectScroll(frame(90), frame(45))
			if err != nil || composer == nil || composer.direction != direction {
				t.Fatalf("direction: %v", err)
			}
			for _, step := range []struct {
				offset int
				adds   bool
			}{{0, true}, {45, false}, {90, false}, {135, true}, {180, true}} {
				added, err := composer.add(composer.orient(frame(step.offset)))
				if added != step.adds || err != nil {
					t.Fatalf("offset %d: added=%v, error=%v", step.offset, added, err)
				}
			}
			assertImage := func(end int) {
				t.Helper()
				got, want := composer.Image(), cloneRGBA(full.SubImage(region(0, end)))
				if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
					t.Fatal("incorrect stitch pixels or extent")
				}
			}
			assertImage(380)
			if !composer.Undo() {
				t.Fatal("undo failed")
			}
			assertImage(335)
			if score := overlapError(composer.last, composer.orient(frame(135)), 0); score != 0 {
				t.Fatalf("wrong undo viewport: %v", score)
			}
		})
	}
}

func TestReverseScrollPreservesImageOrder(t *testing.T) {
	for _, direction := range []scrollDirection{scrollUp, scrollLeft} {
		t.Run(scrollDirectionLabels[direction], func(t *testing.T) {
			full := scrollFixture()
			region := func(start, end int) image.Rectangle { return image.Rect(0, start, 144, end) }
			if direction.horizontal() {
				full = transposeRGBA(full)
				region = func(start, end int) image.Rectangle { return image.Rect(start, 0, end, 144) }
			}
			frame := func(offset int) *image.RGBA { return cloneRGBA(full.SubImage(region(offset, offset+200))) }
			composer := newDirectionComposer(frame(190), direction)
			for _, offset := range []int{140, 70, 0} {
				if added, err := composer.add(composer.orient(frame(offset))); !added || err != nil {
					t.Fatalf("offset %d: %v", offset, err)
				}
			}
			if !composer.Undo() {
				t.Fatal("undo failed")
			}
			got, want := composer.Image(), cloneRGBA(full.SubImage(region(70, 390)))
			if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("reverse stitch or undo changed the pixel order")
			}
		})
	}
}

func TestScrollRejectsUnrelatedAndAmbiguousFrames(t *testing.T) {
	full := scrollFixture()
	first := cloneRGBA(full.SubImage(image.Rect(0, 0, 144, 200)))
	composer := newDirectionComposer(first, scrollDown)
	if added, err := composer.add(cloneRGBA(full.SubImage(image.Rect(0, 450, 144, 650)))); added || err == nil {
		t.Fatal("non-overlapping content was accepted")
	}
	repeated := image.NewRGBA(image.Rect(0, 0, 144, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 144; x++ {
			v := byte((y % 20) * 12)
			repeated.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}
	shifted := image.NewRGBA(repeated.Bounds())
	for y := 0; y < 200; y++ {
		for x := 0; x < 144; x++ {
			shifted.SetRGBA(x, y, repeated.RGBAAt(x, (y+5)%200))
		}
	}
	composer = newDirectionComposer(repeated, scrollDown)
	if added, err := composer.add(shifted); added || err == nil {
		t.Fatal("ambiguous repeating content was accepted")
	}
}
