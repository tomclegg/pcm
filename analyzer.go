// Package pcm decodes PCM audio data and reports loudness levels.
//
// The loudness calculation is somewhat arbitrary, and does not
// account for factors like DC offset in the input signal. It might be
// good enough to reveal loudness trends, detect when loudness departs
// from an expected range, etc. YMMV.
package pcm

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Analyzer decodes PCM audio data, computes RMS loudness over a
// specified window, and reports it at specified intervals.
//
// Do not modify an Analyzer's fields after calling Write.
type Analyzer struct {
	SampleRate   int64
	WordSize     uint
	Channels     int
	LittleEndian bool
	Signed       bool

	// Duration of loudness computation window. Typical values are
	// 400*time.Millisecond (momentary loudness) and 3*time.Second
	// (short term loudness).
	Window time.Duration

	// Time interval (relative to the audio data, not wall clock
	// time) between calls to ObserveRMS/ObservePeak.
	ObserveEvery time.Duration

	// Func to call with current window loudness in dB.
	ObserveRMS func(rms float64)

	pending   []byte  // bytes written but not yet decoded
	squares   []int64 // values added to rolling sum
	nwindow   int64   // window size in #samples
	sum       int64   // rolling sum
	next      int     // index (in squares) of next sample
	countdown int64   // samples until next observe
	nobserve  int64   // samples per ObserveEvery interval

	// Func to call with peak sample from last interval in dB.
	ObservePeak func(peak float64)
	peak        int64 // max sample amplitude since last call to ObservePeak

	wordMax float64 // max sample amplitude for this word size
}

var ErrBadParameters = errors.New("bad Analyzer parameters")

// UseMIMEType sets the Analyzer's SampleRate, WordSize, Channels,
// LittleEndian, and Signed fields to match the given MIME type (e.g.,
// a Content-Type header value).  It returns an error if the MIME type
// is unsupported or not understood.
//
// Currently, little-endian signed 16-bit streams are supported, as in
// "audio/L16; rate=44100; channels=2".
func (a *Analyzer) UseMIMEType(mt string) error {
	var rate, channels int64
	for i, s := range strings.Split(mt, ";") {
		s = strings.TrimSpace(s)
		if i == 0 {
			if !strings.HasPrefix(s, "audio/L16") {
				return fmt.Errorf("unrecognized MIME type %q", s)
			}
			continue
		}
		kv := strings.Split(strings.ToLower(s), "=")
		if len(kv) != 2 {
			continue
		}
		var dst *int64
		switch kv[0] {
		case "rate":
			dst = &rate
		case "channels":
			dst = &channels
		default:
			continue
		}
		*dst, _ = strconv.ParseInt(kv[1], 10, 64)
		if *dst < 1 {
			return fmt.Errorf("invalid %s %q", kv[0], kv[1])
		}
	}
	if rate == 0 || channels == 0 {
		return fmt.Errorf("incomplete header (need rate and channels): %q", mt)
	}
	a.SampleRate = rate
	a.Channels = int(channels)
	a.WordSize = 16
	a.LittleEndian = true
	a.Signed = true
	return nil
}

// Write decodes and analyzes the supplied PCM audio data, calling
// ObserveRMS and ObservePeak as needed.
func (a *Analyzer) Write(p []byte) (int, error) {
	if a.nwindow == 0 {
		if a.Channels < 1 || a.WordSize == 0 || a.WordSize&7 != 0 || a.WordSize >= 64 || a.SampleRate < 1 || a.SampleRate*int64(a.ObserveEvery)/int64(time.Second) < 1 {
			return 0, ErrBadParameters
		}
		a.nwindow = int64(a.Channels) * a.SampleRate * int64(a.Window) / int64(time.Second)
		if a.Window != a.ObserveEvery {
			a.squares = make([]int64, 0, int(a.nwindow))
		}
		a.next = -1
		a.wordMax = float64(uint64(1) << (a.WordSize - 1))
		a.nobserve = a.SampleRate*int64(a.ObserveEvery)/int64(time.Second) - 1
		a.countdown = a.nobserve
	}

	var bigshift, littleshift uint
	if a.LittleEndian {
		littleshift = 1
	} else {
		bigshift = 1
	}
	n := len(p)
	if len(a.pending) > 0 {
		p = append(a.pending, p...)
	}
	for len(p) >= a.Channels*int(a.WordSize)/8 {
		for c := 0; c < a.Channels; c++ {
			var s int64
			for b := uint(0); b < a.WordSize; b += 8 {
				s = (s << (bigshift * 8)) | (int64(p[0]) << (littleshift * b))
				p = p[1:]
			}
			if a.Signed {
				if s&(1<<(a.WordSize-1)) != 0 {
					s = (s ^ (1<<a.WordSize - 1)) + 1
				}
			} else {
				s -= 1 << (a.WordSize - 1)
			}
			square := s * s

			a.sum += square
			if a.squares != nil {
				if a.next++; a.next == cap(a.squares) {
					a.next = 0
				} else if a.next == len(a.squares) {
					a.squares = append(a.squares, 0)
				}
				a.sum -= a.squares[a.next]
				a.squares[a.next] = square
			}

			if a.peak < s {
				a.peak = s
			} else if a.peak < -s {
				a.peak = -s
			}
		}
		if a.countdown--; a.countdown == 0 {
			n := a.nwindow
			if a.squares != nil {
				n = int64(len(a.squares))
			}
			if a.ObserveRMS != nil {
				a.ObserveRMS(10 * math.Log10(math.Sqrt(float64(a.sum/n))/a.wordMax))
			}
			if a.ObservePeak != nil {
				a.ObservePeak(10 * math.Log10(float64(a.peak)/a.wordMax))
				a.peak = 0
			}
			if a.squares == nil {
				a.sum = 0
			}
			a.countdown = a.nobserve
		}
	}
	a.pending = append([]byte(nil), p...)
	return n, nil
}
