package mad

type huffQuad struct {
	offset     uint16
	final      bool
	bits       uint8 // ptr.bits, or value.hlen when final
	v, w, x, y uint8
}

type huffPair struct {
	offset uint16
	final  bool
	bits   uint8 // ptr.bits, or value.hlen when final
	x, y   uint8
}

type huffTable struct {
	table     []huffPair
	linbits   uint16
	startbits uint16
}

// fixedFloat is one rq_table entry: mantissa * 2^exponent == x^(4/3).
type fixedFloat struct {
	mantissa uint32
	exponent uint8
}

var sflenTable = [16]struct{ slen1, slen2 uint8 }{
	{0, 0}, {0, 1}, {0, 2}, {0, 3},
	{3, 0}, {1, 1}, {1, 2}, {1, 3},
	{2, 1}, {2, 2}, {2, 3}, {3, 1},
	{3, 2}, {3, 3}, {4, 2}, {4, 3},
}

var nsfbTable = [6][3][4]uint8{
	{{6, 5, 5, 5}, {9, 9, 9, 9}, {6, 9, 9, 9}},
	{{6, 5, 7, 3}, {9, 9, 12, 6}, {6, 9, 12, 6}},
	{{11, 10, 0, 0}, {18, 18, 0, 0}, {15, 18, 0, 0}},
	{{7, 7, 7, 0}, {12, 12, 12, 0}, {6, 15, 12, 0}},
	{{6, 6, 6, 3}, {12, 9, 9, 6}, {6, 12, 9, 6}},
	{{8, 8, 5, 0}, {15, 12, 9, 0}, {6, 18, 9, 0}},
}

// MPEG-1 scalefactor band widths (Table B.8 of ISO/IEC 11172-3).
var (
	sfb48000Long = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6, 6, 8, 10,
		12, 16, 18, 22, 28, 34, 40, 46, 54, 54, 192,
	}
	sfb44100Long = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6, 8, 8, 10,
		12, 16, 20, 24, 28, 34, 42, 50, 54, 76, 158,
	}
	sfb32000Long = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6, 8, 10, 12,
		16, 20, 24, 30, 38, 46, 56, 68, 84, 102, 26,
	}
	sfb48000Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 6,
		6, 6, 6, 6, 6, 10, 10, 10, 12, 12, 12, 14, 14,
		14, 16, 16, 16, 20, 20, 20, 26, 26, 26, 66, 66, 66,
	}
	sfb44100Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 6,
		6, 6, 8, 8, 8, 10, 10, 10, 12, 12, 12, 14, 14,
		14, 18, 18, 18, 22, 22, 22, 30, 30, 30, 56, 56, 56,
	}
	sfb32000Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 6,
		6, 6, 8, 8, 8, 12, 12, 12, 16, 16, 16, 20, 20,
		20, 26, 26, 26, 34, 34, 34, 42, 42, 42, 12, 12, 12,
	}
	sfb48000Mixed = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6,
		4, 4, 4, 6, 6, 6, 6, 6, 6, 10,
		10, 10, 12, 12, 12, 14, 14, 14, 16, 16,
		16, 20, 20, 20, 26, 26, 26, 66, 66, 66,
	}
	sfb44100Mixed = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6,
		4, 4, 4, 6, 6, 6, 8, 8, 8, 10,
		10, 10, 12, 12, 12, 14, 14, 14, 18, 18,
		18, 22, 22, 22, 30, 30, 30, 56, 56, 56,
	}
	sfb32000Mixed = []uint8{
		4, 4, 4, 4, 4, 4, 6, 6,
		4, 4, 4, 6, 6, 6, 8, 8, 8, 12,
		12, 12, 16, 16, 16, 20, 20, 20, 26, 26,
		26, 34, 34, 34, 42, 42, 42, 12, 12, 12,
	}
)

// MPEG-2 scalefactor band widths (Table B.2 of ISO/IEC 13818-3).
var (
	sfb24000Long = []uint8{
		6, 6, 6, 6, 6, 6, 8, 10, 12, 14, 16,
		18, 22, 26, 32, 38, 46, 54, 62, 70, 76, 36,
	}
	sfb22050Long = []uint8{
		6, 6, 6, 6, 6, 6, 8, 10, 12, 14, 16,
		20, 24, 28, 32, 38, 46, 52, 60, 68, 58, 54,
	}
	sfb16000Long  = sfb22050Long
	sfb24000Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 6, 6, 6, 8,
		8, 8, 10, 10, 10, 12, 12, 12, 14, 14, 14, 18, 18,
		18, 24, 24, 24, 32, 32, 32, 44, 44, 44, 12, 12, 12,
	}
	sfb22050Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 6, 6, 6, 6,
		6, 6, 8, 8, 8, 10, 10, 10, 14, 14, 14, 18, 18,
		18, 26, 26, 26, 32, 32, 32, 42, 42, 42, 18, 18, 18,
	}
	sfb16000Short = []uint8{
		4, 4, 4, 4, 4, 4, 4, 4, 4, 6, 6, 6, 8,
		8, 8, 10, 10, 10, 12, 12, 12, 14, 14, 14, 18, 18,
		18, 24, 24, 24, 30, 30, 30, 40, 40, 40, 18, 18, 18,
	}
	sfb24000Mixed = []uint8{
		6, 6, 6, 6, 6, 6,
		6, 6, 6, 8, 8, 8, 10, 10, 10, 12,
		12, 12, 14, 14, 14, 18, 18, 18, 24, 24,
		24, 32, 32, 32, 44, 44, 44, 12, 12, 12,
	}
	sfb22050Mixed = []uint8{
		6, 6, 6, 6, 6, 6,
		6, 6, 6, 6, 6, 6, 8, 8, 8, 10,
		10, 10, 14, 14, 14, 18, 18, 18, 26, 26,
		26, 32, 32, 32, 42, 42, 42, 18, 18, 18,
	}
	sfb16000Mixed = []uint8{
		6, 6, 6, 6, 6, 6,
		6, 6, 6, 8, 8, 8, 10, 10, 10, 12,
		12, 12, 14, 14, 14, 18, 18, 18, 24, 24,
		24, 30, 30, 30, 40, 40, 40, 18, 18, 18,
	}
)

// MPEG 2.5 scalefactor band widths.
var (
	sfb12000Long = sfb16000Long
	sfb11025Long = sfb12000Long
	sfb8000Long  = []uint8{
		12, 12, 12, 12, 12, 12, 16, 20, 24, 28, 32,
		40, 48, 56, 64, 76, 90, 2, 2, 2, 2, 2,
	}
	sfb12000Short = sfb16000Short
	sfb11025Short = sfb12000Short
	sfb8000Short  = []uint8{
		8, 8, 8, 8, 8, 8, 8, 8, 8, 12, 12, 12, 16,
		16, 16, 20, 20, 20, 24, 24, 24, 28, 28, 28, 36, 36,
		36, 2, 2, 2, 2, 2, 2, 2, 2, 2, 26, 26, 26,
	}
	sfb12000Mixed = sfb16000Mixed
	sfb11025Mixed = sfb12000Mixed
	// the 8000 Hz short block scalefactor bands do not break after the
	// first 36 frequency lines, so this is probably wrong (libmad note)
	sfb8000Mixed = []uint8{
		12, 12, 12,
		4, 4, 4, 8, 8, 8, 12, 12, 12, 16, 16, 16,
		20, 20, 20, 24, 24, 24, 28, 28, 28, 36, 36, 36,
		2, 2, 2, 2, 2, 2, 2, 2, 2, 26, 26, 26,
	}
)

var sfbwidthTable = [9]struct{ l, s, m []uint8 }{
	{sfb48000Long, sfb48000Short, sfb48000Mixed},
	{sfb44100Long, sfb44100Short, sfb44100Mixed},
	{sfb32000Long, sfb32000Short, sfb32000Mixed},
	{sfb24000Long, sfb24000Short, sfb24000Mixed},
	{sfb22050Long, sfb22050Short, sfb22050Mixed},
	{sfb16000Long, sfb16000Short, sfb16000Mixed},
	{sfb12000Long, sfb12000Short, sfb12000Mixed},
	{sfb11025Long, sfb11025Short, sfb11025Mixed},
	{sfb8000Long, sfb8000Short, sfb8000Mixed},
}

// pretab is the scalefactor band preemphasis (Table B.6 of ISO/IEC 11172-3).
var pretab = [22]uint8{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 3, 3, 3, 2, 0,
}

// rootTable[3 + x] = 2^(x/4)
var rootTable = [7]fixed{
	0x09837f05, 0x0b504f33, 0x0d744fcd, 0x10000000,
	0x1306fe0a, 0x16a09e66, 0x1ae89f99,
}

// Coefficients for aliasing reduction (Table B.9 of ISO/IEC 11172-3).
var aliasCs = [8]fixed{
	+0x0db84a81, +0x0e1b9d7f, +0x0f31adcf, +0x0fbba815,
	+0x0feda417, +0x0ffc8fc8, +0x0fff964c, +0x0ffff8d3,
}

var aliasCa = [8]fixed{
	-0x083b5fe7, -0x078c36d2, -0x05039814, -0x02e91dd1,
	-0x0183603a, -0x00a7cb87, -0x003a2847, -0x000f27b4,
}

// windowL[i] = sin((PI / 36) * (i + 1/2))
var windowL = [36]fixed{
	0x00b2aa3e, 0x0216a2a2, 0x03768962, 0x04cfb0e2,
	0x061f78aa, 0x07635284, 0x0898c779, 0x09bd7ca0,
	0x0acf37ad, 0x0bcbe352, 0x0cb19346, 0x0d7e8807,
	0x0e313245, 0x0ec835e8, 0x0f426cb5, 0x0f9ee890,
	0x0fdcf549, 0x0ffc19fd, 0x0ffc19fd, 0x0fdcf549,
	0x0f9ee890, 0x0f426cb5, 0x0ec835e8, 0x0e313245,
	0x0d7e8807, 0x0cb19346, 0x0bcbe352, 0x0acf37ad,
	0x09bd7ca0, 0x0898c779, 0x07635284, 0x061f78aa,
	0x04cfb0e2, 0x03768962, 0x0216a2a2, 0x00b2aa3e,
}

// windowS[i] = sin((PI / 12) * (i + 1/2))
var windowS = [12]fixed{
	0x0216a2a2, 0x061f78aa, 0x09bd7ca0, 0x0cb19346,
	0x0ec835e8, 0x0fdcf549, 0x0fdcf549, 0x0ec835e8,
	0x0cb19346, 0x09bd7ca0, 0x061f78aa, 0x0216a2a2,
}

// isTable[i] = tan(i * (PI / 12)) / (1 + tan(i * (PI / 12)))
var isTable = [7]fixed{
	0x00000000, 0x0361962f, 0x05db3d74, 0x08000000,
	0x0a24c28c, 0x0c9e69d1, 0x10000000,
}

// isLsfTable holds the LSF intensity stereo coefficients.
var isLsfTable = [2][15]fixed{
	{
		0x0d744fcd, 0x0b504f33, 0x09837f05, 0x08000000,
		0x06ba27e6, 0x05a8279a, 0x04c1bf83, 0x04000000,
		0x035d13f3, 0x02d413cd, 0x0260dfc1, 0x02000000,
		0x01ae89fa, 0x016a09e6, 0x01306fe1,
	}, {
		0x0b504f33, 0x08000000, 0x05a8279a, 0x04000000,
		0x02d413cd, 0x02000000, 0x016a09e6, 0x01000000,
		0x00b504f3, 0x00800000, 0x005a827a, 0x00400000,
		0x002d413d, 0x00200000, 0x0016a09e,
	},
}
