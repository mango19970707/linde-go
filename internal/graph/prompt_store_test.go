package graph

import (
	"context"
	"errors"
)

type stubPromptStore struct {
	prompts map[string]PromptDefinition
	err     error
}

func (s stubPromptStore) List(ctx context.Context) ([]PromptDefinition, error) {
	if s.err != nil {
		return nil, s.err
	}
	result := make([]PromptDefinition, 0, len(s.prompts))
	for _, prompt := range s.prompts {
		result = append(result, prompt)
	}
	return result, nil
}

func (s stubPromptStore) Get(ctx context.Context, key string) (PromptDefinition, error) {
	if s.err != nil {
		return PromptDefinition{}, s.err
	}
	prompt, ok := s.prompts[key]
	if !ok {
		return PromptDefinition{}, errors.New("prompt not found")
	}
	return prompt, nil
}

func (s stubPromptStore) Upsert(ctx context.Context, prompt PromptDefinition) error {
	if s.err != nil {
		return s.err
	}
	return nil
}

func (s stubPromptStore) Reset(ctx context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	return nil
}
