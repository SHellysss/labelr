package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/ui"
)

// selectModel fetches available models for the provider and lets the user pick one.
// For cloud providers: fetches from models.dev (cached).
// For Ollama: queries local running models.
func selectModel(providerName string) (string, error) {
	if providerName == "ollama" {
		return selectOllamaModel()
	}
	return selectCloudModel(providerName)
}

func selectCloudModel(providerName string) (string, error) {
	var models []string
	var fetchErr error

	err := spinner.New().
		Title("Fetching available models...").
		Action(func() {
			models, fetchErr = ai.FetchModelsForProvider(providerName)
		}).
		Run()
	if err != nil {
		return "", err
	}
	if fetchErr != nil {
		ui.Error(fmt.Sprintf("Could not fetch models: %v", fetchErr))
		// Fall back to text input
		var model string
		huh.NewInput().Title("Enter model name:").Value(&model).Run()
		return model, nil
	}

	if len(models) == 0 {
		// No models found — fall back to text input
		var model string
		huh.NewInput().Title("Enter model name:").Value(&model).Run()
		return model, nil
	}

	// Build options: model list + "Other (custom)"
	options := make([]huh.Option[string], 0, len(models)+1)
	for _, m := range models {
		options = append(options, huh.NewOption(m, m))
	}
	options = append(options, huh.NewOption("Other (custom)", "__other__"))

	var selected string
	huh.NewSelect[string]().
		Title("Which model?").
		Options(options...).
		Value(&selected).
		Run()

	if selected == "__other__" {
		var model string
		huh.NewInput().Title("Enter model name:").Value(&model).Run()
		return model, nil
	}

	return selected, nil
}

func selectOllamaModel() (string, error) {
	var models []string
	var fetchErr error

	err := spinner.New().
		Title("Checking Ollama for running models...").
		Action(func() {
			models, fetchErr = ai.FetchOllamaModels()
		}).
		Run()
	if err != nil {
		return "", err
	}
	if fetchErr != nil {
		ui.Error(fetchErr.Error())
		return "", fetchErr
	}

	options := make([]huh.Option[string], len(models))
	for i, m := range models {
		options[i] = huh.NewOption(m, m)
	}

	var selected string
	huh.NewSelect[string]().
		Title("Which running model?").
		Options(options...).
		Value(&selected).
		Run()

	return selected, nil
}
