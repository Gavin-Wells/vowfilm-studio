package studio

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateMusicIsBeatLockedStereo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "score.wav")
	if err := composeTemplateMusic(path, 24, 144); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw[:4]) != "RIFF" || binary.LittleEndian.Uint32(raw[24:]) != 48000 || binary.LittleEndian.Uint16(raw[22:]) != 2 {
		t.Fatalf("expected 48kHz stereo WAV header")
	}
	if len(raw) < 44+48000*24*4 || binary.LittleEndian.Uint32(raw[40:]) == 0 {
		t.Fatalf("expected a complete audio payload")
	}
	peak, sum := 0, 0
	for i := 44; i+3 < len(raw); i += 4 {
		l := int(int16(binary.LittleEndian.Uint16(raw[i:])))
		if l < 0 {
			l = -l
		}
		if l > peak {
			peak = l
		}
		sum += l
	}
	if peak == 0 || peak >= 32767 || sum == 0 {
		t.Fatalf("expected audible headroom-preserving score (peak=%d)", peak)
	}
}
