package image

import (
	_ "embed"
	"fmt"
	stdimage "image"
	"math"
	"sync"

	"github.com/disintegration/imaging"
	pigo "github.com/esimov/pigo/core"
	"github.com/ppxb/miyabi/internal/domain"
)

// PosterVersion invalidates generated posters when their composition changes.
const PosterVersion = 2

// Pigo's MIT-licensed model is embedded so cropping needs no runtime downloads.
// Source: esimov/pigo, commit 7465ed14de4797ad2e7db017b46c794153e9e7ab.
//
//go:embed cascade/facefinder
var faceCascade []byte

var posterClassifier = sync.OnceValues(func() (*pigo.Pigo, error) {
	return pigo.NewPigo().Unpack(faceCascade)
})

type posterFace struct {
	bounds stdimage.Rectangle
	weight float64
}

func cropPoster(source stdimage.Image, layout domain.CoverLayout) (stdimage.Image, error) {
	region := posterRegion(source.Bounds(), layout)
	if region != source.Bounds() {
		// Exclude the back cover before detection so its stills cannot affect
		// face size, clustering or the final composition.
		source = imaging.Crop(source, region)
	}
	bounds := source.Bounds()
	if bounds.Dx()*3 == bounds.Dy()*2 {
		return source, nil
	}
	faces, err := detectPosterFaces(source)
	if err != nil {
		return nil, err
	}
	return imaging.Crop(source, posterWindow(bounds, faces, layout)), nil
}

func posterRegion(bounds stdimage.Rectangle, layout domain.CoverLayout) stdimage.Rectangle {
	if layout == domain.CoverJacket && bounds.Dx() > bounds.Dy() {
		bounds.Min.X += bounds.Dx() * 45 / 100
	}
	return bounds
}

// PosterSize measures the usable original pixels under the same layout policy
// used for cropping. It never treats upscaling as additional detail.
func PosterSize(width, height int, layout domain.CoverLayout) (int, int) {
	region := posterRegion(stdimage.Rect(0, 0, width, height), layout)
	return min(region.Dx(), region.Dy()*2/3), min(region.Dy(), region.Dx()*3/2)
}

func detectPosterFaces(source stdimage.Image) ([]posterFace, error) {
	const maxSize, minFaceSize = 650, 20
	small := imaging.Fit(source, maxSize, maxSize, imaging.Lanczos)
	w, h := small.Bounds().Dx(), small.Bounds().Dy()
	if min(w, h) < minFaceSize {
		return nil, nil
	}
	classifier, err := posterClassifier()
	if err != nil {
		return nil, fmt.Errorf("load face detector: %w", err)
	}
	// Match MetaTube's upright, tilted and sideways searches. Three bounded
	// workers share the read-only classifier; each owns its pixels and results.
	var detections [3][]pigo.Detection
	var workers sync.WaitGroup
	for rotation := range detections {
		workers.Go(func() {
			img := small
			switch rotation {
			case 1:
				img = imaging.Rotate90(small)
			case 2:
				img = imaging.Rotate270(small)
			}
			params := pigo.CascadeParams{
				MinSize: minFaceSize, MaxSize: min(w, h), ShiftFactor: 0.1, ScaleFactor: 1.08,
				ImageParams: pigo.ImageParams{Pixels: posterPixels(img),
					Rows: img.Bounds().Dy(), Cols: img.Bounds().Dx(), Dim: img.Bounds().Dx()},
			}
			for _, angle := range []float64{0, 0.13, 0.87} {
				for _, face := range classifier.RunCascade(params, angle) {
					switch rotation {
					case 1:
						face.Row, face.Col = face.Col, w-1-face.Row
					case 2:
						face.Row, face.Col = h-1-face.Col, face.Row
					}
					detections[rotation] = append(detections[rotation], face)
				}
			}
		})
	}
	workers.Wait()
	all := append(append(detections[0], detections[1]...), detections[2]...)
	bounds := source.Bounds()
	sx, sy := float64(bounds.Dx())/float64(w), float64(bounds.Dy())/float64(h)
	var faces []posterFace
	for _, face := range classifier.ClusterDetections(all, 0.2) {
		if face.Q < 5 {
			continue
		}
		// Leave room around the detected face, including the forehead. Map back
		// to the original pixels; the reduced image is never used for output.
		radius := float64(face.Scale) * 0.65
		box := stdimage.Rect(
			int(math.Floor((float64(face.Col)-radius)*sx)), int(math.Floor((float64(face.Row)-radius)*sy)),
			int(math.Ceil((float64(face.Col)+radius)*sx)), int(math.Ceil((float64(face.Row)+radius)*sy)),
		).Add(bounds.Min).Intersect(bounds)
		if !box.Empty() {
			faces = append(faces, posterFace{bounds: box, weight: float64(face.Scale) * float64(face.Q)})
		}
	}
	return faces, nil
}

// Read NRGBA bytes directly to avoid allocating an interface color per pixel.
func posterPixels(img *stdimage.NRGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	pixels := make([]byte, w*h)
	for y := range h {
		row := img.Pix[y*img.Stride:]
		for x := range w {
			r, g, b, a := uint32(row[x*4]), uint32(row[x*4+1]), uint32(row[x*4+2]), uint32(row[x*4+3])
			pixels[y*w+x] = uint8((299*r + 587*g + 114*b) * a / (1000 * 255))
		}
	}
	return pixels
}

// Score complete faces more highly than fragments. Unlike averaging every
// detected face, this cannot choose empty space between two distant subjects.
func posterWindow(bounds stdimage.Rectangle, faces []posterFace, layout domain.CoverLayout) stdimage.Rectangle {
	w, h := bounds.Dx(), bounds.Dy()
	horizontal := w*3 > h*2
	length, start, end := max(1, h*2/3), bounds.Min.X, bounds.Max.X
	if !horizontal {
		length, start, end = max(1, w*3/2), bounds.Min.Y, bounds.Max.Y
	}
	window := func(pos int) stdimage.Rectangle {
		pos = max(start, min(pos, end-length))
		if horizontal {
			return stdimage.Rect(pos, bounds.Min.Y, pos+length, bounds.Max.Y)
		}
		return stdimage.Rect(bounds.Min.X, pos, bounds.Max.X, pos+length)
	}
	axis := func(rect stdimage.Rectangle) (int, int) {
		if horizontal {
			return rect.Min.X, rect.Max.X
		}
		return rect.Min.Y, rect.Max.Y
	}
	// Jackets keep the right-hand front; single images default to the center.
	// Tall portraits use the top edge instead of cutting off heads.
	position := start
	if horizontal {
		position = (start + end - length) / 2
		if layout == domain.CoverJacket {
			position = end - length
		}
	}
	best, bestScore := window(position), 0.0
	score := func(rect stdimage.Rectangle) float64 {
		var total float64
		for _, face := range faces {
			visible := face.bounds.Intersect(rect)
			fraction := float64(visible.Dx()*visible.Dy()) / float64(face.bounds.Dx()*face.bounds.Dy())
			total += face.weight * fraction * fraction
		}
		return total
	}
	consider := func(pos int) {
		rect := window(pos)
		if value := score(rect); value > bestScore {
			best, bestScore = rect, value
		}
	}
	consider(position)
	for _, face := range faces {
		lo, hi := axis(face.bounds)
		for _, pos := range []int{(lo + hi - length) / 2, lo, hi - length} {
			consider(pos)
		}
	}
	// Center the selected group while keeping every fully retained face inside.
	var center, weight float64
	low, high := start, end
	for _, face := range faces {
		if face.bounds.In(best) {
			lo, hi := axis(face.bounds)
			center += float64(lo+hi) / 2 * face.weight
			weight += face.weight
			low, high = max(low, hi-length), min(high, lo)
		}
	}
	if weight > 0 {
		pos := max(low, min(int(math.Round(center/weight))-length/2, high))
		if centered := window(pos); score(centered) >= bestScore {
			best = centered
		}
	}
	return best
}
