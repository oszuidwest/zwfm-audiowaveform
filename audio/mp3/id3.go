package mp3

// id3QuerySize is ID3_TAG_QUERYSIZE.
const id3QuerySize = 10

// id3TagQuery is id3_tag_query from libid3tag 0.16.4: it returns the size of
// the ID3 tag at the start of data, a negative size for an ID3v2 footer, or
// 0 when data does not start with a tag.
func id3TagQuery(data []byte) int64 { //nolint:gocyclo // mirrors id3_tag_query's header checks
	if len(data) >= 3 && data[0] == 'T' && data[1] == 'A' && data[2] == 'G' {
		return 128
	}

	if len(data) >= 10 &&
		((data[0] == 'I' && data[1] == 'D' && data[2] == '3') ||
			(data[0] == '3' && data[1] == 'D' && data[2] == 'I')) &&
		data[3] < 0xff && data[4] < 0xff &&
		data[6] < 0x80 && data[7] < 0x80 && data[8] < 0x80 && data[9] < 0x80 {
		flags := data[5]
		var size int64
		for _, b := range data[6:10] {
			size = size<<7 | int64(b&0x7f)
		}

		if data[0] == '3' {
			return -size - 10
		}

		const footerPresent = 0x10
		if flags&footerPresent != 0 {
			size += 10
		}
		return 10 + size
	}

	return 0
}
