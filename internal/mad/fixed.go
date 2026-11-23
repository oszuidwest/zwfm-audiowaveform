package mad

// fixed is libmad's mad_fixed_t: a signed 32-bit value with 28 fraction bits.
type fixed = int32

const (
	// FracBits is MAD_F_FRACBITS.
	FracBits = 28
	// FixedOne is MAD_F_ONE.
	FixedOne fixed = 0x10000000

	fixedMax fixed = 0x7fffffff
)

// mul is mad_f_mul for FPM_64BIT with OPT_ACCURACY: a full 64-bit product,
// rounded to the nearest fixed-point value.
func mul(x, y fixed) fixed {
	return fixed((int64(x)*int64(y) + 1<<(FracBits-1)) >> FracBits)
}

// mulWide is mad_f_mul applied to a difference that the reference build
// computes without 32-bit overflow. Signed overflow is undefined in C, and
// clang at -Os (Homebrew's optimization level) folds the sign extension of
// such a difference into 64-bit loads and arithmetic, so a difference that
// does not fit in 32 bits reaches the multiply whole instead of wrapped.
func mulWide(x int64, y fixed) fixed {
	return fixed((x*int64(y) + 1<<(FracBits-1)) >> FracBits)
}
