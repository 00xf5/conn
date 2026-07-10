package agent

// G.711 μ-law encode/decode (PCMU) — pure Go, no libopus.

func linearToMulaw(sample int16) byte {
	const (
		bias = 0x84
		clip = 32635
	)
	sign := byte(0)
	if sample < 0 {
		sign = 0x80
		sample = -sample
		if sample < 0 {
			sample = clip
		}
	}
	if sample > clip {
		sample = clip
	}
	sample = sample + bias
	exponent := byte(7)
	for expMask := int16(0x4000); (sample&expMask) == 0 && exponent > 0; exponent-- {
		expMask >>= 1
	}
	mantissa := byte((sample >> (exponent + 3)) & 0x0F)
	return ^(sign | (exponent << 4) | mantissa)
}

func mulawToLinear(mu byte) int16 {
	mu = ^mu
	sign := mu & 0x80
	exponent := (mu >> 4) & 0x07
	mantissa := mu & 0x0F
	sample := ((int16(mantissa) << 3) + 0x84) << exponent
	sample -= 0x84
	if sign != 0 {
		return -sample
	}
	return sample
}

func pcm16ToMulaw(pcm []int16) []byte {
	out := make([]byte, len(pcm))
	for i, s := range pcm {
		out[i] = linearToMulaw(s)
	}
	return out
}

func mulawToPCM16(mu []byte) []int16 {
	out := make([]int16, len(mu))
	for i, b := range mu {
		out[i] = mulawToLinear(b)
	}
	return out
}
