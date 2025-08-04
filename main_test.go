package main

import (
	"testing"
)

func TestParseAudioInfo(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected AudioInfo
	}{
		{
			name: "24-bit 96kHz FLAC",
			input: `Input File     : 'test.flac'
Channels       : 2
Sample Rate    : 96000
Precision      : 24-bit
Duration       : 00:03:45.23 = 21621600 samples ~ 16216.2 CDDA sectors
File Size      : 64.5M
Bit Rate       : 2.41M
Sample Encoding: 24-bit Signed Integer PCM`,
			expected: AudioInfo{Bits: 24, Rate: 96000},
		},
		{
			name: "16-bit 44.1kHz FLAC",
			input: `Input File     : 'test.flac'
Channels       : 2
Sample Rate    : 44100
Precision      : 16-bit
Duration       : 00:03:45.23 = 9953100 samples ~ 16216.2 CDDA sectors
File Size      : 39.5M
Bit Rate       : 1.41M
Sample Encoding: 16-bit Signed Integer PCM`,
			expected: AudioInfo{Bits: 16, Rate: 44100},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAudioInfo(tc.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Bits != tc.expected.Bits {
				t.Errorf("Expected bits %d, got %d", tc.expected.Bits, result.Bits)
			}

			if result.Rate != tc.expected.Rate {
				t.Errorf("Expected rate %d, got %d", tc.expected.Rate, result.Rate)
			}
		})
	}
}

func TestDetermineConversion(t *testing.T) {
	testCases := []struct {
		name               string
		input              AudioInfo
		expectedConversion bool
		expectedBitrate    []string
		expectedSampleRate []string
	}{
		{
			name:               "24-bit 96kHz needs conversion",
			input:              AudioInfo{Bits: 24, Rate: 96000},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L", "48000"},
		},
		{
			name:               "16-bit 44.1kHz no conversion",
			input:              AudioInfo{Bits: 16, Rate: 44100},
			expectedConversion: false,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "16-bit 88.2kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 88200},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			needsConversion, bitrateArgs, sampleRateArgs := determineConversion(&tc.input)

			if needsConversion != tc.expectedConversion {
				t.Errorf("Expected conversion %v, got %v", tc.expectedConversion, needsConversion)
			}

			if len(bitrateArgs) != len(tc.expectedBitrate) {
				t.Errorf("Expected bitrate args %v, got %v", tc.expectedBitrate, bitrateArgs)
			} else {
				for i, arg := range bitrateArgs {
					if arg != tc.expectedBitrate[i] {
						t.Errorf("Expected bitrate arg %s, got %s", tc.expectedBitrate[i], arg)
					}
				}
			}

			if len(sampleRateArgs) != len(tc.expectedSampleRate) {
				t.Errorf("Expected sample rate args %v, got %v", tc.expectedSampleRate, sampleRateArgs)
			} else {
				for i, arg := range sampleRateArgs {
					if arg != tc.expectedSampleRate[i] {
						t.Errorf("Expected sample rate arg %s, got %s", tc.expectedSampleRate[i], arg)
					}
				}
			}
		})
	}
}
