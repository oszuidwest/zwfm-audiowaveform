package waveform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

// ReadJSON reads the JSON waveform format (version 1 or 2).
//
// Like audiowaveform, it reads the object as a stream: "data" values are
// converted with the "bits" value seen so far (16 until then), unknown keys
// are rejected, missing "channels" or "bits" default to 1 and 16, and
// anything after the object is ignored.
func ReadJSON(r io.Reader) (*Data, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	d := &Data{Channels: 1, Bits: 16}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()

	var foundVersion, foundLength, foundData bool
	tok, err := dec.Token()
	empty := err == io.EOF // empty input only misses values
	if !empty && (err != nil || tok != json.Delim('{')) {
		return nil, errors.New("Invalid JSON structure: expected object")
	}
	for !empty {
		tok, err := dec.Token()
		if err != nil {
			return nil, jsonSyntaxError(input, err)
		}
		if tok == json.Delim('}') {
			break
		}
		key, ok := tok.(string)
		if !ok {
			return nil, jsonSyntaxError(input, nil)
		}
		if key == "data" {
			if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
				return nil, errors.New("Expected data to be an array")
			}
			foundData = true
			if err := readJSONData(dec, d); err != nil {
				return nil, err
			}
			continue
		}
		tok, err = dec.Token()
		num, isNum := tok.(json.Number)
		if err != nil || !isNum {
			if !isJSONKey(key) {
				return nil, fmt.Errorf("Unexpected value: %s", key)
			}
			return nil, fmt.Errorf("Expected %s to be a number", key)
		}
		f, _ := num.Float64()
		v := int(f)
		switch key {
		case "version":
			foundVersion = true
			if v != 1 && v != 2 {
				return nil, errors.New("Invalid version: expecting 1 or 2")
			}
		case "channels":
			if d.Channels = v; v <= 0 {
				return nil, fmt.Errorf("Invalid channels: %d", v)
			}
		case "sample_rate":
			if d.SampleRate = v; v <= 0 {
				return nil, fmt.Errorf("Invalid sample rate: %d", v)
			}
		case "samples_per_pixel":
			if d.SamplesPerPixel = v; v <= 0 {
				return nil, fmt.Errorf("Invalid scale: %d", v)
			}
		case "bits":
			if d.Bits = v; v != 8 && v != 16 {
				return nil, errors.New("Invalid bits: expecting 8 or 16")
			}
		case "length":
			foundLength = true
			if v < 0 {
				return nil, fmt.Errorf("Invalid length: %d", v)
			}
		default:
			return nil, fmt.Errorf("Unexpected value: %s", key)
		}
	}
	switch {
	case !foundVersion:
		return nil, errors.New("Missing value: version")
	case !foundLength:
		return nil, errors.New("Missing value: length")
	case d.SampleRate == 0:
		return nil, errors.New("Missing value: sample_rate")
	case d.SamplesPerPixel == 0:
		return nil, errors.New("Missing value: samples_per_pixel")
	case !foundData:
		return nil, errors.New("Missing value: data")
	}
	return d, nil
}

func isJSONKey(key string) bool {
	switch key {
	case "version", "channels", "sample_rate", "samples_per_pixel", "bits", "length":
		return true
	}
	return false
}

func readJSONData(dec *json.Decoder, d *Data) error {
	for {
		tok, err := dec.Token()
		if err == nil && tok == json.Delim(']') {
			return nil
		}
		num, ok := tok.(json.Number)
		if err != nil || !ok {
			return errors.New("Invalid JSON structure: the data array must only contain numbers")
		}
		v, _ := num.Float64()
		if d.Bits == 8 {
			if v < math.MinInt8 || v > math.MaxInt8 {
				return fmt.Errorf("Data value out of range: %.6g", v)
			}
			d.Samples = append(d.Samples, int16(v*256))
		} else {
			// No range check: audiowaveform's check is always true.
			d.Samples = append(d.Samples, int16(int32(v)))
		}
	}
}

// jsonSyntaxError reports where the JSON broke. Like pdjson, the "column" is
// the byte position in the whole input.
func jsonSyntaxError(input []byte, err error) error {
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		return errors.New("Invalid JSON format")
	}
	before := input[:min(int(se.Offset), len(input))]
	line := bytes.Count(before, []byte("\n")) + 1
	return fmt.Errorf("Invalid JSON format at line %d, column %d", line, len(before)+1)
}
