// +build Ignore

package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tomclegg/pcm"
)

func main() {
	a := &pcm.Analyzer{
		Window:       400 * time.Millisecond,
		ObserveEvery: 400 * time.Millisecond,
		ObserveRMS: func(rms float64) {
			fmt.Printf("%*s%f\n", int(rms*40), "|", rms)
		},
	}
	a.UseMIMEType("audio/L16; rate=44100; channels=2")
	_, err := io.Copy(a, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
