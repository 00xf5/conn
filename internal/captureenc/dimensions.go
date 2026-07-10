package captureenc

// AlignEncodeDimensions rounds to H.264 macroblock boundaries (16px).
func AlignEncodeDimensions(w, h int) (int, int) {
	if w <= 0 {
		w = 1280
	}
	if h <= 0 {
		h = 720
	}
	w = (w + 15) & ^15
	if h&1 != 0 {
		h++
	}
	return w, h
}

// FitEncodeDimensions downscales for in-process HW encoders that cannot sustain
// full desktop resolution on low-power iGPUs (pixels scale ~linearly with encode cost).
func FitEncodeDimensions(w, h, maxW int) (int, int) {
	if maxW > 0 && w > maxW {
		h = h * maxW / w
		w = maxW
	}
	return AlignEncodeDimensions(w, h)
}
