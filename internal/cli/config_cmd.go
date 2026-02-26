package cli

import (
	"context"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "View or edit configuration",
		RunE:  runConfig,
	}
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Show current config
	fmt.Println()
	ui.KeyValue("Gmail", cfg.Gmail.Email)
	ui.KeyValue("Provider", cfg.AI.Provider)
	ui.KeyValue("Model", cfg.AI.Model)
	ui.KeyValue("Base URL", cfg.AI.BaseURL)
	ui.KeyValue("Poll", fmt.Sprintf("%ds", cfg.PollInterval))

	fmt.Println()
	fmt.Printf("  %s\n", ui.BoldStr("Labels"))
	for _, l := range cfg.Labels {
		fmt.Printf("    %s: %s\n", l.Name, l.Description)
	}

	// Ask what to edit
	var editChoice string
	huh.NewSelect[string]().
		Title("What would you like to change?").
		Options(
			huh.NewOption("Nothing (exit)", "none"),
			huh.NewOption("Gmail account (re-authenticate)", "gmail"),
			huh.NewOption("AI provider / model", "ai"),
			huh.NewOption("Labels", "labels"),
			huh.NewOption("Poll interval", "poll"),
		).
		Value(&editChoice).
		Run()

	switch editChoice {
	case "none":
		return nil

	case "gmail":
		ui.Dim("A browser window will open for you to sign in.")
		fmt.Println()

		_, err := gmailpkg.Authenticate(config.CredentialsPath())
		if err != nil {
			return fmt.Errorf("Gmail authentication failed: %w", err)
		}

		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return fmt.Errorf("creating token source: %w", err)
		}
		client, err := gmailpkg.NewClient(context.Background(), ts)
		if err != nil {
			return fmt.Errorf("creating Gmail client: %w", err)
		}
		email, _, err := client.GetProfile(context.Background())
		if err != nil {
			return fmt.Errorf("getting profile: %w", err)
		}

		cfg.Gmail.Email = email
		ui.Success(fmt.Sprintf("Connected as %s", email))

	case "ai":
		providerNames := ai.ProviderNames()
		var selectedProvider string
		huh.NewSelect[string]().
			Title("Which AI provider?").
			Options(huh.NewOptions(providerNames...)...).
			Value(&selectedProvider).
			Run()

		provider, _ := ai.GetProvider(selectedProvider)
		cfg.AI.Provider = selectedProvider
		cfg.AI.BaseURL = provider.BaseURL

		// Dynamic model selection
		model, err := selectModel(selectedProvider)
		if err != nil {
			ui.Error(err.Error())
			return nil
		}
		cfg.AI.Model = model

		if provider.EnvKey != "" {
			var apiKey string
			huh.NewInput().Title("API key:").Value(&apiKey).EchoMode(huh.EchoModePassword).Run()
			if apiKey != "" {
				cfg.AI.APIKey = apiKey
			}
		}

	case "labels":
		options := make([]huh.Option[string], len(cfg.Labels))
		for i, l := range cfg.Labels {
			options[i] = huh.NewOption(fmt.Sprintf("%s — %s", l.Name, l.Description), l.Name).Selected(true)
		}

		selectedNames := make([]string, len(cfg.Labels))
		for i, l := range cfg.Labels {
			selectedNames[i] = l.Name
		}

		huh.NewMultiSelect[string]().
			Title("Keep which labels? (deselect to remove)").
			Options(options...).
			Value(&selectedNames).
			Run()

		selectedSet := make(map[string]bool)
		for _, n := range selectedNames {
			selectedSet[n] = true
		}
		var kept []config.Label
		for _, l := range cfg.Labels {
			if selectedSet[l.Name] {
				kept = append(kept, l)
			}
		}
		cfg.Labels = kept

		for {
			var addMore bool
			huh.NewConfirm().Title("Add a custom label?").Value(&addMore).Run()
			if !addMore {
				break
			}
			var name, description string
			huh.NewInput().Title("Label name:").Value(&name).Run()
			huh.NewInput().Title("Description:").Value(&description).Run()
			if name != "" {
				cfg.Labels = append(cfg.Labels, config.Label{Name: name, Description: description})
				ui.Success(fmt.Sprintf("Added: %s", name))
			}
		}

		ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
		if tsErr == nil {
			client, clientErr := gmailpkg.NewClient(context.Background(), ts)
			if clientErr == nil {
				store, dbErr := db.Open(config.DBPath())
				if dbErr == nil {
					defer store.Close()
					for _, l := range cfg.Labels {
						if _, err := store.GetLabelMapping(l.Name); err != nil {
							gmailID, createErr := client.CreateLabel(context.Background(), l.Name)
							if createErr == nil {
								store.SetLabelMapping(l.Name, gmailID)
								ui.Success(fmt.Sprintf("Created in Gmail: %s", l.Name))
							}
						}
					}
				}
			}
		}

	case "poll":
		var intervalStr string
		huh.NewInput().Title("Poll interval (seconds):").Value(&intervalStr).Run()
		var interval int
		fmt.Sscanf(intervalStr, "%d", &interval)
		if interval > 0 {
			cfg.PollInterval = interval
		}
	}

	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	ui.Success("Config saved")
	return nil
}
