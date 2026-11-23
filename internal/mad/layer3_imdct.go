package mad

// fastsdct computes a 9-point SDCT-II; it writes every other slot of y.
func fastsdct(x *[9]fixed, y []fixed) {
	const (
		c0 = fixed(0x1f838b8d)  // 2 * cos( 1 * PI / 18)
		c1 = fixed(0x1bb67ae8)  // 2 * cos( 3 * PI / 18)
		c2 = fixed(0x18836fa3)  // 2 * cos( 4 * PI / 18)
		c3 = fixed(0x1491b752)  // 2 * cos( 5 * PI / 18)
		c4 = fixed(0x0af1d43a)  // 2 * cos( 7 * PI / 18)
		c5 = fixed(0x058e86a0)  // 2 * cos( 8 * PI / 18)
		c6 = fixed(-0x1e11f642) // 2 * cos(16 * PI / 18)
	)

	a0 := x[3] + x[5]
	a1 := x[3] - x[5]
	a2 := x[6] + x[2]
	a3 := x[6] - x[2]
	a4 := x[1] + x[7]
	a5 := x[1] - x[7]
	a6 := x[8] + x[0]
	a7 := x[8] - x[0]

	a8 := a0 + a2
	a9 := a0 - a2
	a10 := a0 - a6
	a11 := a2 - a6
	a12 := a8 + a6
	a13 := a1 - a3
	a14 := a13 + a7
	a15 := a3 + a7
	a16 := a1 - a7
	a17 := a1 + a3

	m0 := mul(a17, -c3)
	m1 := mul(a16, -c0)
	m2 := mul(a15, -c4)
	m3 := mul(a14, -c1)
	m4 := mul(a5, -c1)
	m5 := mul(a11, -c6)
	m6 := mul(a10, -c5)
	m7 := mul(a9, -c2)

	a18 := x[4] + a4
	a19 := 2*x[4] - a4
	a20 := a19 + m5
	a21 := a19 - m5
	a22 := a19 + m6
	a23 := m4 + m2
	a24 := m4 - m2
	a25 := m4 + m1

	// output to every other slot for convenience
	y[0] = a18 + a12
	y[2] = m0 - a25
	y[4] = m7 - a20
	y[6] = m3
	y[8] = a21 - m6
	y[10] = a24 - m1
	y[12] = a12 - 2*a18
	y[14] = a23 + m0
	y[16] = a22 + m7
}

// scale[i] = 2 * cos(PI * (2 * i + 1) / (2 * 18))
var sdctIIScale = [9]fixed{
	0x1fe0d3b4, 0x1ee8dd47, 0x1d007930,
	0x1a367e59, 0x16a09e66, 0x125abcf8,
	0x0d8616bc, 0x08483ee1, 0x02c9fad7,
}

func sdctII(x *[18]fixed, X *[18]fixed) {
	var tmp [9]fixed

	// divide the 18-point SDCT-II into two 9-point SDCT-IIs

	// even input butterfly
	for i := 0; i < 9; i++ {
		tmp[i] = x[i] + x[18-i-1]
	}
	fastsdct(&tmp, X[0:])

	// odd input butterfly and scaling; the reference build subtracts in 64
	// bits here (see mulWide), the only place where it does not wrap
	for i := 0; i < 9; i++ {
		tmp[i] = mulWide(int64(x[i])-int64(x[18-i-1]), sdctIIScale[i])
	}
	fastsdct(&tmp, X[1:])

	// output accumulation
	for i := 3; i < 18; i += 8 {
		X[i+0] -= X[(i+0)-2]
		X[i+2] -= X[(i+2)-2]
		X[i+4] -= X[(i+4)-2]
		X[i+6] -= X[(i+6)-2]
	}
}

// scale[i] = 2 * cos(PI * (2 * i + 1) / (4 * 18))
var dctIVScale = [18]fixed{
	0x1ff833fa, 0x1fb9ea93, 0x1f3dd120,
	0x1e84d969, 0x1d906bcf, 0x1c62648b,
	0x1afd100f, 0x1963268b, 0x1797c6a4,
	0x159e6f5b, 0x137af940, 0x11318ef3,
	0x0ec6a507, 0x0c3ef153, 0x099f61c5,
	0x06ed12c5, 0x042d4544, 0x0165547c,
}

func dctIV(y *[18]fixed, X *[18]fixed) {
	var tmp [18]fixed

	// scaling
	for i := 0; i < 18; i++ {
		tmp[i] = mul(y[i], dctIVScale[i])
	}

	// SDCT-II
	sdctII(&tmp, X)

	// scale reduction and output accumulation
	X[0] /= 2
	for i := 1; i < 17; i += 4 {
		X[i+0] = X[i+0]/2 - X[(i+0)-1]
		X[i+1] = X[i+1]/2 - X[(i+1)-1]
		X[i+2] = X[i+2]/2 - X[(i+2)-1]
		X[i+3] = X[i+3]/2 - X[(i+3)-1]
	}
	X[17] = X[17]/2 - X[16]
}

// imdct36 performs the X[18]->x[36] IMDCT with Szu-Wei Lee's algorithm.
func imdct36(x *[18]fixed, y *[36]fixed) {
	var tmp [18]fixed

	// DCT-IV
	dctIV(x, &tmp)

	// convert 18-point DCT-IV to 36-point IMDCT
	for i := 0; i < 9; i++ {
		y[i] = tmp[9+i]
	}
	for i := 9; i < 27; i++ {
		y[i] = -tmp[36-(9+i)-1]
	}
	for i := 27; i < 36; i++ {
		y[i] = -tmp[i-27]
	}
}

// imdctL is III_imdct_l: IMDCT and windowing for long blocks.
func imdctL(X *[18]fixed, z *[36]fixed, blockType uint8) {
	// IMDCT
	imdct36(X, z)

	// windowing
	switch blockType {
	case 0: // normal window
		for i := 0; i < 36; i++ {
			z[i] = mul(z[i], windowL[i])
		}
	case 1: // start block
		for i := 0; i < 18; i++ {
			z[i] = mul(z[i], windowL[i])
		}
		// (i = 18; i < 24; ++i) z[i] unchanged
		for i := 24; i < 30; i++ {
			z[i] = mul(z[i], windowS[i-18])
		}
		for i := 30; i < 36; i++ {
			z[i] = 0
		}
	case 3: // stop block
		for i := 0; i < 6; i++ {
			z[i] = 0
		}
		for i := 6; i < 12; i++ {
			z[i] = mul(z[i], windowS[i-6])
		}
		// (i = 12; i < 18; ++i) z[i] unchanged
		for i := 18; i < 36; i++ {
			z[i] = mul(z[i], windowL[i])
		}
	}
}

// imdctShort is III_imdct_s: IMDCT and windowing for short blocks.
func imdctShort(X *[18]fixed, z *[36]fixed) {
	var y [36]fixed

	// IMDCT
	for w := 0; w < 3; w++ {
		x := X[6*w : 6*w+6]
		yp := y[12*w : 12*w+12]
		si := 0

		for i := 0; i < 3; i++ {
			s := &imdctS[si]
			lo := mul(x[0], s[0]) + mul(x[1], s[1]) + mul(x[2], s[2]) +
				mul(x[3], s[3]) + mul(x[4], s[4]) + mul(x[5], s[5])
			yp[i] = lo
			yp[5-i] = -lo
			si++

			s = &imdctS[si]
			lo = mul(x[0], s[0]) + mul(x[1], s[1]) + mul(x[2], s[2]) +
				mul(x[3], s[3]) + mul(x[4], s[4]) + mul(x[5], s[5])
			yp[i+6] = lo
			yp[11-i] = lo
			si++
		}
	}

	// windowing, overlapping and concatenation
	for i := 0; i < 6; i++ {
		z[i+0] = 0
		z[i+6] = mul(y[0+0+i], windowS[i])
		z[i+12] = mul(y[0+6+i], windowS[6+i]) + mul(y[12+0+i], windowS[i])
		z[i+18] = mul(y[12+6+i], windowS[6+i]) + mul(y[24+0+i], windowS[i])
		z[i+24] = mul(y[24+6+i], windowS[6+i])
		z[i+30] = 0
	}
}

// overlap is III_overlap: overlap-add of windowed IMDCT outputs.
func overlap(output *[36]fixed, ov *[18]fixed, sample *[18][32]fixed, sb int) {
	for i := 0; i < 18; i++ {
		sample[i][sb] = output[i] + ov[i]
		ov[i] = output[i+18]
	}
}

// overlapZ is III_overlap_z: "overlap-add" of zero IMDCT outputs.
func overlapZ(ov *[18]fixed, sample *[18][32]fixed, sb int) {
	for i := 0; i < 18; i++ {
		sample[i][sb] = ov[i]
		ov[i] = 0
	}
}

// freqinver is III_freqinver: subband frequency inversion for odd lines.
func freqinver(sample *[18][32]fixed, sb int) {
	for i := 1; i < 18; i += 2 {
		sample[i][sb] = -sample[i][sb]
	}
}
