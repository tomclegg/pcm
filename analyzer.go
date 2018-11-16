package pcm

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Analyzer struct {
	SampleRate   int64
	WordSize     uint
	Channels     int
	LittleEndian bool
	Signed       bool
	Window       time.Duration
	ObserveEvery time.Duration
	ObserveRMS   func(float64)

	pending   []byte  // bytes written but not yet decoded
	squares   []int64 // values added to rolling sum
	len       int
	sum       int64 // rolling sum
	next      int   // index (in squares) of next sample
	countdown int64 // samples until next observe
}

var (
	ErrBadParameters = errors.New("bad Analyzer parameters")
)

func (a *Analyzer) MIMEParams(hdr string) error {
	var rate, channels int64
	for i, s := range strings.Split(hdr, ";") {
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
		return fmt.Errorf("incomplete header (need rate and channels): %q", hdr)
	}
	a.SampleRate = rate
	a.Channels = int(channels)
	a.WordSize = 16
	a.LittleEndian = true
	a.Signed = true
	return nil
}

func (a *Analyzer) Write(p []byte) (int, error) {
	if a.squares == nil {
		if a.Channels < 1 || a.WordSize == 0 || a.WordSize&7 != 0 || a.WordSize >= 64 || a.SampleRate < 1 || a.SampleRate*int64(a.ObserveEvery)/int64(time.Second) < 1 {
			return 0, ErrBadParameters
		}
		a.next = -1
		a.squares = make([]int64, 0, int(int64(a.Channels)*a.SampleRate*int64(a.Window)/int64(time.Second)))
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
	for len(p) > a.Channels*int(a.WordSize)/8 {
		for c := 0; c < a.Channels; c++ {
			var s int64
			for b := uint(0); b < a.WordSize; b += 8 {
				s = (s << (bigshift * 8)) | (int64(p[0]) << (littleshift * b))
				p = p[1:]
			}
			if a.Signed {
				if s&(1<<(a.WordSize-1)) != 0 {
					s -= 1 << a.WordSize
				}
			} else {
				s -= 1 << (a.WordSize - 1)
			}
			square := s * s

			if a.next++; a.next == cap(a.squares) {
				a.next = 0
			} else if a.next == len(a.squares) {
				a.squares = append(a.squares, 0)
			}
			a.sum += square - a.squares[a.next]
			a.squares[a.next] = square
		}
		if a.countdown--; a.countdown <= 0 {
			if a.countdown == 0 {
				a.ObserveRMS(math.Sqrt(float64(a.sum/int64(len(a.squares)))) / float64(uint64(1)<<(a.WordSize-1)))
			}
			a.countdown = a.SampleRate * int64(a.ObserveEvery) / int64(time.Second)
		}
	}
	a.pending = append([]byte(nil), p...)
	return n, nil
}
