package mad

// PCM is libmad's mad_pcm: one frame of synthesized samples.
type PCM struct {
	SampleRate uint32
	Channels   int
	Length     int
	Samples    [2][1152]fixed
}

// Synth is libmad's mad_synth: the polyphase filterbank state.
type Synth struct {
	filter [2][2][2][16][8]fixed
	phase  uint32
	PCM    PCM
}

// costab[i] = cos(PI / (2 * 32) * i)
const (
	costab1  = fixed(0x0ffb10f2)
	costab2  = fixed(0x0fec46d2)
	costab3  = fixed(0x0fd3aac0)
	costab4  = fixed(0x0fb14be8)
	costab5  = fixed(0x0f853f7e)
	costab6  = fixed(0x0f4fa0ab)
	costab7  = fixed(0x0f109082)
	costab8  = fixed(0x0ec835e8)
	costab9  = fixed(0x0e76bd7a)
	costab10 = fixed(0x0e1c5979)
	costab11 = fixed(0x0db941a3)
	costab12 = fixed(0x0d4db315)
	costab13 = fixed(0x0cd9f024)
	costab14 = fixed(0x0c5e4036)
	costab15 = fixed(0x0bdaef91)
	costab16 = fixed(0x0b504f33)
	costab17 = fixed(0x0abeb49a)
	costab18 = fixed(0x0a267993)
	costab19 = fixed(0x0987fbfe)
	costab20 = fixed(0x08e39d9d)
	costab21 = fixed(0x0839c3cd)
	costab22 = fixed(0x078ad74e)
	costab23 = fixed(0x06d74402)
	costab24 = fixed(0x061f78aa)
	costab25 = fixed(0x0563e69d)
	costab26 = fixed(0x04a5018c)
	costab27 = fixed(0x03e33f2f)
	costab28 = fixed(0x031f1708)
	costab29 = fixed(0x0259020e)
	costab30 = fixed(0x01917a6c)
	costab31 = fixed(0x00c8fb30)
)

// dct32 performs the fast in[32]->out[32] DCT.
func dct32(in *[32]fixed, slot uint32, lo, hi *[16][8]fixed) {
	t0 := in[0] + in[31]
	t16 := mul(in[0]-in[31], costab1)
	t1 := in[15] + in[16]
	t17 := mul(in[15]-in[16], costab31)

	t41 := t16 + t17
	t59 := mul(t16-t17, costab2)
	t33 := t0 + t1
	t50 := mul(t0-t1, costab2)

	t2 := in[7] + in[24]
	t18 := mul(in[7]-in[24], costab15)
	t3 := in[8] + in[23]
	t19 := mul(in[8]-in[23], costab17)

	t42 := t18 + t19
	t60 := mul(t18-t19, costab30)
	t34 := t2 + t3
	t51 := mul(t2-t3, costab30)

	t4 := in[3] + in[28]
	t20 := mul(in[3]-in[28], costab7)
	t5 := in[12] + in[19]
	t21 := mul(in[12]-in[19], costab25)

	t43 := t20 + t21
	t61 := mul(t20-t21, costab14)
	t35 := t4 + t5
	t52 := mul(t4-t5, costab14)

	t6 := in[4] + in[27]
	t22 := mul(in[4]-in[27], costab9)
	t7 := in[11] + in[20]
	t23 := mul(in[11]-in[20], costab23)

	t44 := t22 + t23
	t62 := mul(t22-t23, costab18)
	t36 := t6 + t7
	t53 := mul(t6-t7, costab18)

	t8 := in[1] + in[30]
	t24 := mul(in[1]-in[30], costab3)
	t9 := in[14] + in[17]
	t25 := mul(in[14]-in[17], costab29)

	t45 := t24 + t25
	t63 := mul(t24-t25, costab6)
	t37 := t8 + t9
	t54 := mul(t8-t9, costab6)

	t10 := in[6] + in[25]
	t26 := mul(in[6]-in[25], costab13)
	t11 := in[9] + in[22]
	t27 := mul(in[9]-in[22], costab19)

	t46 := t26 + t27
	t64 := mul(t26-t27, costab26)
	t38 := t10 + t11
	t55 := mul(t10-t11, costab26)

	t12 := in[2] + in[29]
	t28 := mul(in[2]-in[29], costab5)
	t13 := in[13] + in[18]
	t29 := mul(in[13]-in[18], costab27)

	t47 := t28 + t29
	t65 := mul(t28-t29, costab10)
	t39 := t12 + t13
	t56 := mul(t12-t13, costab10)

	t14 := in[5] + in[26]
	t30 := mul(in[5]-in[26], costab11)
	t15 := in[10] + in[21]
	t31 := mul(in[10]-in[21], costab21)

	t48 := t30 + t31
	t66 := mul(t30-t31, costab22)
	t40 := t14 + t15
	t57 := mul(t14-t15, costab22)

	t69 := t33 + t34
	t89 := mul(t33-t34, costab4)
	t70 := t35 + t36
	t90 := mul(t35-t36, costab28)
	t71 := t37 + t38
	t91 := mul(t37-t38, costab12)
	t72 := t39 + t40
	t92 := mul(t39-t40, costab20)
	t73 := t41 + t42
	t94 := mul(t41-t42, costab4)
	t74 := t43 + t44
	t95 := mul(t43-t44, costab28)
	t75 := t45 + t46
	t96 := mul(t45-t46, costab12)
	t76 := t47 + t48
	t97 := mul(t47-t48, costab20)

	t78 := t50 + t51
	t100 := mul(t50-t51, costab4)
	t79 := t52 + t53
	t101 := mul(t52-t53, costab28)
	t80 := t54 + t55
	t102 := mul(t54-t55, costab12)
	t81 := t56 + t57
	t103 := mul(t56-t57, costab20)

	t83 := t59 + t60
	t106 := mul(t59-t60, costab4)
	t84 := t61 + t62
	t107 := mul(t61-t62, costab28)
	t85 := t63 + t64
	t108 := mul(t63-t64, costab12)
	t86 := t65 + t66
	t109 := mul(t65-t66, costab20)

	t113 := t69 + t70
	t114 := t71 + t72

	/*  0 */
	hi[15][slot] = t113 + t114
	/* 16 */
	lo[0][slot] = mul(t113-t114, costab16)

	t115 := t73 + t74
	t116 := t75 + t76

	t32 := t115 + t116

	/*  1 */
	hi[14][slot] = t32

	t118 := t78 + t79
	t119 := t80 + t81

	t58 := t118 + t119

	/*  2 */
	hi[13][slot] = t58

	t121 := t83 + t84
	t122 := t85 + t86

	t67 := t121 + t122

	t49 := (t67 * 2) - t32

	/*  3 */
	hi[12][slot] = t49

	t125 := t89 + t90
	t126 := t91 + t92

	t93 := t125 + t126

	/*  4 */
	hi[11][slot] = t93

	t128 := t94 + t95
	t129 := t96 + t97

	t98 := t128 + t129

	t68 := (t98 * 2) - t49

	/*  5 */
	hi[10][slot] = t68

	t132 := t100 + t101
	t133 := t102 + t103

	t104 := t132 + t133

	t82 := (t104 * 2) - t58

	/*  6 */
	hi[9][slot] = t82

	t136 := t106 + t107
	t137 := t108 + t109

	t110 := t136 + t137

	t87 := (t110 * 2) - t67

	t77 := (t87 * 2) - t68

	/*  7 */
	hi[8][slot] = t77

	t141 := mul(t69-t70, costab8)
	t142 := mul(t71-t72, costab24)
	t143 := t141 + t142

	/*  8 */
	hi[7][slot] = t143
	/* 24 */
	lo[8][slot] = (mul(t141-t142, costab16) * 2) - t143

	t144 := mul(t73-t74, costab8)
	t145 := mul(t75-t76, costab24)
	t146 := t144 + t145

	t88 := (t146 * 2) - t77

	/*  9 */
	hi[6][slot] = t88

	t148 := mul(t78-t79, costab8)
	t149 := mul(t80-t81, costab24)
	t150 := t148 + t149

	t105 := (t150 * 2) - t82

	/* 10 */
	hi[5][slot] = t105

	t152 := mul(t83-t84, costab8)
	t153 := mul(t85-t86, costab24)
	t154 := t152 + t153

	t111 := (t154 * 2) - t87

	t99 := (t111 * 2) - t88

	/* 11 */
	hi[4][slot] = t99

	t157 := mul(t89-t90, costab8)
	t158 := mul(t91-t92, costab24)
	t159 := t157 + t158

	t127 := (t159 * 2) - t93

	/* 12 */
	hi[3][slot] = t127

	t160 := (mul(t125-t126, costab16) * 2) - t127

	/* 20 */
	lo[4][slot] = t160
	/* 28 */
	lo[12][slot] = (((mul(t157-t158, costab16) * 2) - t159) * 2) - t160

	t161 := mul(t94-t95, costab8)
	t162 := mul(t96-t97, costab24)
	t163 := t161 + t162

	t130 := (t163 * 2) - t98

	t112 := (t130 * 2) - t99

	/* 13 */
	hi[2][slot] = t112

	t164 := (mul(t128-t129, costab16) * 2) - t130

	t166 := mul(t100-t101, costab8)
	t167 := mul(t102-t103, costab24)
	t168 := t166 + t167

	t134 := (t168 * 2) - t104

	t120 := (t134 * 2) - t105

	/* 14 */
	hi[1][slot] = t120

	t135 := (mul(t118-t119, costab16) * 2) - t120

	/* 18 */
	lo[2][slot] = t135

	t169 := (mul(t132-t133, costab16) * 2) - t134

	t151 := (t169 * 2) - t135

	/* 22 */
	lo[6][slot] = t151

	t170 := (((mul(t148-t149, costab16) * 2) - t150) * 2) - t151

	/* 26 */
	lo[10][slot] = t170
	/* 30 */
	lo[14][slot] = (((((mul(t166-t167, costab16) * 2) -
		t168) * 2) - t169) * 2) - t170

	t171 := mul(t106-t107, costab8)
	t172 := mul(t108-t109, costab24)
	t173 := t171 + t172

	t138 := (t173 * 2) - t110

	t123 := (t138 * 2) - t111

	t139 := (mul(t121-t122, costab16) * 2) - t123

	t117 := (t123 * 2) - t112

	/* 15 */
	hi[0][slot] = t117

	t124 := (mul(t115-t116, costab16) * 2) - t117

	/* 17 */
	lo[1][slot] = t124

	t131 := (t139 * 2) - t124

	/* 19 */
	lo[3][slot] = t131

	t140 := (t164 * 2) - t131

	/* 21 */
	lo[5][slot] = t140

	t174 := (mul(t136-t137, costab16) * 2) - t138

	t155 := (t174 * 2) - t139

	t147 := (t155 * 2) - t140

	/* 23 */
	lo[7][slot] = t147

	t156 := (((mul(t144-t145, costab16) * 2) - t146) * 2) - t147

	/* 25 */
	lo[9][slot] = t156

	t175 := (((mul(t152-t153, costab16) * 2) - t154) * 2) - t155

	t165 := (t175 * 2) - t156

	/* 27 */
	lo[11][slot] = t165

	t176 := (((((mul(t161-t162, costab16) * 2) -
		t163) * 2) - t164) * 2) - t165

	/* 29 */
	lo[13][slot] = t176
	/* 31 */
	lo[15][slot] = (((((((mul(t171-t172, costab16) * 2) -
		t173) * 2) - t174) * 2) - t175) * 2) - t176
}

// mulD is mad_f_mul inside synth.c, where MAD_F_SCALEBITS is redefined to
// MAD_F_FRACBITS - 12 to match the preshifted D[] table.
func mulD(x, y fixed) fixed {
	const scalebits = FracBits - 12
	return fixed((int64(x)*int64(y) + 1<<(scalebits-1)) >> scalebits)
}

// synthFull is synth_full: full frequency PCM synthesis.
func (sy *Synth) synthFull(f *Frame, nch, ns int) {
	for ch := 0; ch < nch; ch++ {
		sbsample := &f.sbsample[ch]
		filter := &sy.filter[ch]
		phase := sy.phase
		pcm := &sy.PCM.Samples[ch]
		p1 := 0

		for s := 0; s < ns; s++ {
			dct32(&sbsample[s], phase>>1, &filter[0][phase&1], &filter[1][phase&1])

			pe := phase &^ 1
			po := ((phase - 1) & 0xf) | 1

			// calculate 32 samples
			fe := &filter[0][phase&1]
			fx := &filter[0][^phase&1]
			fo := &filter[1][^phase&1]

			d := &synthD[0]
			x := &fx[0]
			e := &fe[0]

			lo := -(mulD(x[0], d[po+0]) + mulD(x[1], d[po+14]) +
				mulD(x[2], d[po+12]) + mulD(x[3], d[po+10]) +
				mulD(x[4], d[po+8]) + mulD(x[5], d[po+6]) +
				mulD(x[6], d[po+4]) + mulD(x[7], d[po+2]))
			lo += mulD(e[0], d[pe+0]) + mulD(e[1], d[pe+14]) +
				mulD(e[2], d[pe+12]) + mulD(e[3], d[pe+10]) +
				mulD(e[4], d[pe+8]) + mulD(e[5], d[pe+6]) +
				mulD(e[6], d[pe+4]) + mulD(e[7], d[pe+2])

			pcm[p1] = lo
			p1++
			p2 := p1 + 30

			for sb := 1; sb < 16; sb++ {
				e = &fe[sb]
				o := &fo[sb-1]
				d = &synthD[sb]

				// D[32 - sb][i] == -D[sb][31 - i]
				lo = -(mulD(o[0], d[po+0]) + mulD(o[1], d[po+14]) +
					mulD(o[2], d[po+12]) + mulD(o[3], d[po+10]) +
					mulD(o[4], d[po+8]) + mulD(o[5], d[po+6]) +
					mulD(o[6], d[po+4]) + mulD(o[7], d[po+2]))
				lo += mulD(e[7], d[pe+2]) + mulD(e[6], d[pe+4]) +
					mulD(e[5], d[pe+6]) + mulD(e[4], d[pe+8]) +
					mulD(e[3], d[pe+10]) + mulD(e[2], d[pe+12]) +
					mulD(e[1], d[pe+14]) + mulD(e[0], d[pe+0])

				pcm[p1] = lo
				p1++

				lo = mulD(e[0], d[15-pe]) + mulD(e[1], d[17-pe]) +
					mulD(e[2], d[19-pe]) + mulD(e[3], d[21-pe]) +
					mulD(e[4], d[23-pe]) + mulD(e[5], d[25-pe]) +
					mulD(e[6], d[27-pe]) + mulD(e[7], d[29-pe])
				lo += mulD(o[7], d[29-po]) + mulD(o[6], d[27-po]) +
					mulD(o[5], d[25-po]) + mulD(o[4], d[23-po]) +
					mulD(o[3], d[21-po]) + mulD(o[2], d[19-po]) +
					mulD(o[1], d[17-po]) + mulD(o[0], d[15-po])

				pcm[p2] = lo
				p2--
			}

			d = &synthD[16]
			o := &fo[15]

			lo = mulD(o[0], d[po+0]) + mulD(o[1], d[po+14]) +
				mulD(o[2], d[po+12]) + mulD(o[3], d[po+10]) +
				mulD(o[4], d[po+8]) + mulD(o[5], d[po+6]) +
				mulD(o[6], d[po+4]) + mulD(o[7], d[po+2])

			pcm[p1] = -lo
			p1 += 16

			phase = (phase + 1) % 16
		}
	}
}

// Frame is mad_synth_frame: it synthesizes the PCM samples of f.
func (sy *Synth) Frame(f *Frame) {
	nch := f.Header.Channels()
	ns := f.Header.nsbsamples()

	sy.PCM.SampleRate = f.Header.SampleRate
	sy.PCM.Channels = nch
	sy.PCM.Length = 32 * ns

	sy.synthFull(f, nch, ns)

	sy.phase = (sy.phase + uint32(ns)) % 16
}
