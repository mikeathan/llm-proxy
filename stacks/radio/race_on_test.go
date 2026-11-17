//go:build race

package radio

// underRace is true when the race detector is on: it slows seeding and every look by 10x or more, so a test that measures wall time says
// nothing about the product under it.
const underRace = true
