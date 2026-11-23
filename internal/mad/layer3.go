package mad

// Bits of channel.flags.
const (
	count1tableSelect = 0x01
	scalefacScale     = 0x02
	preflag           = 0x04
	mixedBlockFlag    = 0x08
)

// Bits of Header.ModeExtension.
const (
	iStereo  = 0x1
	msStereo = 0x2
)

type channel struct {
	// from side info
	part23Length     uint16
	bigValues        uint16
	globalGain       uint16
	scalefacCompress uint16

	flags        uint8
	blockType    uint8
	tableSelect  [3]uint8
	subblockGain [3]uint8
	region0Count uint8
	region1Count uint8

	// from main_data
	scalefac [39]uint8 // scalefac_l and/or scalefac_s
}

type granule struct {
	ch [2]channel
}

type sideInfo struct {
	mainDataBegin uint32
	privateBits   uint32
	scfsi         [2]uint8
	gr            [2]granule
}

// layer3State holds the scratch buffers of mad_layer_III and III_decode that
// live on the C stack. Their contents never carry over between frames.
type layer3State struct {
	si     sideInfo
	xr     [2][576]fixed
	output [36]fixed
}

// sideinfo is III_sideinfo.
func sideinfo(ptr *BitPtr, nch int, lsf bool, si *sideInfo) (result Error, privBitlen uint32) {
	switch {
	case lsf && nch == 1:
		privBitlen = 1
	case lsf:
		privBitlen = 2
	case nch == 1:
		privBitlen = 5
	default:
		privBitlen = 3
	}

	if lsf {
		si.mainDataBegin = uint32(ptr.Read(8))
	} else {
		si.mainDataBegin = uint32(ptr.Read(9))
	}
	si.privateBits = uint32(ptr.Read(privBitlen))

	ngr := 1
	if !lsf {
		ngr = 2
		for ch := range nch {
			si.scfsi[ch] = uint8(ptr.Read(4))
		}
	}

	for gr := 0; gr < ngr; gr++ {
		g := &si.gr[gr]
		for ch := range nch {
			c := &g.ch[ch]

			c.part23Length = uint16(ptr.Read(12))
			c.bigValues = uint16(ptr.Read(9))
			c.globalGain = uint16(ptr.Read(8))
			if lsf {
				c.scalefacCompress = uint16(ptr.Read(9))
			} else {
				c.scalefacCompress = uint16(ptr.Read(4))
			}

			if c.bigValues > 288 && result == 0 {
				result = ErrBadBigValues
			}

			c.flags = 0

			// window_switching_flag
			if ptr.Read(1) != 0 {
				c.blockType = uint8(ptr.Read(2))

				if c.blockType == 0 && result == 0 {
					result = ErrBadBlockType
				}
				if !lsf && c.blockType == 2 && si.scfsi[ch] != 0 && result == 0 {
					result = ErrBadScfsi
				}

				c.region0Count = 7
				c.region1Count = 36

				if ptr.Read(1) != 0 {
					c.flags |= mixedBlockFlag
				} else if c.blockType == 2 {
					c.region0Count = 8
				}

				for i := range 2 {
					c.tableSelect[i] = uint8(ptr.Read(5))
				}
				for i := range 3 {
					c.subblockGain[i] = uint8(ptr.Read(3))
				}
			} else {
				c.blockType = 0

				for i := range 3 {
					c.tableSelect[i] = uint8(ptr.Read(5))
				}
				c.region0Count = uint8(ptr.Read(4))
				c.region1Count = uint8(ptr.Read(3))
			}

			// [preflag,] scalefac_scale, count1table_select
			if lsf {
				c.flags |= uint8(ptr.Read(2))
			} else {
				c.flags |= uint8(ptr.Read(3))
			}
		}
	}
	return result, privBitlen
}

// scalefactorsLSF is III_scalefactors_lsf.
func scalefactorsLSF(ptr *BitPtr, c *channel, gr1ch *channel, modeExtension int,
	bitsLeft uint32, part2Length *uint32) Error {
	var slen [4]uint32
	var nsfb *[4]uint8

	start := *ptr
	sc := uint32(c.scalefacCompress)

	index := 0
	if c.blockType == 2 {
		if c.flags&mixedBlockFlag != 0 {
			index = 2
		} else {
			index = 1
		}
	}

	if !(modeExtension&iStereo != 0 && gr1ch != nil) {
		switch {
		case sc < 400:
			slen[0] = (sc >> 4) / 5
			slen[1] = (sc >> 4) % 5
			slen[2] = (sc % 16) >> 2
			slen[3] = sc % 4
			nsfb = &nsfbTable[0][index]
		case sc < 500:
			sc -= 400
			slen[0] = (sc >> 2) / 5
			slen[1] = (sc >> 2) % 5
			slen[2] = sc % 4
			slen[3] = 0
			nsfb = &nsfbTable[1][index]
		default:
			sc -= 500
			slen[0] = sc / 3
			slen[1] = sc % 3
			slen[2] = 0
			slen[3] = 0
			c.flags |= preflag
			nsfb = &nsfbTable[2][index]
		}

		n := 0
		for part := range 4 {
			for i := 0; i < int(nsfb[part]); i++ {
				if bitsLeft < slen[part] {
					return ErrBadScfsi
				}
				c.scalefac[n] = uint8(ptr.Read(slen[part]))
				n++
				bitsLeft -= slen[part]
			}
		}
		for n < 39 {
			c.scalefac[n] = 0
			n++
		}
	} else { // (mode_extension & I_STEREO) && gr1ch (i.e. ch == 1)
		sc >>= 1

		switch {
		case sc < 180:
			slen[0] = sc / 36
			slen[1] = (sc % 36) / 6
			slen[2] = (sc % 36) % 6
			slen[3] = 0
			nsfb = &nsfbTable[3][index]
		case sc < 244:
			sc -= 180
			slen[0] = (sc % 64) >> 4
			slen[1] = (sc % 16) >> 2
			slen[2] = sc % 4
			slen[3] = 0
			nsfb = &nsfbTable[4][index]
		default:
			sc -= 244
			slen[0] = sc / 3
			slen[1] = sc % 3
			slen[2] = 0
			slen[3] = 0
			nsfb = &nsfbTable[5][index]
		}

		n := 0
		for part := range 4 {
			maxPos := uint32(1)<<slen[part] - 1
			for i := 0; i < int(nsfb[part]); i++ {
				if bitsLeft < slen[part] {
					return ErrBadScfsi
				}
				isPos := uint32(ptr.Read(slen[part]))
				bitsLeft -= slen[part]

				c.scalefac[n] = uint8(isPos)
				if isPos == maxPos {
					gr1ch.scalefac[n] = 1
				} else {
					gr1ch.scalefac[n] = 0
				}
				n++
			}
		}
		for n < 39 {
			c.scalefac[n] = 0
			gr1ch.scalefac[n] = 0 // apparently not illegal
			n++
		}
	}

	*part2Length = bitLength(&start, ptr)
	return ErrNone
}

// scalefactors is III_scalefactors.
func scalefactors(ptr *BitPtr, c *channel, gr0ch *channel, scfsi uint32,
	bitsLeft uint32, part2Length *uint32) Error {
	start := *ptr
	slen1 := uint32(sflenTable[c.scalefacCompress].slen1)
	slen2 := uint32(sflenTable[c.scalefacCompress].slen2)

	if c.blockType == 2 {
		sfbi := 0

		nsfb := 6 * 3
		if c.flags&mixedBlockFlag != 0 {
			nsfb = 8 + 3*3
		}
		for ; nsfb > 0; nsfb-- {
			if bitsLeft < slen1 {
				return ErrBadScfsi
			}
			c.scalefac[sfbi] = uint8(ptr.Read(slen1))
			sfbi++
			bitsLeft -= slen1
		}

		for nsfb = 6 * 3; nsfb > 0; nsfb-- {
			if bitsLeft < slen2 {
				return ErrBadScfsi
			}
			c.scalefac[sfbi] = uint8(ptr.Read(slen2))
			sfbi++
			bitsLeft -= slen2
		}

		for nsfb = 1 * 3; nsfb > 0; nsfb-- {
			c.scalefac[sfbi] = 0
			sfbi++
		}
	} else { // channel->block_type != 2
		type part struct {
			mask     uint32
			from, to int
			slen     uint32
		}
		parts := [4]part{
			{0x8, 0, 6, slen1},
			{0x4, 6, 11, slen1},
			{0x2, 11, 16, slen2},
			{0x1, 16, 21, slen2},
		}
		for _, p := range parts {
			if scfsi&p.mask != 0 {
				for sfbi := p.from; sfbi < p.to; sfbi++ {
					c.scalefac[sfbi] = gr0ch.scalefac[sfbi]
				}
			} else {
				for sfbi := p.from; sfbi < p.to; sfbi++ {
					if bitsLeft < p.slen {
						return ErrBadScfsi
					}
					c.scalefac[sfbi] = uint8(ptr.Read(p.slen))
					bitsLeft -= p.slen
				}
			}
		}

		c.scalefac[21] = 0
	}

	*part2Length = bitLength(&start, ptr)
	return ErrNone
}

// exponentsOf is III_exponents.
func exponentsOf(c *channel, sfbwidth []uint8, exponents *[39]int32) {
	gain := int32(c.globalGain) - 210
	scalefacMultiplier := uint32(1)
	if c.flags&scalefacScale != 0 {
		scalefacMultiplier = 2
	}

	if c.blockType == 2 {
		sfbi, l := 0, 0

		if c.flags&mixedBlockFlag != 0 {
			premask := uint32(0)
			if c.flags&preflag != 0 {
				premask = ^uint32(0)
			}

			// long block subbands 0-1
			for l < 36 {
				exponents[sfbi] = gain -
					int32((uint32(c.scalefac[sfbi])+(uint32(pretab[sfbi])&premask))<<scalefacMultiplier)
				l += int(sfbwidth[sfbi])
				sfbi++
			}
		}

		// this is probably wrong for 8000 Hz short/mixed blocks
		gain0 := gain - 8*int32(c.subblockGain[0])
		gain1 := gain - 8*int32(c.subblockGain[1])
		gain2 := gain - 8*int32(c.subblockGain[2])

		for l < 576 {
			exponents[sfbi+0] = gain0 - int32(uint32(c.scalefac[sfbi+0])<<scalefacMultiplier)
			exponents[sfbi+1] = gain1 - int32(uint32(c.scalefac[sfbi+1])<<scalefacMultiplier)
			exponents[sfbi+2] = gain2 - int32(uint32(c.scalefac[sfbi+2])<<scalefacMultiplier)

			l += 3 * int(sfbwidth[sfbi])
			sfbi += 3
		}
	} else { // channel->block_type != 2
		if c.flags&preflag != 0 {
			for sfbi := range 22 {
				exponents[sfbi] = gain -
					int32((uint32(c.scalefac[sfbi])+uint32(pretab[sfbi]))<<scalefacMultiplier)
			}
		} else {
			for sfbi := range 22 {
				exponents[sfbi] = gain -
					int32(uint32(c.scalefac[sfbi])<<scalefacMultiplier)
			}
		}
	}
}

// requantize is III_requantize: it requantizes one (positive) value.
func requantize(value uint32, exp int32) fixed {
	frac := exp % 4 // assumes sign(frac) == sign(exp)
	exp /= 4

	power := &rqTable[value]
	requantized := fixed(power.mantissa)
	exp += int32(power.exponent)

	if exp < 0 {
		if -exp >= 32 {
			// underflow
			requantized = 0
		} else {
			requantized = fixed(int64(requantized) + int64(1)<<(-exp-1))
			requantized >>= -exp
		}
	} else {
		if exp >= 5 {
			// overflow
			requantized = fixedMax
		} else {
			requantized <<= exp
		}
	}

	if frac != 0 {
		return mul(requantized, rootTable[3+frac])
	}
	return requantized
}

// mask is MASK: a field of the given width in bits, taken from cache so
// that it ends sz bits above the bottom.
func mask(cache uint64, sz int32, bits uint32) uint32 {
	return uint32(cache>>uint32(sz-int32(bits))) & (1<<bits - 1)
}

// mask1bit is MASK1BIT.
func mask1bit(cache uint64, sz int32) bool {
	return cache&uint64(int64(int32(1)<<(sz-1))) != 0
}

func signed(neg bool, v fixed) fixed {
	if neg {
		return -v
	}
	return v
}

// huffdecode is III_huffdecode.
func huffdecode(ptr *BitPtr, xr *[576]fixed, c *channel, sfbwidth []uint8, part3Length int32) Error {
	var exponents [39]int32

	bitsLeft := part3Length

	exponentsOf(c, sfbwidth, &exponents)

	peek := *ptr
	ptr.skip(uint32(bitsLeft))

	// align bit reads to byte boundaries
	cachesz := int32(peek.left)
	cachesz += ((32 - 1 - 24) + (24 - cachesz)) &^ 7
	if bitsLeft < cachesz {
		cachesz = bitsLeft
	}

	bitcache := peek.Read(uint32(cachesz))
	bitsLeft -= cachesz
	fakebits := int32(0)

	xi := 0
	sfi := 0

	var exp int32
	ei := 0

	var sfbound int

	// big_values
	{
		var reqcache [16]fixed

		sfbound = xi + int(sfbwidth[sfi])
		sfi++
		rcount := uint32(c.region0Count) + 1

		region := 0
		entry := &huffPairTable[c.tableSelect[0]]
		table := entry.table
		linbits := uint32(entry.linbits)
		startbits := uint32(entry.startbits)

		if table == nil {
			return ErrBadHuffTable
		}

		exp = exponents[ei]
		ei++
		reqhits := uint32(0)

		bigValues := uint32(c.bigValues)

		for bigValues != 0 {
			bigValues--
			if cachesz+bitsLeft-fakebits <= 0 {
				break
			}

			if xi == sfbound {
				sfbound += int(sfbwidth[sfi])
				sfi++

				// change table if region boundary
				rcount--
				if rcount == 0 {
					if region == 0 {
						rcount = uint32(c.region1Count) + 1
					} else {
						rcount = 0 // all remaining
					}

					region++
					entry = &huffPairTable[c.tableSelect[region]]
					table = entry.table
					linbits = uint32(entry.linbits)
					startbits = uint32(entry.startbits)

					if table == nil {
						return ErrBadHuffTable
					}
				}

				if exp != exponents[ei] {
					exp = exponents[ei]
					reqhits = 0
				}
				ei++
			}

			if cachesz < 21 {
				bits := uint32(((32 - 1 - 21) + (21 - cachesz)) &^ 7)
				if bitsLeft < int32(bits) {
					bits = uint32(bitsLeft)
				}
				bitcache = bitcache<<bits | peek.Read(bits)
				cachesz += int32(bits)
				bitsLeft -= int32(bits)
			}
			if cachesz < 21 {
				bits := 21 - cachesz
				bitcache <<= uint32(bits)
				cachesz += bits
				fakebits += bits
			}

			// hcod (0..19)
			clumpsz := startbits
			pair := &table[mask(bitcache, cachesz, clumpsz)]

			for !pair.final {
				cachesz -= int32(clumpsz)

				clumpsz = uint32(pair.bits)
				pair = &table[uint32(pair.offset)+mask(bitcache, cachesz, clumpsz)]
			}

			cachesz -= int32(pair.bits)
			if cachesz < fakebits {
				return ErrBadHuffData
			}

			if linbits != 0 {
				// x (0..14)
				value := uint32(pair.x)

				switch value {
				case 0:
					xr[xi] = 0
				case 15:
					if cachesz < int32(linbits+2) {
						bits := uint32(16)
						if bitsLeft < 16 {
							bits = uint32(bitsLeft)
						}
						bitcache = bitcache<<bits | peek.Read(bits)
						cachesz += int32(bits)
						bitsLeft -= int32(bits)
					}
					if cachesz-fakebits < int32(linbits) {
						return ErrBadHuffData
					}

					value += mask(bitcache, cachesz, linbits)
					cachesz -= int32(linbits)

					requantized := requantize(value, exp)
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				default:
					var requantized fixed
					if reqhits&(1<<value) != 0 {
						requantized = reqcache[value]
					} else {
						reqhits |= 1 << value
						requantized = requantize(value, exp)
						reqcache[value] = requantized
					}
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				}

				// y (0..14)
				value = uint32(pair.y)

				switch value {
				case 0:
					xr[xi+1] = 0
				case 15:
					if cachesz < int32(linbits+1) {
						bits := uint32(16)
						if bitsLeft < 16 {
							bits = uint32(bitsLeft)
						}
						bitcache = bitcache<<bits | peek.Read(bits)
						cachesz += int32(bits)
						bitsLeft -= int32(bits)
					}
					if cachesz-fakebits < int32(linbits) {
						return ErrBadHuffData
					}

					value += mask(bitcache, cachesz, linbits)
					cachesz -= int32(linbits)

					requantized := requantize(value, exp)
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi+1] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				default:
					var requantized fixed
					if reqhits&(1<<value) != 0 {
						requantized = reqcache[value]
					} else {
						reqhits |= 1 << value
						requantized = requantize(value, exp)
						reqcache[value] = requantized
					}
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi+1] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				}
			} else {
				// x (0..1)
				value := uint32(pair.x)

				if value == 0 {
					xr[xi] = 0
				} else {
					var requantized fixed
					if reqhits&(1<<value) != 0 {
						requantized = reqcache[value]
					} else {
						reqhits |= 1 << value
						requantized = requantize(value, exp)
						reqcache[value] = requantized
					}
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				}

				// y (0..1)
				value = uint32(pair.y)

				if value == 0 {
					xr[xi+1] = 0
				} else {
					var requantized fixed
					if reqhits&(1<<value) != 0 {
						requantized = reqcache[value]
					} else {
						reqhits |= 1 << value
						requantized = requantize(value, exp)
						reqcache[value] = requantized
					}
					if cachesz-fakebits < 1 {
						return ErrBadHuffData
					}
					xr[xi+1] = signed(mask1bit(bitcache, cachesz), requantized)
					cachesz--
				}
			}

			xi += 2
		}
	}

	// count1
	{
		table := huffQuadTable[c.flags&count1tableSelect]

		requantized := requantize(1, exp)

		for cachesz+bitsLeft-fakebits > 0 && xi <= 572 {
			// hcod (1..6)
			if cachesz < 10 {
				bits := uint32(16)
				if bitsLeft < 16 {
					bits = uint32(bitsLeft)
				}
				bitcache = bitcache<<bits | peek.Read(bits)
				cachesz += int32(bits)
				bitsLeft -= int32(bits)
			}
			if cachesz < 10 {
				bits := 10 - cachesz
				bitcache <<= uint32(bits)
				cachesz += bits
				fakebits += bits
			}

			quad := &table[mask(bitcache, cachesz, 4)]

			// quad tables guaranteed to have at most one extra lookup
			if !quad.final {
				cachesz -= 4

				quad = &table[uint32(quad.offset)+mask(bitcache, cachesz, uint32(quad.bits))]
			}

			if cachesz-fakebits < int32(quad.bits)+int32(quad.v)+int32(quad.w)+int32(quad.x)+int32(quad.y) {
				// not enough bits for one more entry: they are stuffing bits
				break
			}

			cachesz -= int32(quad.bits)

			if xi == sfbound {
				sfbound += int(sfbwidth[sfi])
				sfi++

				if exp != exponents[ei] {
					exp = exponents[ei]
					requantized = requantize(1, exp)
				}
				ei++
			}

			// v (0..1)
			if quad.v != 0 {
				xr[xi] = signed(mask1bit(bitcache, cachesz), requantized)
				cachesz--
			} else {
				xr[xi] = 0
			}
			// w (0..1)
			if quad.w != 0 {
				xr[xi+1] = signed(mask1bit(bitcache, cachesz), requantized)
				cachesz--
			} else {
				xr[xi+1] = 0
			}

			xi += 2

			if xi == sfbound {
				sfbound += int(sfbwidth[sfi])
				sfi++

				if exp != exponents[ei] {
					exp = exponents[ei]
					requantized = requantize(1, exp)
				}
				ei++
			}

			// x (0..1)
			if quad.x != 0 {
				xr[xi] = signed(mask1bit(bitcache, cachesz), requantized)
				cachesz--
			} else {
				xr[xi] = 0
			}
			// y (0..1)
			if quad.y != 0 {
				xr[xi+1] = signed(mask1bit(bitcache, cachesz), requantized)
				cachesz--
			} else {
				xr[xi+1] = 0
			}

			xi += 2
		}
	}

	// rzero
	for ; xi < 576; xi += 2 {
		xr[xi] = 0
		xr[xi+1] = 0
	}

	return ErrNone
}

// reorder is III_reorder: it reorders the frequency lines of a short block
// into subband order.
func reorder(xr *[576]fixed, c *channel, sfbwidth []uint8) {
	var tmp [32][3][6]fixed
	var sbw, sw [3]int

	// this is probably wrong for 8000 Hz mixed blocks
	sb := 0
	sfi := 0
	if c.flags&mixedBlockFlag != 0 {
		sb = 2

		l := 0
		for l < 36 {
			l += int(sfbwidth[sfi])
			sfi++
		}
	}

	for w := range 3 {
		sbw[w] = sb
		sw[w] = 0
	}

	f := int(sfbwidth[sfi])
	sfi++
	w := 0

	for l := 18 * sb; l < 576; l++ {
		if f == 0 {
			f = int(sfbwidth[sfi]) - 1
			sfi++
			w = (w + 1) % 3
		} else {
			f--
		}

		tmp[sbw[w]][w][sw[w]] = xr[l]
		sw[w]++

		if sw[w] == 6 {
			sw[w] = 0
			sbw[w]++
		}
	}

	// copy tmp[sb:] over xr[18*sb:] in memory order
	n := 0
	for i := sb; i < 32; i++ {
		for j := range 3 {
			n += copy(xr[18*sb+n:], tmp[i][j][:])
		}
	}
}

// stereo is III_stereo: joint stereo processing on a granule.
func stereo(xr *[2][576]fixed, g []granule, h *Header, sfbwidth []uint8) Error {
	var modes [39]int16

	if g[0].ch[0].blockType != g[0].ch[1].blockType ||
		(g[0].ch[0].flags&mixedBlockFlag) != (g[0].ch[1].flags&mixedBlockFlag) {
		return ErrBadStereo
	}

	for i := range modes {
		modes[i] = int16(h.ModeExtension)
	}

	// intensity stereo
	if h.ModeExtension&iStereo != 0 {
		rightCh := &g[0].ch[1]
		rightXr := xr[1][:]

		h.Flags |= FlagIStereo

		// first determine which scalefactor bands are to be processed
		if rightCh.blockType == 2 {
			var lower, start, maxSfb uint32
			var bound [3]uint32

			sfbi, l := 0, 0

			if rightCh.flags&mixedBlockFlag != 0 {
				for l < 36 {
					n := int(sfbwidth[sfbi])
					sfbi++

					for i := range n {
						if rightXr[i] != 0 {
							lower = uint32(sfbi)
							break
						}
					}

					rightXr = rightXr[n:]
					l += n
				}

				start = uint32(sfbi)
			}

			w := 0
			for l < 576 {
				n := int(sfbwidth[sfbi])
				sfbi++

				for i := range n {
					if rightXr[i] != 0 {
						bound[w] = uint32(sfbi)
						maxSfb = bound[w]
						break
					}
				}

				rightXr = rightXr[n:]
				l += n
				w = (w + 1) % 3
			}

			if maxSfb != 0 {
				lower = start
			}

			// long blocks
			for i := uint32(0); i < lower; i++ {
				modes[i] = int16(h.ModeExtension &^ iStereo)
			}

			// short blocks
			w = 0
			for i := start; i < maxSfb; i++ {
				if i < bound[w] {
					modes[i] = int16(h.ModeExtension &^ iStereo)
				}
				w = (w + 1) % 3
			}
		} else { // right_ch->block_type != 2
			bound := 0
			for sfbi, l := 0, 0; l < 576; {
				n := int(sfbwidth[sfbi])
				sfbi++

				for i := range n {
					if rightXr[i] != 0 {
						bound = sfbi
						break
					}
				}

				rightXr = rightXr[n:]
				l += n
			}

			for i := 0; i < bound; i++ {
				modes[i] = int16(h.ModeExtension &^ iStereo)
			}
		}

		// now do the actual processing
		if h.Flags&FlagLSFExt != 0 {
			illegalPos := &g[1].ch[1].scalefac

			// intensity_scale
			lsfScale := &isLsfTable[rightCh.scalefacCompress&0x1]

			for sfbi, l := 0, 0; l < 576; sfbi++ {
				n := int(sfbwidth[sfbi])
				if modes[sfbi]&iStereo == 0 {
					l += n
					continue
				}
				if illegalPos[sfbi] != 0 {
					modes[sfbi] &^= iStereo
					l += n
					continue
				}

				isPos := uint32(rightCh.scalefac[sfbi])

				for i := range n {
					left := xr[0][l+i]

					if isPos == 0 {
						xr[1][l+i] = left
					} else {
						opposite := mul(left, lsfScale[(isPos-1)/2])

						if isPos&1 != 0 {
							xr[0][l+i] = opposite
							xr[1][l+i] = left
						} else {
							xr[1][l+i] = opposite
						}
					}
				}
				l += n
			}
		} else { // !(header->flags & MAD_FLAG_LSF_EXT)
			for sfbi, l := 0, 0; l < 576; sfbi++ {
				n := int(sfbwidth[sfbi])
				if modes[sfbi]&iStereo == 0 {
					l += n
					continue
				}

				isPos := uint32(rightCh.scalefac[sfbi])

				if isPos >= 7 { // illegal intensity position
					modes[sfbi] &^= iStereo
					l += n
					continue
				}

				for i := range n {
					left := xr[0][l+i]

					xr[0][l+i] = mul(left, isTable[isPos])
					xr[1][l+i] = mul(left, isTable[6-isPos])
				}
				l += n
			}
		}
	}

	// middle/side stereo
	if h.ModeExtension&msStereo != 0 {
		h.Flags |= FlagMSStereo

		invsqrt2 := rootTable[3+-2]

		for sfbi, l := 0, 0; l < 576; sfbi++ {
			n := int(sfbwidth[sfbi])
			if modes[sfbi] != msStereo {
				l += n
				continue
			}

			for i := range n {
				m := xr[0][l+i]
				s := xr[1][l+i]

				xr[0][l+i] = mul(m+s, invsqrt2) // left: sum of mid and side over sqrt 2
				xr[1][l+i] = mul(m-s, invsqrt2) // right: mid minus side over sqrt 2
			}
			l += n
		}
	}

	return ErrNone
}

// aliasReduce is III_aliasreduce.
func aliasReduce(xr *[576]fixed, lines int) {
	for x := 18; x < lines; x += 18 {
		for i := range 8 {
			a := xr[x-1-i]
			b := xr[x+i]

			xr[x-1-i] = mul(a, aliasCs[i]) + mul(-b, aliasCa[i])
			xr[x+i] = mul(b, aliasCs[i]) + mul(a, aliasCa[i])
		}
	}
}

// decodeMainData is III_decode: it decodes the frame's main_data.
func decodeMainData(ptr *BitPtr, f *Frame, st *layer3State, nch int, mdLen uint32) Error {
	h := &f.Header
	si := &st.si
	bitsLeft := int32(mdLen * 8)

	sfreq := h.SampleRate
	if h.Flags&FlagMPEG25Ext != 0 {
		sfreq *= 2
	}

	// 48000 => 0, 44100 => 1, 32000 => 2,
	// 24000 => 3, 22050 => 4, 16000 => 5
	sfreqi := ((sfreq >> 7) & 0x000f) + ((sfreq >> 15) & 0x0001) - 8

	if h.Flags&FlagMPEG25Ext != 0 {
		sfreqi += 3
	}

	// scalefactors, Huffman decoding, requantization
	ngr := 2
	if h.Flags&FlagLSFExt != 0 {
		ngr = 1
	}

	for gr := 0; gr < ngr; gr++ {
		g := &si.gr[gr]
		var sfbwidth [2][]uint8
		xr := &st.xr

		for ch := range nch {
			c := &g.ch[ch]
			var part2Length uint32
			var err Error

			sfbwidth[ch] = sfbwidthTable[sfreqi].l
			if c.blockType == 2 {
				if c.flags&mixedBlockFlag != 0 {
					sfbwidth[ch] = sfbwidthTable[sfreqi].m
				} else {
					sfbwidth[ch] = sfbwidthTable[sfreqi].s
				}
			}

			if h.Flags&FlagLSFExt != 0 {
				var gr1ch *channel
				if ch != 0 {
					gr1ch = &si.gr[1].ch[1]
				}
				err = scalefactorsLSF(ptr, c, gr1ch, h.ModeExtension, uint32(bitsLeft), &part2Length)
			} else {
				scfsi := uint32(0)
				if gr != 0 {
					scfsi = uint32(si.scfsi[ch])
				}
				err = scalefactors(ptr, c, &si.gr[0].ch[ch], scfsi, uint32(bitsLeft), &part2Length)
			}
			if err != ErrNone {
				return err
			}

			bitsLeft -= int32(part2Length)

			if part2Length > uint32(c.part23Length) {
				return ErrBadPart3Len
			}

			part3Length := uint32(c.part23Length) - part2Length
			if part3Length > uint32(bitsLeft) {
				return ErrBadPart3Len
			}

			if err := huffdecode(ptr, &xr[ch], c, sfbwidth[ch], int32(part3Length)); err != ErrNone {
				return err
			}

			bitsLeft -= int32(part3Length)
		}

		// joint stereo processing
		if h.Mode == ModeJointStereo && h.ModeExtension != 0 {
			if err := stereo(xr, si.gr[gr:], h, sfbwidth[0]); err != ErrNone {
				return err
			}
		}

		// reordering, alias reduction, IMDCT, overlap-add, frequency inversion
		for ch := range nch {
			c := &g.ch[ch]
			sample := (*[18][32]fixed)(f.sbsample[ch][18*gr : 18*gr+18])
			ov := &f.overlap[ch]
			x := &xr[ch]
			output := &st.output

			if c.blockType == 2 {
				reorder(x, c, sfbwidth[ch])

				// Alias reduction on the lower two subbands of mixed
				// blocks, as most implementations do (no OPT_STRICT).
				if c.flags&mixedBlockFlag != 0 {
					aliasReduce(x, 36)
				}
			} else {
				aliasReduce(x, 576)
			}

			l := 0

			// subbands 0-1
			if c.blockType != 2 || c.flags&mixedBlockFlag != 0 {
				blockType := c.blockType
				if c.flags&mixedBlockFlag != 0 {
					blockType = 0
				}

				// long blocks
				for sb := range 2 {
					imdctL((*[18]fixed)(x[l:l+18]), output, blockType)
					overlap(output, &ov[sb], sample, sb)
					l += 18
				}
			} else {
				// short blocks
				for sb := range 2 {
					imdctShort((*[18]fixed)(x[l:l+18]), output)
					overlap(output, &ov[sb], sample, sb)
					l += 18
				}
			}

			freqinver(sample, 1)

			// (nonzero) subbands 2-31
			i := 576
			for i > 36 && x[i-1] == 0 {
				i--
			}

			sblimit := 32 - (576-i)/18

			if c.blockType != 2 {
				// long blocks
				for sb := 2; sb < sblimit; sb++ {
					imdctL((*[18]fixed)(x[l:l+18]), output, c.blockType)
					overlap(output, &ov[sb], sample, sb)

					if sb&1 != 0 {
						freqinver(sample, sb)
					}
					l += 18
				}
			} else {
				// short blocks
				for sb := 2; sb < sblimit; sb++ {
					imdctShort((*[18]fixed)(x[l:l+18]), output)
					overlap(output, &ov[sb], sample, sb)

					if sb&1 != 0 {
						freqinver(sample, sb)
					}
					l += 18
				}
			}

			// remaining (zero) subbands
			for sb := sblimit; sb < 32; sb++ {
				overlapZ(&ov[sb], sample, sb)

				if sb&1 != 0 {
					freqinver(sample, sb)
				}
			}
		}
	}

	return ErrNone
}

// layerIII is mad_layer_III.
func layerIII(s *Stream, f *Frame) bool {
	h := &f.Header
	var nextMdBegin uint32
	result := true

	bufend := s.bitPtrAt(s.bufEnd)

	// allocate Layer III dynamic structures
	if s.mainData == nil {
		s.mainData = new([mainDataLen]byte)
	}
	if f.overlap == nil {
		f.overlap = new([2][32][18]fixed)
	}
	if s.l3 == nil {
		s.l3 = new(layer3State)
	}
	st := s.l3
	si := &st.si

	nch := h.Channels()
	var siLen int
	switch {
	case h.Flags&FlagLSFExt != 0 && nch == 1:
		siLen = 9
	case h.Flags&FlagLSFExt != 0:
		siLen = 17
	case nch == 1:
		siLen = 17
	default:
		siLen = 32
	}

	// check frame sanity
	if s.NextFrame-s.ptr.nextByte() < siLen {
		s.Err = ErrBadFrameLen
		s.mdLen = 0
		return false
	}

	// check CRC word
	if h.Flags&FlagProtection != 0 {
		h.crcCheck = crc(s.ptr, uint32(siLen*8), h.crcCheck)

		if h.crcCheck != h.crcTarget {
			s.Err = ErrBadCRC
			result = false
		}
	}

	// decode frame side information
	serr, privBitlen := sideinfo(&s.ptr, nch, h.Flags&FlagLSFExt != 0, si)
	if serr != ErrNone && result {
		s.Err = serr
		result = false
	}

	h.Flags |= int(privBitlen)
	h.PrivateBits |= int(si.privateBits)

	// find main_data of next frame
	{
		peek := s.bitPtrAt(s.NextFrame)

		if bitLength(&peek, &bufend) >= 57 {
			header := peek.Read(32)
			if header&0xffe60000 == 0xffe20000 { // syncword | layer
				if header&0x00010000 == 0 { // protection_bit
					peek.skip(16) // crc_check
				}
				if header&0x00080000 != 0 { // ID
					nextMdBegin = uint32(peek.Read(9))
				} else {
					nextMdBegin = uint32(peek.Read(8))
				}
			}
		}
	}

	// find main_data of this frame
	frameSpace := uint32(s.NextFrame - s.ptr.nextByte())

	if nextMdBegin > si.mainDataBegin+frameSpace {
		nextMdBegin = 0
	}

	mdLen := si.mainDataBegin + frameSpace - nextMdBegin

	if mdLen+BufferGuard > mainDataLen {
		s.Err = ErrLostSync
		s.sync = false
		return false
	}

	var ptr BitPtr
	frameUsed := uint32(0)

	if si.mainDataBegin == 0 {
		ptr = s.ptr
		s.mdLen = 0

		frameUsed = mdLen
	} else {
		if si.mainDataBegin > s.mdLen {
			if result {
				s.Err = ErrBadDataPtr
				result = false
			}
		} else {
			md := s.mainData[:]
			copy(md, md[s.mdLen-si.mainDataBegin:s.mdLen])
			s.mdLen = si.mainDataBegin

			ptr = newBitPtr(md, 0)

			if mdLen > si.mainDataBegin {
				frameUsed = mdLen - si.mainDataBegin
				src := s.ptr.nextByte()
				copy(md[s.mdLen:s.mdLen+frameUsed], s.data[src:src+int(frameUsed)])
				s.mdLen += frameUsed
			}
		}
	}

	frameFree := frameSpace - frameUsed

	// decode main_data
	if result {
		if err := decodeMainData(&ptr, f, st, nch, mdLen); err != ErrNone {
			s.Err = err
			result = false
		}

		// designate ancillary bits
		s.AncPtr = ptr
	}

	// preload main_data buffer with up to 511 bytes for next frame(s)
	md := s.mainData[:]
	if frameFree >= nextMdBegin {
		src := s.NextFrame - int(nextMdBegin)
		copy(md[:nextMdBegin], s.data[src:src+int(nextMdBegin)])
		s.mdLen = nextMdBegin
	} else {
		if mdLen < si.mainDataBegin {
			extra := si.mainDataBegin - mdLen
			if extra+frameFree > nextMdBegin {
				extra = nextMdBegin - frameFree
			}

			if extra < s.mdLen {
				copy(md, md[s.mdLen-extra:s.mdLen])
				s.mdLen = extra
			}
		} else {
			s.mdLen = 0
		}

		src := s.NextFrame - int(frameFree)
		copy(md[s.mdLen:s.mdLen+frameFree], s.data[src:src+int(frameFree)])
		s.mdLen += frameFree
	}

	return result
}
