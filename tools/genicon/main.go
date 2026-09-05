// genicon renders the EdgeHop menu-bar template icon. Two display outlines and
// opposing arrows match the app icon while remaining legible at 22 points.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const size = 44 // 2x for retina; template icon scales down to ~22pt

var black = color.RGBA{0, 0, 0, 255}

// filled polygon helper via point-in-polygon test.
func fillTriangle(img *image.RGBA, p1, p2, p3 [2]int) {
	minX, maxX := min(p1[0], p2[0], p3[0]), max(p1[0], p2[0], p3[0])
	minY, maxY := min(p1[1], p2[1], p3[1]), max(p1[1], p2[1], p3[1])
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if insideTriangle(float64(x)+0.5, float64(y)+0.5, p1, p2, p3) {
				img.Set(x, y, black)
			}
		}
	}
}

func insideTriangle(px, py float64, a, b, c [2]int) bool {
	d1 := sign(px, py, a, b)
	d2 := sign(px, py, b, c)
	d3 := sign(px, py, c, a)
	neg := (d1 < 0) || (d2 < 0) || (d3 < 0)
	pos := (d1 > 0) || (d2 > 0) || (d3 > 0)
	return !(neg && pos)
}

func sign(px, py float64, a, b [2]int) float64 {
	return (px-float64(b[0]))*(float64(a[1])-float64(b[1])) -
		(float64(a[0])-float64(b[0]))*(py-float64(b[1]))
}

func fillBar(img *image.RGBA, x0, y0, x1, y1 int) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.Set(x, y, black)
		}
	}
}

func strokeRect(img *image.RGBA, x0, y0, x1, y1, width int) {
	fillBar(img, x0, y0, x1, y0+width-1)
	fillBar(img, x0, y1-width+1, x1, y1)
	fillBar(img, x0, y0, x0+width-1, y1)
	fillBar(img, x1-width+1, y0, x1, y1)
}

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Two displays with a narrow seam between them.
	strokeRect(img, 2, 7, 15, 36, 3)
	strokeRect(img, 29, 7, 42, 36, 3)

	// Hop right across the upper half.
	fillBar(img, 12, 14, 32, 18)
	fillTriangle(img,
		[2]int{38, 16}, [2]int{30, 10}, [2]int{30, 22})

	// Hop back across the lower half.
	fillBar(img, 12, 26, 32, 30)
	fillTriangle(img,
		[2]int{6, 28}, [2]int{14, 22}, [2]int{14, 34})

	if err := os.MkdirAll("cmd/edgehop/assets", 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("cmd/edgehop/assets/menubar.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote cmd/edgehop/assets/menubar.png")
}
