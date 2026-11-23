package mad

// linearTable is the Layer I linear scaling table.
var linearTable = [14]fixed{
	0x15555555, 0x12492492, 0x11111111, 0x10842108,
	0x10410410, 0x10204081, 0x10101010, 0x10080402,
	0x10040100, 0x10020040, 0x10010010, 0x10008004,
	0x10004001, 0x10002000,
}

// lostSync sets ErrLostSync and drops sync. Layers I and II call it when a
// read would run past the end of the frame.
func (s *Stream) lostSync() bool {
	s.Err = ErrLostSync
	s.sync = false
	return false
}

// iSample is I_sample.
func iSample(ptr *BitPtr, nb uint32, s *Stream) fixed {
	frameEnd := s.bitPtrAt(s.NextFrame)
	if bitLength(ptr, &frameEnd) < nb {
		s.lostSync()
		return 0
	}

	sample := fixed(ptr.Read(nb))

	// invert most significant bit, extend sign, then scale to fixed format
	sample ^= 1 << (nb - 1)
	sample |= -(sample & (1 << (nb - 1)))
	sample <<= FracBits - (nb - 1)

	// requantize the sample
	sample += FixedOne >> (nb - 1)

	return mul(sample, linearTable[nb-2])
}

// layerI is mad_layer_I.
func layerI(s *Stream, f *Frame) bool {
	h := &f.Header
	var allocation, scalefactor [2][32]uint8

	bufend := s.bitPtrAt(s.bufEnd)
	frameEnd := s.bitPtrAt(s.NextFrame)

	nch := h.Channels()

	bound := 32
	if h.Mode == ModeJointStereo {
		h.Flags |= FlagIStereo
		bound = 4 + h.ModeExtension*4
	}

	if h.Flags&FlagProtection != 0 {
		n := uint32(4 * (bound*nch + (32 - bound)))
		if bitLength(&s.ptr, &bufend) < n {
			s.Err = ErrBadCRC
			return false
		}
		h.crcCheck = crc(s.ptr, n, h.crcCheck)
		if h.crcCheck != h.crcTarget {
			s.Err = ErrBadCRC
			return false
		}
	}

	// decode bit allocations
	for sb := 0; sb < bound; sb++ {
		for ch := 0; ch < nch; ch++ {
			if bitLength(&s.ptr, &frameEnd) < 4 {
				return s.lostSync()
			}
			nb := s.ptr.Read(4)
			if nb == 15 {
				s.Err = ErrBadBitAlloc
				return false
			}
			if nb != 0 {
				allocation[ch][sb] = uint8(nb + 1)
			} else {
				allocation[ch][sb] = 0
			}
		}
	}
	for sb := bound; sb < 32; sb++ {
		if bitLength(&s.ptr, &frameEnd) < 4 {
			return s.lostSync()
		}
		nb := s.ptr.Read(4)
		if nb == 15 {
			s.Err = ErrBadBitAlloc
			return false
		}
		var a uint8
		if nb != 0 {
			a = uint8(nb + 1)
		}
		allocation[0][sb] = a
		allocation[1][sb] = a
	}

	// decode scalefactors
	for sb := 0; sb < 32; sb++ {
		for ch := 0; ch < nch; ch++ {
			if allocation[ch][sb] != 0 {
				if bitLength(&s.ptr, &frameEnd) < 6 {
					return s.lostSync()
				}
				scalefactor[ch][sb] = uint8(s.ptr.Read(6))
			}
		}
	}

	// decode samples
	for sl := 0; sl < 12; sl++ {
		for sb := 0; sb < bound; sb++ {
			for ch := 0; ch < nch; ch++ {
				nb := allocation[ch][sb]
				if nb != 0 {
					f.sbsample[ch][sl][sb] = mul(iSample(&s.ptr, uint32(nb), s),
						sfTable[scalefactor[ch][sb]])
				} else {
					f.sbsample[ch][sl][sb] = 0
				}
				if s.Err != ErrNone {
					return false
				}
			}
		}
		for sb := bound; sb < 32; sb++ {
			nb := allocation[0][sb]
			if nb != 0 {
				if bitLength(&s.ptr, &frameEnd) < uint32(nb) {
					return s.lostSync()
				}
				sample := iSample(&s.ptr, uint32(nb), s)
				if s.Err != ErrNone {
					return false
				}
				for ch := 0; ch < nch; ch++ {
					f.sbsample[ch][sl][sb] = mul(sample, sfTable[scalefactor[ch][sb]])
				}
			} else {
				for ch := 0; ch < nch; ch++ {
					f.sbsample[ch][sl][sb] = 0
				}
			}
		}
	}
	return true
}

type sbquant struct {
	sblimit int
	offsets [30]uint8
}

// sbquantTable is sbquant_table in layer12.c.
var sbquantTable = [5]sbquant{
	// ISO/IEC 11172-3 Table B.2a
	{27, [30]uint8{7, 7, 7, 6, 6, 6, 6, 6, 6, 6, 6, 3, 3, 3, 3, 3,
		3, 3, 3, 3, 3, 3, 3, 0, 0, 0, 0}},
	// ISO/IEC 11172-3 Table B.2b
	{30, [30]uint8{7, 7, 7, 6, 6, 6, 6, 6, 6, 6, 6, 3, 3, 3, 3, 3,
		3, 3, 3, 3, 3, 3, 3, 0, 0, 0, 0, 0, 0, 0}},
	// ISO/IEC 11172-3 Table B.2c
	{8, [30]uint8{5, 5, 2, 2, 2, 2, 2, 2}},
	// ISO/IEC 11172-3 Table B.2d
	{12, [30]uint8{5, 5, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2}},
	// ISO/IEC 13818-3 Table B.1
	{30, [30]uint8{4, 4, 4, 4, 2, 2, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1,
		1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
}

var bitallocTable = [8]struct{ nbal, offset uint32 }{
	{2, 0}, {2, 3}, {3, 3}, {3, 1}, {4, 2}, {4, 3}, {4, 4}, {4, 5},
}

var offsetTable = [6][15]uint8{
	{0, 1, 16},
	{0, 1, 2, 3, 4, 5, 16},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
	{0, 1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 16},
	{0, 2, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
}

type quantClass struct {
	nlevels uint16
	group   uint8
	bits    uint8
	C       fixed
	D       fixed
}

// iiSamples is II_samples.
func iiSamples(ptr *BitPtr, qc *quantClass, output *[3]fixed, s *Stream) {
	var sample [3]uint32
	frameEnd := s.bitPtrAt(s.NextFrame)

	nb := uint32(qc.group)
	if nb != 0 {
		if bitLength(ptr, &frameEnd) < uint32(qc.bits) {
			s.lostSync()
			return
		}
		// degrouping
		c := uint32(ptr.Read(uint32(qc.bits)))
		nlevels := uint32(qc.nlevels)
		for i := 0; i < 3; i++ {
			sample[i] = c % nlevels
			c /= nlevels
		}
	} else {
		nb = uint32(qc.bits)
		for i := 0; i < 3; i++ {
			if bitLength(ptr, &frameEnd) < nb {
				s.lostSync()
				return
			}
			sample[i] = uint32(ptr.Read(nb))
		}
	}

	for i := 0; i < 3; i++ {
		// invert most significant bit, extend sign, then scale to fixed format
		requantized := fixed(sample[i] ^ 1<<(nb-1))
		requantized |= -(requantized & (1 << (nb - 1)))
		requantized <<= FracBits - (nb - 1)

		// requantize the sample
		output[i] = mul(requantized+qc.D, qc.C)
	}
}

// layerII is mad_layer_II.
func layerII(s *Stream, f *Frame) bool {
	h := &f.Header
	var allocation, scfsi [2][32]uint8
	var scalefactor [2][32][3]uint8
	var samples [3]fixed

	frameEnd := s.bitPtrAt(s.NextFrame)

	nch := h.Channels()

	var index int
	switch {
	case h.Flags&FlagLSFExt != 0:
		index = 4
	case h.Flags&FlagFreeFormat != 0:
		index = freeformatIndex(h)
	default:
		bitratePerChannel := h.Bitrate
		if nch == 2 {
			bitratePerChannel /= 2
		} else if bitratePerChannel > 192000 {
			// ISO/IEC 11172-3 does not allow single channel mode for 224,
			// 256, 320, or 384 kbps bitrates in Layer II.
			s.Err = ErrBadMode
			return false
		}
		switch {
		case bitratePerChannel <= 48000:
			if h.SampleRate == 32000 {
				index = 3
			} else {
				index = 2
			}
		case bitratePerChannel <= 80000:
			index = 0
		default:
			index = freeformatIndex(h)
		}
	}

	sblimit := sbquantTable[index].sblimit
	offsets := &sbquantTable[index].offsets

	bound := 32
	if h.Mode == ModeJointStereo {
		h.Flags |= FlagIStereo
		bound = 4 + h.ModeExtension*4
	}
	if bound > sblimit {
		bound = sblimit
	}

	start := s.ptr

	// decode bit allocations
	for sb := 0; sb < bound; sb++ {
		nbal := bitallocTable[offsets[sb]].nbal
		for ch := 0; ch < nch; ch++ {
			if bitLength(&s.ptr, &frameEnd) < nbal {
				return s.lostSync()
			}
			allocation[ch][sb] = uint8(s.ptr.Read(nbal))
		}
	}
	for sb := bound; sb < sblimit; sb++ {
		nbal := bitallocTable[offsets[sb]].nbal
		if bitLength(&s.ptr, &frameEnd) < nbal {
			return s.lostSync()
		}
		a := uint8(s.ptr.Read(nbal))
		allocation[0][sb] = a
		allocation[1][sb] = a
	}

	// decode scalefactor selection info
	for sb := 0; sb < sblimit; sb++ {
		for ch := 0; ch < nch; ch++ {
			if allocation[ch][sb] != 0 {
				if bitLength(&s.ptr, &frameEnd) < 2 {
					return s.lostSync()
				}
				scfsi[ch][sb] = uint8(s.ptr.Read(2))
			}
		}
	}

	// check CRC word
	if h.Flags&FlagProtection != 0 {
		h.crcCheck = crc(start, bitLength(&start, &s.ptr), h.crcCheck)
		if h.crcCheck != h.crcTarget {
			s.Err = ErrBadCRC
			return false
		}
	}

	// decode scalefactors
	for sb := 0; sb < sblimit; sb++ {
		for ch := 0; ch < nch; ch++ {
			if allocation[ch][sb] == 0 {
				continue
			}
			if bitLength(&s.ptr, &frameEnd) < 6 {
				return s.lostSync()
			}
			sf := &scalefactor[ch][sb]
			sf[0] = uint8(s.ptr.Read(6))

			switch scfsi[ch][sb] {
			case 2:
				sf[2] = sf[0]
				sf[1] = sf[0]
			case 0:
				if bitLength(&s.ptr, &frameEnd) < 6 {
					return s.lostSync()
				}
				sf[1] = uint8(s.ptr.Read(6))
				fallthrough
			case 1, 3:
				if bitLength(&s.ptr, &frameEnd) < 6 {
					return s.lostSync()
				}
				sf[2] = uint8(s.ptr.Read(6))
			}

			if scfsi[ch][sb]&1 != 0 {
				sf[1] = sf[scfsi[ch][sb]-1]
			}
		}
	}

	// decode samples
	for gr := 0; gr < 12; gr++ {
		for sb := 0; sb < bound; sb++ {
			for ch := 0; ch < nch; ch++ {
				if a := allocation[ch][sb]; a != 0 {
					qi := offsetTable[bitallocTable[offsets[sb]].offset][a-1]
					iiSamples(&s.ptr, &qcTable[qi], &samples, s)
					if s.Err != ErrNone {
						return false
					}
					sf := sfTable[scalefactor[ch][sb][gr/4]]
					for i := 0; i < 3; i++ {
						f.sbsample[ch][3*gr+i][sb] = mul(samples[i], sf)
					}
				} else {
					for i := 0; i < 3; i++ {
						f.sbsample[ch][3*gr+i][sb] = 0
					}
				}
			}
		}

		for sb := bound; sb < sblimit; sb++ {
			if a := allocation[0][sb]; a != 0 {
				qi := offsetTable[bitallocTable[offsets[sb]].offset][a-1]
				iiSamples(&s.ptr, &qcTable[qi], &samples, s)
				if s.Err != ErrNone {
					return false
				}
				for ch := 0; ch < nch; ch++ {
					sf := sfTable[scalefactor[ch][sb][gr/4]]
					for i := 0; i < 3; i++ {
						f.sbsample[ch][3*gr+i][sb] = mul(samples[i], sf)
					}
				}
			} else {
				for ch := 0; ch < nch; ch++ {
					for i := 0; i < 3; i++ {
						f.sbsample[ch][3*gr+i][sb] = 0
					}
				}
			}
		}

		for ch := 0; ch < nch; ch++ {
			for i := 0; i < 3; i++ {
				for sb := sblimit; sb < 32; sb++ {
					f.sbsample[ch][3*gr+i][sb] = 0
				}
			}
		}
	}
	return true
}

func freeformatIndex(h *Header) int {
	if h.SampleRate == 48000 {
		return 0
	}
	return 1
}
