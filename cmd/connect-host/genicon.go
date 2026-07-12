//go:build ignore

// Generates icon.ico for WorthyJoin-Host (desktop / taskbar / shortcut).
// Run: go run ./cmd/connect-host/genicon.go
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	out := filepath.Join("cmd", "connect-host", "icon.ico")
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	sizes := []int{16, 32, 48, 256}
	entries := make([]icoEntry, 0, len(sizes))
	for _, s := range sizes {
		pngBytes, err := encodePNG(render(s))
		if err != nil {
			panic(err)
		}
		entries = append(entries, icoEntry{size: s, png: pngBytes})
	}
	if err := writeICO(out, entries); err != nil {
		panic(err)
	}
	fmt.Println("wrote", out)
}

type icoEntry struct {
	size int
	png  []byte
}

func writeICO(path string, entries []icoEntry) error {
	var body bytes.Buffer
	offset := 6 + 16*len(entries)
	type dir struct {
		w, h  byte
		size  uint32
		off   uint32
	}
	dirs := make([]dir, len(entries))
	for i, e := range entries {
		w, h := e.size, e.size
		if w >= 256 {
			w = 0
		}
		if h >= 256 {
			h = 0
		}
		dirs[i] = dir{w: byte(w), h: byte(h), size: uint32(len(e.png)), off: uint32(offset)}
		offset += len(e.png)
		_, _ = body.Write(e.png)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_ = binary.Write(f, binary.LittleEndian, uint16(0)) // reserved
	_ = binary.Write(f, binary.LittleEndian, uint16(1)) // type icon
	_ = binary.Write(f, binary.LittleEndian, uint16(len(entries)))
	for _, d := range dirs {
		_, _ = f.Write([]byte{d.w, d.h, 0, 0})
		_ = binary.Write(f, binary.LittleEndian, uint16(1))  // planes
		_ = binary.Write(f, binary.LittleEndian, uint16(32)) // bit count
		_ = binary.Write(f, binary.LittleEndian, d.size)
		_ = binary.Write(f, binary.LittleEndian, d.off)
	}
	_, err = f.Write(body.Bytes())
	return err
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func render(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// Transparent outside the rounded tile.
	clear(img)

	accent := color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	accentDark := color.RGBA{R: 0x25, G: 0x63, B: 0xeb, A: 0xff}
	white := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}

	pad := float64(size) * 0.06
	radius := float64(size) * 0.22
	x0, y0 := pad, pad
	x1, y1 := float64(size)-pad, float64(size)-pad

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if !inRoundedRect(fx, fy, x0, y0, x1, y1, radius) {
				continue
			}
			// Subtle top→bottom shade so it reads as a real app tile.
			t := (fy - y0) / (y1 - y0)
			img.Set(x, y, lerp(accent, accentDark, t*0.55))
		}
	}

	cx := float64(size) / 2
	cy := float64(size) / 2
	outer := float64(size) * 0.28
	inner := float64(size) * 0.12
	drawStar(img, cx, cy, outer, inner, white)
	return img
}

func clear(img *image.RGBA) {
	for i := range img.Pix {
		img.Pix[i] = 0
	}
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

func inRoundedRect(x, y, x0, y0, x1, y1, r float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	// Inside the straight mid-bands.
	if x >= x0+r && x <= x1-r {
		return true
	}
	if y >= y0+r && y <= y1-r {
		return true
	}
	// Corner circles.
	corners := [...][2]float64{
		{x0 + r, y0 + r},
		{x1 - r, y0 + r},
		{x0 + r, y1 - r},
		{x1 - r, y1 - r},
	}
	for _, c := range corners {
		dx, dy := x-c[0], y-c[1]
		if dx*dx+dy*dy <= r*r {
			return true
		}
	}
	return false
}

func drawStar(img *image.RGBA, cx, cy, outer, inner float64, col color.RGBA) {
	const points = 5
	verts := make([][2]float64, points*2)
	for i := 0; i < points*2; i++ {
		ang := -math.Pi/2 + float64(i)*math.Pi/float64(points)
		r := outer
		if i%2 == 1 {
			r = inner
		}
		verts[i][0] = cx + math.Cos(ang)*r
		verts[i][1] = cy + math.Sin(ang)*r
	}
	minX, minY := int(cx-outer-1), int(cy-outer-1)
	maxX, maxY := int(cx+outer+1), int(cy+outer+1)
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	b := img.Bounds()
	if maxX >= b.Max.X {
		maxX = b.Max.X - 1
	}
	if maxY >= b.Max.Y {
		maxY = b.Max.Y - 1
	}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if pointInPolygon(float64(x)+0.5, float64(y)+0.5, verts) {
				img.Set(x, y, col)
			}
		}
	}
}

func pointInPolygon(x, y float64, poly [][2]float64) bool {
	inside := false
	j := len(poly) - 1
	for i := 0; i < len(poly); i++ {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		intersect := ((yi > y) != (yj > y)) &&
			(x < (xj-xi)*(y-yi)/(yj-yi+1e-12)+xi)
		if intersect {
			inside = !inside
		}
		j = i
	}
	return inside
}
