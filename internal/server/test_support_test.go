package server

import (
	"context"
	"errors"
	"math/rand"

	"github.com/jasper0507/what-to-eat/internal/meal"
	"github.com/jasper0507/what-to-eat/internal/onboarding"
)

func NewWithDecisionRandomSeedForTest(config Config, seed int64) (*App, error) {
	nim, err := onboarding.NewNIMAdapter(config.NIM)
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
	_ []onboarding.Message,
) (onboarding.NIMResult, error) {
	if len(s.steps) == 0 {
		return onboarding.NIMResult{}, errors.New("scripted NIM has no response")
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
			return onboarding.NIMResult{}, callContext.Err()
		}
	}
	if step.Error != "" {
		return onboarding.NIMResult{}, errors.New(step.Error)
	}
	preferences := make([]onboarding.NIMPreference, 0, len(step.Preferences))
	for dishName, weight := range step.Preferences {
		preferences = append(preferences, onboarding.NIMPreference{
			DishName: dishName,
			Weight:   weight,
		})
	}
	return onboarding.NIMResult{
		Reply:       step.Reply,
		Complete:    step.Complete,
		Preferences: preferences,
	}, nil
}

func NewWithScriptedNIMForTest(
	config Config,
	steps []ScriptedNIMStep,
) (*App, error) {
	return newApp(config, meal.NewDecisionRandom(), &scriptedNIM{steps: steps})
}
