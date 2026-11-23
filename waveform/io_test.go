package waveform

import (
	"bytes"
	"os"
	"testing"
)

func TestFormatsMatchUpstreamFixtures(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	const base = "test_file_stereo_8bit_64spp_wav"
	fromDat, err := ReadDat(bytes.NewReader(read(base + ".dat")))
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := ReadJSON(bytes.NewReader(read(base + ".json")))
	if err != nil {
		t.Fatal(err)
	}
	twoChannels, err := ReadDat(bytes.NewReader(read("test_file_2channel_8bit_64spp_wav.dat")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		data  *Data
		write func(*Data, *bytes.Buffer) error
		want  string
	}{
		{"dat to dat", fromDat, func(d *Data, b *bytes.Buffer) error { return d.WriteDat(b, 8) }, base + ".dat"},
		{"dat to json", fromDat, func(d *Data, b *bytes.Buffer) error { return d.WriteJSON(b, 8) }, base + ".json"},
		{"dat to txt", fromDat, func(d *Data, b *bytes.Buffer) error { return d.WriteText(b, 8) }, base + ".txt"},
		{"json to dat", fromJSON, func(d *Data, b *bytes.Buffer) error { return d.WriteDat(b, 8) }, base + ".dat"},
		{"two channel dat", twoChannels, func(d *Data, b *bytes.Buffer) error { return d.WriteDat(b, 8) }, "test_file_2channel_8bit_64spp_wav.dat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := tt.write(tt.data, &got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), read(tt.want)) {
				t.Errorf("output differs from %s", tt.want)
			}
		})
	}
}
