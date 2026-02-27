package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"golang.org/x/oauth2"

	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Set up and start labelr",
		Long:  "Interactive first-time setup: Gmail auth, AI provider, labels, then starts the daemon.",
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println()
	ui.Bold("Welcome to labelr!")

	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// --- Gmail OAuth ---
	var ts oauth2.TokenSource

	// Check if valid credentials already exist
	existingTS, loadErr := gmailpkg.TokenSource(config.CredentialsPath())
	if loadErr == nil {
		// Try to use existing credentials
		if _, tokErr := existingTS.Token(); tokErr == nil {
			ts = existingTS
			ui.Success("Gmail already connected")
		}
	}

	if ts == nil {
		ui.Header("Connect your Gmail account")
		ui.Dim("A browser window will open for you to sign in.")
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

		ts, err = gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return fmt.Errorf("creating token source: %w", err)
		}
	}
	client, err := gmailpkg.NewClient(context.Background(), ts)
	if err != nil {
		return fmt.Errorf("creating Gmail client: %w", err)
	}
	email, historyID, err := client.GetProfile(context.Background())
	if err != nil {
		return fmt.Errorf("getting profile: %w", err)
	}
	ui.Success(fmt.Sprintf("Connected as %s", email))

	// --- AI Provider ---
	var selectedProvider string
	var model string
	var apiKey string

	for {
		ui.Header("Choose your AI provider")

		providerNames := ai.ProviderNames()
		huh.NewSelect[string]().
			Title("Which AI provider?").
			Options(huh.NewOptions(providerNames...)...).
			Value(&selectedProvider).
			Run()

		provider, _ := ai.GetProvider(selectedProvider)

		// Model selection
		var err error
		model, err = selectModel(selectedProvider)
		if err != nil {
			return err
		}

		// API Key
		apiKey = ""
		if provider.EnvKey != "" {
			if envVal := os.Getenv(provider.EnvKey); envVal != "" {
				ui.Info(fmt.Sprintf("Found API key in $%s", provider.EnvKey))
				apiKey = envVal
			} else {
				huh.NewInput().
					Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
					Value(&apiKey).
					EchoMode(huh.EchoModePassword).
					Run()
			}
		}

		// Validate connection
		classifier := ai.NewClassifier(apiKey, provider.BaseURL, model, config.DefaultLabels())

		var validateErr error
		spinErr := spinner.New().
			Title("Verifying connection...").
			Action(func() {
				validateErr = classifier.ValidateConnection(context.Background())
			}).
			Run()
		if spinErr != nil {
			return spinErr
		}

		if validateErr == nil {
			ui.Success(fmt.Sprintf("Connected to %s / %s", selectedProvider, model))
			break
		}

		ui.Error(fmt.Sprintf("Could not connect to %s / %s", selectedProvider, model))
		ui.Dim("This could mean: invalid API key, model doesn't support structured output, or network issue")
		fmt.Println()

		var retry bool
		huh.NewConfirm().
			Title("Try again with different settings?").
			Value(&retry).
			Run()

		if !retry {
			return fmt.Errorf("setup cancelled")
		}
	}

	// --- Labels ---
	ui.Header("Configure labels")

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
			ui.Success(fmt.Sprintf("Added: %s", name))
		}
	}

	// Save config
	provider, _ := ai.GetProvider(selectedProvider)
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
	store, err := db.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	var labelErr error
	spinErr := spinner.New().
		Title("Creating Gmail labels...").
		Action(func() {
			customIdx := 0
			for _, l := range labels {
				bg, tx := gmailpkg.ColorForLabel(l.Name, customIdx)
				if !gmailpkg.IsDefaultLabel(l.Name) {
					customIdx++
				}
				gmailID, err := client.CreateLabel(context.Background(), l.Name, bg, tx)
				if err != nil {
					labelErr = err
					continue
				}
				store.SetLabelMapping(l.Name, gmailID)
			}
		}).
		Run()
	if spinErr != nil {
		return spinErr
	}

	if labelErr != nil {
		ui.Error(fmt.Sprintf("Some labels could not be created: %v", labelErr))
	}
	for _, l := range labels {
		ui.Success(l.Name)
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
		var msgs []struct{ ID, ThreadID string }
		var fetchErr error
		spinErr := spinner.New().
			Title("Fetching recent emails...").
			Action(func() {
				msgs, fetchErr = client.ListRecentMessages(context.Background(), 10)
			}).
			Run()
		if spinErr != nil {
			return spinErr
		}
		if fetchErr != nil {
			ui.Error(fmt.Sprintf("Could not fetch recent messages: %v", fetchErr))
		} else {
			for _, m := range msgs {
				store.InsertMessage(m.ID, m.ThreadID)
			}
			ui.Success(fmt.Sprintf("Queued %d emails for labeling", len(msgs)))
		}
	}

	// Auto-start daemon
	mgr := service.Detect()
	if mgr == nil {
		ui.Dim("Could not detect service manager — run 'labelr daemon' manually")
		return nil
	}

	// Check if already running (re-init case)
	if running, _ := mgr.IsRunning(); running {
		spinErr := spinner.New().
			Title("Restarting daemon with new config...").
			Action(func() {
				mgr.Stop()
				mgr.Install(findBinary())
				mgr.Start()
			}).
			Run()
		if spinErr != nil {
			return spinErr
		}
		ui.Success("labelr daemon restarted")
	} else {
		spinErr := spinner.New().
			Title("Starting labelr...").
			Action(func() {
				mgr.Install(findBinary())
				mgr.Start()
			}).
			Run()
		if spinErr != nil {
			return spinErr
		}
		ui.Success("labelr is running in the background")
	}

	ui.Dim("Use 'labelr status' to check on it")
	fmt.Println()

	return nil
}

// findBinary returns the path to the labelr binary.
func findBinary() string {
	if path, err := os.Executable(); err == nil {
		return path
	}
	return "labelr"
}
