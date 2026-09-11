package studio

import (
	"encoding/binary"
	"math"
	"os"
)

// composeTemplateMusic writes a deterministic, original instrumental bed for fixed
// templates. Its event clock is a 144 BPM quarter note, so every percussive attack
// and harmonic change stays locked to the edit's beat grid.
func composeTemplateMusic(path string, seconds int, requestedBPM ...int) error {
	const rate = 48000
	if seconds <= 0 {
		return os.WriteFile(path, nil, 0600)
	}
	bpm := 144.0
	if len(requestedBPM) > 0 && requestedBPM[0] >= 96 && requestedBPM[0] <= 180 {
		bpm = float64(requestedBPM[0])
	}
	beat := 60 / bpm
	count := rate * seconds
	data := make([]byte, 44+count*4)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], rate)
	binary.LittleEndian.PutUint32(data[28:], rate*4)
	binary.LittleEndian.PutUint16(data[32:], 4)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(count*4))

	// Bright but warm minor-key progression: Am-F-C-G. Values are MIDI notes;
	// the low octave is the sub bass and the upper octave carries the hook.
	chords := [][4]int{{45, 48, 52, 57}, {41, 45, 48, 53}, {48, 52, 55, 60}, {43, 47, 50, 55}}
	arp := []int{0, 2, 3, 2, 1, 2, 3, 2}
	melody := []int{69, 72, 76, 79, 76, 72, 74, 76, 81, 79, 76, 72, 69, 72, 76, 74}
	noteHz := func(m int) float64 { return 440 * math.Pow(2, float64(m-69)/12) }

	// Deterministic noise keeps snare/clap transients repeatable across renders.
	noise := func(n int) float64 {
		x := uint32(n)*747796405 + 2891336453
		x = ((x >> ((x >> 28) + 4)) ^ x) * 277803737
		return float64(int32((x>>22)^x)) / 2147483648
	}
	frac := func(age, attack, release float64) float64 {
		if age < 0 || age > release {
			return 0
		}
		return (1 - math.Exp(-age*attack)) * math.Exp(-age*(3.8/release))
	}
	section := func(t float64) float64 {
		switch {
		case t < 10:
			return .35 + .65*t/10 // build into the first full phrase
		case t < 20:
			return 1
		case t < 22:
			return .12 * (1 - (t-20)/2) // intentional breath before the drop
		case t < 50:
			return 1.08
		case t < 57:
			return 1.22
		default:
			return math.Max(0, 1-(t-57)/float64(maxInt(seconds-57, 1)))
		}
	}
	for i := 0; i < count; i++ {
		t := float64(i) / rate
		energy := section(t)
		// Restart the groove at the 22s drop so the edit's hero cut lands on beat 1.
		clock := t
		if t >= 22 {
			clock = t - 22
		}
		beatNo := int(math.Floor(clock / beat))
		beatAge := clock - float64(beatNo)*beat
		barNo := beatNo / 4
		chord := chords[barNo%len(chords)]
		var left, right float64

		// Wide, gently filtered pad. Detuned partials give it body without harsh aliasing.
		padAge := math.Mod(clock, beat*4)
		padEnv := .72 + .28*math.Sin(math.Pi*padAge/(beat*4))
		for j, m := range chord[1:] {
			f := noteHz(m + 12)
			phase := 2 * math.Pi * f * t
			voice := math.Sin(phase) + .23*math.Sin(phase*2+.3) + .08*math.Sin(phase*3-.2)
			amp := (.034 - float64(j)*.004) * padEnv * energy
			left += amp * voice
			right += amp * (.93*voice + .08*math.Sin(phase*1.003))
		}

		// Four-on-the-floor kick; the downward pitch sweep creates a clear transient.
		if t >= 10 && !(t >= 20 && t < 22) && t < float64(seconds) {
			age := beatAge
			if beatNo%4 == 0 || beatNo%4 == 1 || beatNo%4 == 2 || beatNo%4 == 3 {
				f := 155 - 95*math.Min(age*18, 1)
				kick := .42 * math.Sin(2*math.Pi*f*age) * math.Exp(-age*23)
				kick += .12 * math.Sin(2*math.Pi*55*age) * math.Exp(-age*10)
				left += kick * energy
				right += kick * energy
			}
		}
		// Snare/clap on beats 2 and 4, with a short tonal body and stereo noise.
		if beatNo%4 == 1 || beatNo%4 == 3 {
			snare := .12 * noise(i) * math.Exp(-beatAge*30)
			snare += .07 * math.Sin(2*math.Pi*190*beatAge) * math.Exp(-beatAge*18)
			left += snare * energy
			right += snare * (1.06 * energy)
		}
		// Offbeat hats and 16th-note ghost hats add forward motion.
		halfAge := math.Mod(clock, beat/2)
		hat := .028 * noise(i+17) * math.Exp(-halfAge*65)
		left += hat * energy
		right += .82 * hat * energy
		if beatNo%2 == 1 {
			openAge := math.Mod(clock-beat/2, beat)
			open := .018 * noise(i+71) * math.Exp(-math.Max(openAge, 0)*12)
			left += open * energy
			right += .76 * open * energy
		}

		// Root bass follows each beat. A kick-shaped sidechain leaves room for the attack.
		root := noteHz(chord[0] - 12)
		bass := .18 * (math.Sin(2*math.Pi*root*clock) + .18*math.Sin(2*math.Pi*root*2*clock))
		bass *= math.Exp(-beatAge*1.8) + .32
		duck := 1 - .55*math.Exp(-beatAge*20)
		left += bass * duck * energy
		right += bass * duck * energy * .96

		// Eighth-note arpeggio plus a quarter-note hook, rising in the final lift.
		step := int(math.Floor(clock / (beat / 2)))
		arpAge := math.Mod(clock, beat/2)
		arpNote := noteHz(chord[arp[step%len(arp)]] + 12)
		arpTone := .065 * (math.Sin(2*math.Pi*arpNote*clock) + .16*math.Sin(2*math.Pi*arpNote*2*clock)) * math.Exp(-arpAge*7)
		left += arpTone * energy
		right += arpTone * 1.12 * energy
		leadAge := math.Mod(clock, beat/2)
		leadNote := noteHz(melody[(step/2+barNo*2)%len(melody)] + func() int {
			if t >= 50 {
				return 12
			}
			return 0
		}())
		lead := .075 * (math.Sin(2*math.Pi*leadNote*clock) + .2*math.Sin(2*math.Pi*leadNote*2*clock)) * frac(leadAge, 35, beat/2)
		left += lead * energy
		right += lead * 1.08 * energy

		// Noisy risers and impacts mark the build/drop transitions.
		for _, start := range []float64{8, 20, 48} {
			age := t - start
			if age >= 0 && age < 2 {
				riser := .035 * noise(i+int(start*100)) * (age / 2) * math.Exp(-age*.7)
				left += riser * math.Sin(2*math.Pi*(600+2200*age)*t)
				right += .8 * riser * math.Sin(2*math.Pi*(620+2250*age)*t)
			}
		}
		for _, at := range []float64{22, 50} {
			age := t - at
			if age >= 0 && age < .8 {
				impact := .3 * math.Sin(2*math.Pi*(90-45*math.Min(age*2, 1))*age) * math.Exp(-age*7)
				impact += .08 * noise(i+int(at*1000)) * math.Exp(-age*20)
				left += impact
				right += .98 * impact
			}
		}
		// Soft saturation and a two-second release prevent clicks and clipping.
		fade := math.Min(1, t/.12) * math.Min(1, (float64(seconds)-t)/2)
		left = math.Tanh(left*1.22) * .82 * fade
		right = math.Tanh(right*1.22) * .82 * fade
		binary.LittleEndian.PutUint16(data[44+i*4:], uint16(int16(math.Max(-.98, math.Min(.98, left))*32767)))
		binary.LittleEndian.PutUint16(data[46+i*4:], uint16(int16(math.Max(-.98, math.Min(.98, right))*32767)))
	}
	return os.WriteFile(path, data, 0600)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
