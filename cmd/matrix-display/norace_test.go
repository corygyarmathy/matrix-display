//go:build !race

package main

// raceDetector is whether the tests run with -race, which slows rendering
// several times over.
const raceDetector = false
