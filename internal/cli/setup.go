package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"

	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/tui"
	tuisetup "github.com/pankajbeniwal/labelr/internal/tui/setup"
	"github.com/pankajbeniwal/labelr/internal/ui"
	"github.com/spf13/cobra"
)

func NewSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Set up or reconfigure labelr",
		Long:  "First-time setup wizard or reconfigure existing settings: Gmail auth, AI provider, labels.",
		RunE:  runSetup,
	}
}

func runSetup(cmd *cobra.Command, args []string) error {
	if err := os.MkdirAll(config.Dir(), 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Detect mode: first-run vs reconfigure
	existingCfg, cfgErr := config.Load(config.DefaultPath())
	if cfgErr == nil && existingCfg.AI.Provider != "" {
		fmt.Println()
		return runReconfigure(existingCfg)
	}

	// First-time setup — launch TUI wizard
	wizard, err := tuisetup.NewWizard()
	if err != nil {
		return err
	}
	return tui.Run(wizard)
}

// --- Shared helpers (used by reconfigure flow) ---

func setupAI(existingCfg *config.Config) (string, string, string, error) {
	for {
		ui.Header("Choose your AI provider")

		providerNames := ai.ProviderNamesOrdered()
		var selectedProvider string
		huh.NewSelect[string]().
			Title("Which AI provider?").
			Options(huh.NewOptions(providerNames...)...).
			Value(&selectedProvider).
			Run()

		provider, _ := ai.GetProvider(selectedProvider)

		// Model selection
		model, err := selectModel(selectedProvider)
		if err != nil {
			return "", "", "", err
		}

		// API Key
		apiKey := ""
		if provider.EnvKey != "" {
			if envVal := os.Getenv(provider.EnvKey); envVal != "" {
				ui.Info(fmt.Sprintf("Found API key in $%s", provider.EnvKey))
				apiKey = envVal
			} else if existingCfg != nil && existingCfg.AI.Provider == selectedProvider && existingCfg.AI.APIKey != "" {
				// Same provider, offer to reuse
				var reuseKey bool
				huh.NewConfirm().
					Title("Use existing API key?").
					Value(&reuseKey).
					Run()
				if reuseKey {
					apiKey = existingCfg.AI.APIKey
				} else {
					huh.NewInput().
						Title(fmt.Sprintf("Enter your %s API key:", provider.Name)).
						Value(&apiKey).
						EchoMode(huh.EchoModePassword).
						Run()
				}
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
			return "", "", "", spinErr
		}

		if validateErr == nil {
			ui.Success(fmt.Sprintf("Connected to %s / %s", selectedProvider, model))
			return selectedProvider, model, apiKey, nil
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
			return "", "", "", fmt.Errorf("setup cancelled")
		}
	}
}

func setupModelOnly(cfg *config.Config) (string, error) {
	model, err := selectModel(cfg.AI.Provider)
	if err != nil {
		return "", err
	}

	// Validate with existing API key
	provider, _ := ai.GetProvider(cfg.AI.Provider)
	apiKey := cfg.ResolveAPIKey()
	classifier := ai.NewClassifier(apiKey, provider.BaseURL, model, cfg.Labels)

	var validateErr error
	spinErr := spinner.New().
		Title("Verifying connection...").
		Action(func() {
			validateErr = classifier.ValidateConnection(context.Background())
		}).
		Run()
	if spinErr != nil {
		return "", spinErr
	}

	if validateErr != nil {
		ui.Error(fmt.Sprintf("Could not connect to %s / %s", cfg.AI.Provider, model))
		ui.Dim("This could mean: model doesn't support structured output or network issue")
		return "", fmt.Errorf("validation failed")
	}

	ui.Success(fmt.Sprintf("Connected to %s / %s", cfg.AI.Provider, model))
	return model, nil
}

func setupLabels(existingLabels []config.Label) ([]config.Label, error) {
	ui.Header("Configure labels")

	var sourceLabels []config.Label
	if existingLabels != nil {
		sourceLabels = existingLabels
	} else {
		sourceLabels = config.DefaultLabels()
	}

	options := make([]huh.Option[string], len(sourceLabels))
	selectedNames := make([]string, len(sourceLabels))
	for i, l := range sourceLabels {
		options[i] = huh.NewOption(fmt.Sprintf("%s — %s", l.Name, l.Description), l.Name).Selected(true)
		selectedNames[i] = l.Name
	}

	title := "Which default labels do you want?"
	if existingLabels != nil {
		title = "Keep which labels? (deselect to remove)"
	}

	huh.NewMultiSelect[string]().
		Title(title).
		Options(options...).
		Value(&selectedNames).
		Run()

	selectedSet := make(map[string]bool)
	for _, n := range selectedNames {
		selectedSet[n] = true
	}
	var labels []config.Label
	for _, l := range sourceLabels {
		if selectedSet[l.Name] {
			labels = append(labels, l)
		}
	}

	// Custom labels
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

	return labels, nil
}

func createLabelsInGmail(client *gmailpkg.Client, store *db.Store, labels []config.Label) {
	var labelErr error
	spinErr := spinner.New().
		Title("Creating Gmail labels...").
		Action(func() {
			customIdx := 0
			for _, l := range labels {
				// Check if label already has a color stored in DB
				_, existingBg, existingTx, dbErr := store.GetLabelMappingWithColor(l.Name)
				var bg, tx string
				if dbErr == nil && existingBg != "" {
					bg, tx = existingBg, existingTx
				} else {
					bg, tx = gmailpkg.ColorForLabel(l.Name, customIdx)
				}

				if !gmailpkg.IsDefaultLabel(l.Name) {
					customIdx++
				}

				gmailID, err := client.CreateLabel(context.Background(), l.Name, bg, tx)
				if err != nil {
					labelErr = err
					continue
				}
				store.SetLabelMappingWithColor(l.Name, gmailID, bg, tx)
			}
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error creating labels: %v", spinErr))
		return
	}

	if labelErr != nil {
		ui.Error(fmt.Sprintf("Some labels could not be created: %v", labelErr))
	}
	for _, l := range labels {
		ui.Success(l.Name)
	}
}

func startDaemon() {
	mgr := service.Detect()
	if mgr == nil {
		ui.Dim("Could not detect service manager — run 'labelr daemon' manually")
		return
	}

	if running, _ := mgr.IsRunning(); running {
		restartDaemon(mgr)
		return
	}

	spinErr := spinner.New().
		Title("Starting labelr...").
		Action(func() {
			mgr.Install(findBinary())
			mgr.Start()
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error starting daemon: %v", spinErr))
		return
	}
	ui.Success("labelr is running in the background")
	ui.Dim("Use 'labelr status' to check on it")
	fmt.Println()
}

func restartDaemon(mgr service.Manager) {
	spinErr := spinner.New().
		Title("Restarting daemon with new config...").
		Action(func() {
			mgr.Stop()
			mgr.Install(findBinary())
			mgr.Start()
		}).
		Run()
	if spinErr != nil {
		ui.Error(fmt.Sprintf("Error restarting daemon: %v", spinErr))
		return
	}
	ui.Success("Daemon restarted")
}

func restartDaemonIfRunning() {
	mgr := service.Detect()
	if mgr == nil {
		return
	}
	if running, _ := mgr.IsRunning(); running {
		restartDaemon(mgr)
	}
}

// findBinary returns the path to the labelr binary.
func findBinary() string {
	if path, err := os.Executable(); err == nil {
		return path
	}
	return "labelr"
}

// --- Reconfigure menu ---

func runReconfigure(cfg *config.Config) error {
	for {
		// Show TUI menu with config summary
		view := tuisetup.NewReconfigureView(cfg)
		if err := tui.Run(view); err != nil {
			return err
		}

		choice := view.Choice()
		if choice == "none" || choice == "" {
			return nil
		}

		// Execute sub-flow outside TUI (these use blocking huh prompts)
		switch choice {
		case "gmail":
			ui.Dim("Opening browser to sign in...")
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
			email, historyID, err := client.GetProfile(context.Background())
			if err != nil {
				return fmt.Errorf("getting profile: %w", err)
			}
			cfg.Gmail.Email = email
			ui.Success(fmt.Sprintf("Connected as %s", email))

			// Create labels in the new account and reset history ID
			store, dbErr := db.Open(config.DBPath())
			if dbErr == nil {
				createLabelsInGmail(client, store, cfg.Labels)
				store.SetState("history_id", fmt.Sprintf("%d", historyID))
				store.Close()
			}

		case "ai":
			provider, model, apiKey, err := setupAI(cfg)
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			providerInfo, _ := ai.GetProvider(provider)
			cfg.AI.Provider = provider
			cfg.AI.Model = model
			cfg.AI.APIKey = apiKey
			cfg.AI.BaseURL = providerInfo.BaseURL

		case "model":
			model, err := setupModelOnly(cfg)
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			cfg.AI.Model = model

		case "labels":
			newLabels, err := setupLabels(cfg.Labels)
			if err != nil {
				return err
			}

			// Find removed labels and delete from Gmail
			removedLabels := findRemovedLabels(cfg.Labels, newLabels)
			if len(removedLabels) > 0 {
				removeLabelsFromGmail(removedLabels)
			}

			// Find new labels and create in Gmail
			addedLabels := findAddedLabels(cfg.Labels, newLabels)
			if len(addedLabels) > 0 {
				ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
				if tsErr == nil {
					client, clientErr := gmailpkg.NewClient(context.Background(), ts)
					if clientErr == nil {
						store, dbErr := db.Open(config.DBPath())
						if dbErr == nil {
							createLabelsInGmail(client, store, addedLabels)
							store.Close()
						}
					}
				}
			}

			cfg.Labels = newLabels

		case "poll":
			interval, err := promptPollInterval()
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			cfg.PollInterval = interval
		}

		// Save after each change
		if err := config.Save(config.DefaultPath(), cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		ui.Success("Config saved")

		// Restart daemon if running
		restartDaemonIfRunning()

		fmt.Println()
	}
}

// --- Label diff helpers ---

func findRemovedLabels(old, new []config.Label) []config.Label {
	newSet := make(map[string]bool)
	for _, l := range new {
		newSet[l.Name] = true
	}
	var removed []config.Label
	for _, l := range old {
		if !newSet[l.Name] {
			removed = append(removed, l)
		}
	}
	return removed
}

func findAddedLabels(old, new []config.Label) []config.Label {
	oldSet := make(map[string]bool)
	for _, l := range old {
		oldSet[l.Name] = true
	}
	var added []config.Label
	for _, l := range new {
		if !oldSet[l.Name] {
			added = append(added, l)
		}
	}
	return added
}

func removeLabelsFromGmail(labels []config.Label) {
	ts, tsErr := gmailpkg.TokenSource(config.CredentialsPath())
	if tsErr != nil {
		return
	}
	client, clientErr := gmailpkg.NewClient(context.Background(), ts)
	if clientErr != nil {
		return
	}
	store, dbErr := db.Open(config.DBPath())
	if dbErr != nil {
		return
	}
	defer store.Close()

	for _, l := range labels {
		gmailID, err := store.GetLabelMapping(l.Name)
		if err != nil {
			continue
		}
		if err := client.DeleteLabel(context.Background(), gmailID); err != nil {
			ui.Error(fmt.Sprintf("Could not delete label %q from Gmail: %v", l.Name, err))
			continue
		}
		store.DeleteLabelMapping(l.Name)
		ui.Info(fmt.Sprintf("Removed from Gmail: %s", l.Name))
	}
}

func promptPollInterval() (int, error) {
	for {
		var intervalStr string
		huh.NewInput().
			Title("Poll interval (seconds):").
			Value(&intervalStr).
			Run()

		interval, err := strconv.Atoi(intervalStr)
		if err != nil || interval <= 0 {
			ui.Error("Please enter a positive number")
			continue
		}
		return interval, nil
	}
}
