// Package mad is a pure Go port of libmad 0.16.5, the MPEG audio decoder for
// layers I, II and III, as built by Homebrew and linked by audiowaveform.
// Its output matches that build sample for sample.
//
// That build uses 64-bit fixed-point math (FPM_64BIT) with OPT_ACCURACY: its
// mad_build string reads "NDEBUG FPM_64BIT OPT_ACCURACY", because the
// OPTIMIZE option of libmad's CMake file does not end up as SPEED. Every
// multiply is a 64-bit product, rounded and shifted back to 28 fraction
// bits, and every multiply-accumulate step rounds its own product. The port
// wraps on 32-bit overflow like the C code, with one exception that follows
// the machine code clang emits at -Os (see mulWide).
//
// The structure follows the C library: a Stream holds the input buffer and
// the decoding state, Frame.Decode decodes one frame into subband samples
// and a Synth turns a Frame into PCM. Decoding options
// (MAD_OPTION_IGNORECRC, MAD_OPTION_HALFSAMPLERATE), stream skipping and the
// timer are not ported: audiowaveform uses none of them.
//
// Copyright notice of the original library:
//
//	libmad - MPEG audio decoder library
//	Copyright (C) 2000-2004 Underbit Technologies, Inc.
//
//	This program is free software; you can redistribute it and/or modify
//	it under the terms of the GNU General Public License as published by
//	the Free Software Foundation; either version 2 of the License, or
//	(at your option) any later version.
//
//	This program is distributed in the hope that it will be useful,
//	but WITHOUT ANY WARRANTY; without even the implied warranty of
//	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//	GNU General Public License for more details.
//
// This port is distributed as part of audiowaveform-go under the GNU General
// Public License version 3 or later, as the "any later version" clause above
// allows.
package mad
