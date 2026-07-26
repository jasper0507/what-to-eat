//go:build testserver

package server

import (
	"context"
	"errors"
	"strings"

	"github.com/jasper0507/what-to-eat/internal/meal"
	"github.com/jasper0507/what-to-eat/internal/onboarding"
)

type ScriptedNIMRuleForTest struct {
	Contains    string
	Reply       string
	Complete    bool
	Preferences map[string]float64
	Error       string
}

type ruleNIM struct {
	rules []ScriptedNIMRuleForTest
}

func (s *ruleNIM) Respond(
	_ context.Context,
	messages []onboarding.Message,
) (onboarding.NIMResult, error) {
	var transcript strings.Builder
	for _, message := range messages {
		if message.Role == "user" {
			transcript.WriteString(message.Content)
			transcript.WriteByte('\n')
		}
	}
	for _, rule := range s.rules {
		if !strings.Contains(transcript.String(), rule.Contains) {
			continue
		}
		if rule.Error != "" {
			return onboarding.NIMResult{}, errors.New(rule.Error)
		}
		preferences := make([]onboarding.NIMPreference, 0, len(rule.Preferences))
		for dishName, weight := range rule.Preferences {
			preferences = append(preferences, onboarding.NIMPreference{
				DishName: dishName,
				Weight:   weight,
			})
		}
		return onboarding.NIMResult{
			Reply:       rule.Reply,
			Complete:    rule.Complete,
			Preferences: preferences,
		}, nil
	}
	return onboarding.NIMResult{}, errors.New("scripted NIM has no matching rule")
}

func NewWithScriptedNIMRulesForTest(
	config Config,
	rules []ScriptedNIMRuleForTest,
) (*App, error) {
	return newApp(config, meal.NewDecisionRandom(), &ruleNIM{rules: rules})
}
