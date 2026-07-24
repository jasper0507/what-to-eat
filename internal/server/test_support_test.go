package server

import "math/rand"

func NewWithDecisionRandomSeedForTest(config Config, seed int64) (*App, error) {
	return newApp(config, rand.New(rand.NewSource(seed)))
}
