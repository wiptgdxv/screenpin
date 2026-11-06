package scroll

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"slices"
)

var ErrLimit = errors.New("长图达到像素上限")

type scrollDirection int32

const (
	scrollDown scrollDirection = iota
	scrollUp
	scrollRight
	scrollLeft
)

func (d scrollDirection) horizontal() bool { return d == scrollRight || d == scrollLeft }
func (d scrollDirection) reversed() bool   { return d == scrollUp || d == scrollLeft }

type scrollAddition struct {
	previousOffset int
	prepended      bool
}

// Composer tracks the stitched bounds and the current viewport on the detected axis.
type Composer struct {
	last       *image.RGBA
	pieces     []*image.RGBA
	height     int
	direction  scrollDirection
	top        int
	offset     int
	history    []scrollAddition
	axisLocked bool
}

// New starts a capture whose scroll axis is detected from the first movement.
func New(first image.Image) *Composer {
	return newDirectionComposer(first, scrollDown)
}

// Add updates the current viewport and stitches any newly exposed content.
func (s *Composer) Add(frame *image.RGBA) error {
	if frame.Bounds().Size() != s.frameSize() {
		return fmt.Errorf("选区尺寸发生变化")
	}
	if s.axisLocked {
		_, err := s.add(s.orient(frame))
		return err
	}
	match, err := detectScroll(s.last, frame)
	if match != nil {
		*s = *match
		s.axisLocked = true
	}
	return err
}

// AlignAfterUndo waits for the viewport to return to the last retained frame.
func (s *Composer) AlignAfterUndo(frame *image.RGBA) bool {
	if frame.Bounds().Size() != s.frameSize() || overlapError(s.last, s.orient(frame), 0) >= 1.2 {
		return false
	}
	if len(s.history) == 0 {
		*s = *New(s.Image())
	}
	return true
}

func (s *Composer) frameSize() image.Point {
	size := s.last.Bounds().Size()
	if s.direction.horizontal() {
		return image.Pt(size.Y, size.X)
	}
	return size
}

// Axis reports whether a scroll axis is known and whether it is horizontal.
func (s *Composer) Axis() (horizontal, known bool) {
	return s.direction.horizontal(), s.axisLocked
}

func cloneRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
func transposeRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			from, to := y*src.Stride+x*4, x*dst.Stride+y*4
			copy(dst.Pix[to:to+4], src.Pix[from:from+4])
		}
	}
	return dst
}

func reverseRows(img *image.RGBA) {
	width, height := img.Bounds().Dx()*4, img.Bounds().Dy()
	row := make([]byte, width)
	for y := 0; y < height/2; y++ {
		top, bottom := y*img.Stride, (height-1-y)*img.Stride
		copy(row, img.Pix[top:top+width])
		copy(img.Pix[top:top+width], img.Pix[bottom:bottom+width])
		copy(img.Pix[bottom:bottom+width], row)
	}
}

func newDirectionComposer(first image.Image, direction scrollDirection) *Composer {
	s := &Composer{direction: direction}
	s.last = s.orient(cloneRGBA(first))
	s.pieces = []*image.RGBA{s.last}
	s.height = s.last.Bounds().Dy()
	return s
}

// Normalize both axes to rows, with the first movement pointing down.
func (s *Composer) orient(frame *image.RGBA) *image.RGBA {
	if s.direction.horizontal() {
		frame = transposeRGBA(frame)
	} else if s.direction.reversed() {
		frame = cloneRGBA(frame)
	}
	if s.direction.reversed() {
		reverseRows(frame)
	}
	return frame
}

func (s *Composer) Undo() bool {
	if len(s.history) == 0 {
		return false
	}
	last := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	index := len(s.pieces) - 1
	if last.prepended {
		index = 0
	}
	removed := s.pieces[index].Bounds().Dy()
	s.pieces = slices.Delete(s.pieces, index, index+1)
	s.height -= removed
	if last.prepended {
		s.top += removed
	}
	s.offset = last.previousOffset
	w, h := s.last.Bounds().Dx(), s.last.Bounds().Dy()
	s.last = image.NewRGBA(image.Rect(0, 0, w, h))
	viewport := image.Rect(0, s.offset, w, s.offset+h)
	y := s.top
	for _, piece := range s.pieces {
		bounds := image.Rect(0, y, w, y+piece.Bounds().Dy())
		if visible := bounds.Intersect(viewport); !visible.Empty() {
			draw.Draw(s.last, visible.Sub(viewport.Min), piece, visible.Min.Sub(bounds.Min), draw.Src)
		}
		y = bounds.Max.Y
		if y >= viewport.Max.Y {
			break
		}
	}
	return true
}

func (s *Composer) Size() image.Point {
	if s.direction.horizontal() {
		return image.Pt(s.height, s.last.Bounds().Dx())
	}
	return image.Pt(s.last.Bounds().Dx(), s.height)
}

// Compare overlapping rows at their original pixels.
func overlapError(previous, next *image.RGBA, shift int) float64 {
	return sampledOverlapError(previous, next, shift, 96)
}

func sampledOverlapError(previous, next *image.RGBA, shift, samples int) float64 {
	w, h := previous.Bounds().Dx(), previous.Bounds().Dy()
	overlap := h - shift
	xStep, yStep := max(1, w/samples), max(1, overlap/samples)
	total, count := int64(0), int64(0)
	for y := 0; y < overlap; y += yStep {
		for x := 0; x < w; x += xStep {
			a := (y+shift)*previous.Stride + x*4
			b := y*next.Stride + x*4
			for channel := 0; channel < 3; channel++ {
				diff := int(previous.Pix[a+channel]) - int(next.Pix[b+channel])
				if diff < 0 {
					diff = -diff
				}
				total += int64(diff)
				count++
			}
		}
	}
	if count == 0 {
		return math.Inf(1)
	}
	return float64(total) / float64(count)
}

func (s *Composer) add(next *image.RGBA) (bool, error) {
	if next.Bounds().Size() != s.last.Bounds().Size() {
		return false, fmt.Errorf("选区尺寸发生变化")
	}
	if next.Bounds().Min != (image.Point{}) {
		next = cloneRGBA(next)
	}
	if overlapError(s.last, next, 0) < .35 {
		return false, nil
	}
	shift, err := findScrollShift(s.last, next, true)
	if err != nil {
		return false, err
	}
	return s.placeFrame(next, shift)
}

func findScrollShift(previous, next *image.RGBA, bidirectional bool) (int, error) {
	height := next.Bounds().Dy()
	maxShift := height - max(20, height/12)
	first := 1
	if bidirectional {
		first = -maxShift
	}
	scores := make([]float64, maxShift-first+1)
	best, bestScore := 0, math.Inf(1)
	for shift := first; shift <= maxShift; shift++ {
		score := math.Inf(1)
		if shift != 0 {
			earlier, later := previous, next
			if shift < 0 {
				earlier, later = next, previous
			}
			delta := absInt(shift)
			if sampledOverlapError(earlier, later, delta, 20) <= 9 {
				score = overlapError(earlier, later, delta)
			}
		}
		scores[shift-first] = score
		if score < bestScore {
			best, bestScore = shift, score
		}
	}
	if best == 0 || bestScore > 3 {
		return 0, fmt.Errorf("重叠不足 · 滚回上一段")
	}
	for i, score := range scores {
		if absInt(i+first-best) > 2 && score < bestScore+.8 {
			return 0, fmt.Errorf("内容重复 · 请再滚一点")
		}
	}
	return best, nil
}

func detectScroll(first, next *image.RGBA) (*Composer, error) {
	if overlapError(first, next, 0) < .35 {
		return nil, nil
	}
	var match *Composer
	for _, direction := range []scrollDirection{scrollDown, scrollUp, scrollRight, scrollLeft} {
		length := first.Bounds().Dy()
		if direction.horizontal() {
			length = first.Bounds().Dx()
		}
		if length < 80 {
			continue
		}
		candidate := newDirectionComposer(first, direction)
		oriented := candidate.orient(next)
		shift, err := findScrollShift(candidate.last, oriented, false)
		if err != nil {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("方向不明确，请再滚一点")
		}
		if _, err = candidate.placeFrame(oriented, shift); err != nil {
			return nil, err
		}
		match = candidate
	}
	if match == nil {
		return nil, fmt.Errorf("尚未识别滚动方向，请保留相邻画面的重叠")
	}
	return match, nil
}

// The viewport can move within the stitched image without adding any pixels.
func (s *Composer) placeFrame(next *image.RGBA, shift int) (bool, error) {
	offset := s.offset + shift
	height := next.Bounds().Dy()
	prepend := offset < s.top
	extra := max(s.top-offset, offset+height-(s.top+s.height))
	if extra <= 0 {
		s.offset, s.last = offset, next
		return false, nil
	}
	if int64(next.Bounds().Dx())*int64(s.height+extra) > 32*1024*1024 {
		return false, ErrLimit
	}
	strip := image.NewRGBA(image.Rect(0, 0, next.Bounds().Dx(), extra))
	sourceY := height - extra
	if prepend {
		sourceY = 0
	}
	draw.Draw(strip, strip.Bounds(), next, image.Pt(0, sourceY), draw.Src)
	s.history = append(s.history, scrollAddition{previousOffset: s.offset, prepended: prepend})
	if prepend {
		s.pieces = slices.Insert(s.pieces, 0, strip)
		s.top = offset
	} else {
		s.pieces = append(s.pieces, strip)
	}
	s.height += extra
	s.offset, s.last = offset, next
	return true, nil
}

func (s *Composer) Image() *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, s.last.Bounds().Dx(), s.height))
	y := 0
	for _, piece := range s.pieces {
		draw.Draw(result, image.Rect(0, y, result.Bounds().Dx(), y+piece.Bounds().Dy()), piece, image.Point{}, draw.Src)
		y += piece.Bounds().Dy()
	}
	if s.direction.reversed() {
		reverseRows(result)
	}
	if s.direction.horizontal() {
		return transposeRGBA(result)
	}
	return result
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
