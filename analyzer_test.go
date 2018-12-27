package pcm

import (
	"math"
	"testing"
	"time"
)

func TestObserve(t *testing.T) {
	for _, trial := range []struct {
		interval   time.Duration
		frequency  float64
		scale      float64
		expectRMS  float64
		expectPeak float64
	}{
		{time.Second, 16000, 32768, -1.500468, 0},
		{time.Second, 16000, 32000, -1.603469, -0.103},
		{time.Second, 16000, 16384, -4.510852, -3.0103},
		{time.Second / 2, 16000, 16000, -4.737631, -3.1133},
		{time.Second * 2, 16000, 16000, 0, 0}, // reporting interval > input size
	} {
		var lastRMS, lastPeak float64
		a := Analyzer{
			Window:       trial.interval,
			ObserveEvery: trial.interval,
			ObserveRMS:   func(rms float64) { lastRMS = rms },
			ObservePeak:  func(peak float64) { lastPeak = peak },
		}
		a.UseMIMEType("audio/L16; rate=44100; channels=2")
		data := make([]byte, 44100*4)
		for i := 0; i < len(data); i += 4 {
			s := int16(math.Sin(float64(i)*2*math.Pi/trial.frequency/4) * trial.scale)
			data[i] = byte(s & 0xff)
			data[i+1] = byte(s >> 8)
			data[i+2] = byte(s & 0xff)
			data[i+3] = byte(s >> 8)
		}
		a.Write(data)
		if math.Abs(lastRMS-trial.expectRMS) > 0.000001 {
			t.Errorf("bad RMS %f (trial %v)", lastRMS, trial)
		}
		if math.Abs(lastPeak-trial.expectPeak) > 0.000001 {
			t.Errorf("bad peak %f (trial %v)", lastPeak, trial)
		}
	}
}
