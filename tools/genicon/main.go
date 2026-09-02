// genicon renders the menu-bar template icon for the tray app. It draws a
// simple bidirectional-control glyph (two opposing arrows) in black + alpha,
// which macOS treats as a template image and recolors automatically.
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
	minX, maxX := min3(p1[0], p2[0], p3[0]), max3(p1[0], p2[0], p3[0])
	minY, maxY := min3(p1[1], p2[1], p3[1]), max3(p1[1], p2[1], p3[1])
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

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func max3(a, b, c int) int {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func fillBar(img *image.RGBA, x0, y0, x1, y1 int) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			img.Set(x, y, black)
		}
	}
}

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Left arrow (pointing left) on the lower-left.
	fillTriangle(img,
		[2]int{6, 34}, [2]int{26, 24}, [2]int{26, 44})
	// Right arrow (pointing right) on the upper-right.
	fillTriangle(img,
		[2]int{38, 10}, [2]int{18, 0}, [2]int{18, 20})
	// Connecting diagonal bars to suggest an exchange of control.
	fillBar(img, 20, 6, 24, 20)
	fillBar(img, 20, 24, 24, 38)

	if err := os.MkdirAll("cmd/uc-tray/assets", 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create("cmd/uc-tray/assets/menubar.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote cmd/uc-tray/assets/menubar.png")
}
