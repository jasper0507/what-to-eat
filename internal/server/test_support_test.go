package server

import (
	"context"
	"errors"
	"math/rand"
)

func NewWithDecisionRandomSeedForTest(config Config, seed int64) (*App, error) {
	nim, err := newNIMAdapter(config.NIM)
	if err != nil {
		return nil, err
	}
	return newApp(config, rand.New(rand.NewSource(seed)), nim)
}

type ScriptedNIMStep struct {
	Reply       string
	Complete    bool
	Preferences map[string]float64
	Error       string
	Started     chan<- struct{}
	Release     <-chan struct{}
}

type scriptedNIM struct {
	steps []ScriptedNIMStep
}

func (s *scriptedNIM) Respond(
	callContext context.Context,
	_ []onboardingMessage,
) (nimInterviewResult, error) {
	if len(s.steps) == 0 {
		return nimInterviewResult{}, errors.New("scripted NIM has no response")
	}
	step := s.steps[0]
	s.steps = s.steps[1:]
	if step.Started != nil {
		close(step.Started)
	}
	if step.Release != nil {
		select {
		case <-step.Release:
		case <-callContext.Done():
			return nimInterviewResult{}, callContext.Err()
		}
	}
	if step.Error != "" {
		return nimInterviewResult{}, errors.New(step.Error)
	}
	preferences := make([]nimPreference, 0, len(step.Preferences))
	for dishName, weight := range step.Preferences {
		preferences = append(preferences, nimPreference{
			DishName: dishName,
			Weight:   weight,
		})
	}
	return nimInterviewResult{
		Reply:       step.Reply,
		Complete:    step.Complete,
		Preferences: preferences,
	}, nil
}

func NewWithScriptedNIMForTest(
	config Config,
	steps []ScriptedNIMStep,
) (*App, error) {
	return newApp(config, newDecisionRandom(), &scriptedNIM{steps: steps})
}
