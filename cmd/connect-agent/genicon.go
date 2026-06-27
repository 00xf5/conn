//go:build ignore

package main

import (
	"encoding/binary"
	"os"
)

func main() {
	const w, h = 16, 16
	xor := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		xor[i*4+0] = 246 // B
		xor[i*4+1] = 130 // G
		xor[i*4+2] = 59  // R
		xor[i*4+3] = 255 // A
	}
	andMask := make([]byte, w*4) // 16 rows, 4 bytes padded per row

	bih := make([]byte, 40)
	binary.LittleEndian.PutUint32(bih[0:], 40)
	binary.LittleEndian.PutUint32(bih[4:], w)
	binary.LittleEndian.PutUint32(bih[8:], h*2)
	binary.LittleEndian.PutUint16(bih[12:], 1)
	binary.LittleEndian.PutUint16(bih[14:], 32)

	image := append(append(bih, xor...), andMask...)
	size := uint32(len(image))
	offset := uint32(6 + 16)

	f, err := os.Create("cmd/connect-agent/icon.ico")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// ICONDIR
	_ = binary.Write(f, binary.LittleEndian, uint16(0))
	_ = binary.Write(f, binary.LittleEndian, uint16(1))
	_ = binary.Write(f, binary.LittleEndian, uint16(1))

	// ICONDIRENTRY
	_, _ = f.Write([]byte{w, h, 0, 0, 1, 0, 32, 0})
	_ = binary.Write(f, binary.LittleEndian, size)
	_ = binary.Write(f, binary.LittleEndian, offset)

	_, _ = f.Write(image)
}
