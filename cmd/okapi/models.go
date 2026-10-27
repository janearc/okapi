package main

import (
	"github.com/janearc/okapi/internal/draft"
	"github.com/janearc/okapi/internal/ollama"
)

// modelClient is the model draft asks, and what draft needs to know
// of it.
type modelClient interface {
	draft.Chatter
	// Up says whether the model's endpoint answers at all.
	Up() bool
	// URL is the endpoint the client talks to.
	URL() string
	// Model is the model the client asks.
	Model() string
}

// modelSource makes the client draft asks its model through.
type modelSource interface {
	// Client is the client for the named model, or for the one the
	// environment names when model is blank.
	Client(model string) modelClient
}

// ollamaModels is the model ollama serves on this machine.
type ollamaModels struct{}

// Client is the environment's client, asking model when one is named.
func (ollamaModels) Client(model string) modelClient {
	client := ollama.FromEnv(ollama.HTTP())
	if model != "" {
		return client.WithModel(model)
	}
	return client
}
