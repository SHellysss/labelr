package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/spf13/cobra"
)

func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive first-time setup",
		Long:  "Set up Gmail authentication, AI provider, and label configuration.",
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Welcome to labelr! Let's get you set up.")
	fmt.Println()

	// Ensure config directory exists
	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Step 1: Gmail OAuth
	fmt.Println("Step 1: Connect your Gmail account")
	fmt.Println("A browser window will open for you to sign in with Google.")
	fmt.Println()

	var proceed bool
	huh.NewConfirm().
		Title("Ready to authenticate with Gmail?").
		Value(&proceed).
		Run()

	if !proceed {
		return fmt.Errorf("setup cancelled")
	}

	token, err := gmailpkg.Authenticate(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("Gmail authentication failed: %w", err)
	}
	_ = token

	// Get user email
	ts, err := gmailpkg.TokenSource(config.CredentialsPath())
	if err != nil {
		return fmt.Errorf("creating token source: %w", err)
	}
	client, err := gmailpkg.NewClient(context.Background(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}
	email, historyID, err := client.GetProfile(context.Background())
	if err != nil {
		return fmt.Errorf("getting profile: %w", err)
	}
	fmt.Printf("Connected as: %s\n\n", email)

	// Step 2: AI Provider
	fmt.Println("Step 2: Choose your AI provider")

	providerNames := ai.ProviderNames()
	var selectedProvider string
	huh.NewSelect[string]().
		Title("Which AI provider?").
		Options(huh.NewOptions(providerNames...)...).
		Value(&selectedProvider).
		Run()

	provider, _ := ai.GetProvider(selectedProvider)

	// Step 3: Model
	var model string
	huh.NewInput().
		Title("Which model? (e.g., gpt-4o-mini, llama3, deepseek-chat)").
		Value(&model).
		Run()

	// Step 4: API Key
	var apiKey string
	if provider.EnvKey != "" {
		// Check env first
		if envVal := os.Getenv(provider.EnvKey); envVal != "" {
			fmt.Printf("Found API key in $%s\n", provider.EnvKey)
			apiKey = envVal
		} else {
			huh.NewInput().
				Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
				Value(&apiKey).
				EchoMode(huh.EchoModePassword).
				Run()
		}
	}

	// Step 5: Labels — checklist of defaults + add custom
	fmt.Println("\nStep 3: Configure labels")
	defaults := config.DefaultLabels()

	options := make([]huh.Option[string], len(defaults))
	for i, l := range defaults {
		options[i] = huh.NewOption(fmt.Sprintf("%s — %s", l.Name, l.Description), l.Name).Selected(true)
	}

	selectedNames := make([]string, len(defaults))
	for i, l := range defaults {
		selectedNames[i] = l.Name
	}

	huh.NewMultiSelect[string]().
		Title("Which default labels do you want?").
		Options(options...).
		Value(&selectedNames).
		Run()

	// Build label list from selected defaults
	selectedSet := make(map[string]bool)
	for _, n := range selectedNames {
		selectedSet[n] = true
	}
	var labels []config.Label
	for _, l := range defaults {
		if selectedSet[l.Name] {
			labels = append(labels, l)
		}
	}

	// Add custom labels loop
	for {
		var addMore bool
		huh.NewConfirm().
			Title("Add a custom label?").
			Value(&addMore).
			Run()

		if !addMore {
			break
		}

		var name, description string
		huh.NewInput().Title("Label name:").Value(&name).Run()
		huh.NewInput().Title("Description (helps AI classify):").Value(&description).Run()

		if name != "" {
			labels = append(labels, config.Label{Name: name, Description: description})
			fmt.Printf("  Added: %s\n", name)
		}
	}

	// Save config
	cfg := &config.Config{
		Gmail:        config.GmailConfig{Email: email},
		AI:           config.AIConfig{Provider: selectedProvider, Model: model, APIKey: apiKey, BaseURL: provider.BaseURL},
		Labels:       labels,
		PollInterval: 60,
	}
	if err := config.Save(config.DefaultPath(), cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	// Create labels in Gmail
	fmt.Println("\nCreating Gmail labels...")
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	for _, l := range labels {
		gmailID, err := client.CreateLabel(context.Background(), l.Name)
		if err != nil {
			fmt.Printf("  Warning: could not create label %q: %v\n", l.Name, err)
			continue
		}
		store.SetLabelMapping(l.Name, gmailID)
		fmt.Printf("  Created: %s\n", l.Name)
	}

	// Store initial historyId
	store.SetState("history_id", fmt.Sprintf("%d", historyID))

	// Offer test run
	var testRun bool
	huh.NewConfirm().
		Title("Label your 10 most recent emails to test?").
		Value(&testRun).
		Run()

	if testRun {
		fmt.Println("Fetching recent emails...")
		msgs, err := client.ListRecentMessages(context.Background(), 10)
		if err != nil {
			fmt.Printf("Warning: could not fetch recent messages: %v\n", err)
		} else {
			for _, m := range msgs {
				store.InsertMessage(m.ID, m.ThreadID)
			}
			fmt.Printf("Added %d emails to queue. They'll be labeled when you run 'labelr start'.\n", len(msgs))
		}
	}

	// Restart daemon if it's currently running so it picks up new credentials
	mgr := service.Detect()
	if mgr != nil {
		if running, _ := mgr.IsRunning(); running {
			fmt.Println("\nRestarting daemon with new credentials...")
			mgr.Stop()
			mgr.Start()
			fmt.Println("Daemon restarted.")
		} else {
			fmt.Println("\nSetup complete! Run 'labelr start' to begin labeling emails.")
		}
	} else {
		fmt.Println("\nSetup complete! Run 'labelr start' to begin labeling emails.")
	}

	return nil
}
