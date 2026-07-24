//go:build testserver

package server

import (
	"context"
	"errors"
	"strings"
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
	messages []onboardingMessage,
) (nimInterviewResult, error) {
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
			return nimInterviewResult{}, errors.New(rule.Error)
		}
		preferences := make([]nimPreference, 0, len(rule.Preferences))
		for dishName, weight := range rule.Preferences {
			preferences = append(preferences, nimPreference{
				DishName: dishName,
				Weight:   weight,
			})
		}
		return nimInterviewResult{
			Reply:       rule.Reply,
			Complete:    rule.Complete,
			Preferences: preferences,
		}, nil
	}
	return nimInterviewResult{}, errors.New("scripted NIM has no matching rule")
}

func NewWithScriptedNIMRulesForTest(
	config Config,
	rules []ScriptedNIMRuleForTest,
) (*App, error) {
	return newApp(config, newDecisionRandom(), &ruleNIM{rules: rules})
}
