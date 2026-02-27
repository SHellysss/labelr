package setup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	"github.com/pankajbeniwal/labelr/internal/db"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/service"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// reconfigurePhase represents where we are in the reconfigure flow.
type reconfigurePhase int

const (
	phaseMenu     reconfigurePhase = iota
	phaseSubflow                   // running a sub-flow (gmail, ai, model, labels, poll)
	phaseSaving                    // saving config + restarting daemon
)

// --- Messages ---

type gmailReauthDoneMsg struct {
	email     string
	historyID uint64
	err       error
}

type labelSyncDoneMsg struct{ err error }

type configSavedMsg struct{ err error }

type daemonRestartedMsg struct{ err error }

// --- Sub-flow types ---

type subflowKind int

const (
	subflowGmail  subflowKind = iota
	subflowAI                 // full provider + model + key
	subflowModel              // just model change
	subflowLabels
	subflowPoll
)

// ReconfigureView is a single TUI view for the entire reconfigure experience.
// It never exits the shell — all sub-flows run inline as huh forms or spinners.
type ReconfigureView struct {
	cfg   *config.Config
	store *db.Store

	phase   reconfigurePhase
	subflow subflowKind

	// Menu
	menuForm   *huh.Form
	menuChoice string

	// Sub-flow: Gmail
	gmailSpinner SpinnerStep

	// Sub-flow: AI (reuses aiStep-like state machine)
	aiPhase    int // 0=provider, 1=fetch-models, 2=model-select, 3=apikey, 4=validate
	aiForm     *huh.Form
	aiSpinner  SpinnerStep
	aiProvider string
	aiModel    string
	aiAPIKey   string

	// Sub-flow: Model only
	modelPhase int // 0=fetch, 1=select, 2=validate
	modelForm  *huh.Form
	modelSpin  SpinnerStep
	modelVal   string

	// Sub-flow: Labels
	labelPhase   int // 0=sub-menu, 1=name, 2=desc, 3=remove-select
	labelForm    *huh.Form
	labelAction  string // "add", "remove", "back"
	labelName    string
	labelDesc    string
	labelDupErr  string
	labelRemSel  []string // selected labels to remove

	// Sub-flow: Poll
	pollForm *huh.Form
	pollStr  string
	pollErr  string

	// Saving phase
	saveSpinner SpinnerStep
	saveStatus  string // accumulated status lines

	width  int
	height int
}

func NewReconfigureView(cfg *config.Config) *ReconfigureView {
	v := &ReconfigureView{
		cfg:   cfg,
		phase: phaseMenu,
	}
	v.buildMenu()
	return v
}

func (v *ReconfigureView) Title() string { return "setup" }

func (v *ReconfigureView) HelpKeys() []key.Binding {
	switch v.phase {
	case phaseMenu:
		return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter, tui.KeyQuit}
	case phaseSubflow:
		keys := []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter}
		keys = append(keys, tui.KeyBack)
		return keys
	default:
		return []key.Binding{tui.KeyQuit}
	}
}

func (v *ReconfigureView) Init() tea.Cmd {
	return v.menuForm.Init()
}

func (v *ReconfigureView) buildMenu() {
	v.menuChoice = ""
	v.menuForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to change?").
				Options(
					huh.NewOption("Nothing, exit", "none"),
					huh.NewOption("Gmail account", "gmail"),
					huh.NewOption("AI provider / model", "ai"),
					huh.NewOption("Just the model", "model"),
					huh.NewOption("Labels", "labels"),
					huh.NewOption("Poll interval", "poll"),
				).
				Value(&v.menuChoice),
		),
	).WithShowHelp(true)
}

// Choice returns the selected menu option (for backwards compat, unused now).
func (v *ReconfigureView) Choice() string {
	return v.menuChoice
}

// ─── Update ──────────────────────────────────────

func (v *ReconfigureView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, tui.KeyQuit) && v.phase != phaseSubflow {
			return v, tea.Quit
		}
		// Back from sub-flow to menu
		if key.Matches(msg, tui.KeyBack) && v.phase == phaseSubflow {
			v.phase = phaseMenu
			v.buildMenu()
			return v, v.menuForm.Init()
		}
	}

	switch v.phase {
	case phaseMenu:
		return v.updateMenu(msg)
	case phaseSubflow:
		return v.updateSubflow(msg)
	case phaseSaving:
		return v.updateSaving(msg)
	}
	return v, nil
}

func (v *ReconfigureView) updateMenu(msg tea.Msg) (tui.View, tea.Cmd) {
	form, cmd := v.menuForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		v.menuForm = f
	}
	if v.menuForm.State == huh.StateCompleted {
		switch v.menuChoice {
		case "none", "":
			return v, tea.Quit
		case "gmail":
			return v, v.startGmailSubflow()
		case "ai":
			return v, v.startAISubflow()
		case "model":
			return v, v.startModelSubflow()
		case "labels":
			return v, v.startLabelsSubflow()
		case "poll":
			return v, v.startPollSubflow()
		}
	}
	return v, cmd
}

// ─── Sub-flow: Gmail ─────────────────────────────

func (v *ReconfigureView) startGmailSubflow() tea.Cmd {
	v.phase = phaseSubflow
	v.subflow = subflowGmail
	v.gmailSpinner = newSpinnerStep("Connecting to Gmail...")
	return tea.Batch(v.gmailSpinner.spinner.Tick, v.doGmailReauth())
}

func (v *ReconfigureView) doGmailReauth() tea.Cmd {
	return func() tea.Msg {
		_, err := gmailpkg.Authenticate(config.CredentialsPath())
		if err != nil {
			return gmailReauthDoneMsg{err: fmt.Errorf("authentication failed: %w", err)}
		}
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return gmailReauthDoneMsg{err: fmt.Errorf("token source: %w", err)}
		}
		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return gmailReauthDoneMsg{err: fmt.Errorf("client: %w", err)}
		}
		email, historyID, err := client.GetProfile(ctx)
		if err != nil {
			return gmailReauthDoneMsg{err: fmt.Errorf("profile: %w", err)}
		}
		return gmailReauthDoneMsg{email: email, historyID: historyID}
	}
}

// ─── Sub-flow: AI (provider + model + key) ───────

func (v *ReconfigureView) startAISubflow() tea.Cmd {
	v.phase = phaseSubflow
	v.subflow = subflowAI
	v.aiPhase = 0
	v.aiProvider = ""
	v.aiModel = ""
	v.aiAPIKey = ""

	providerNames := ai.ProviderNamesOrdered()
	options := make([]huh.Option[string], len(providerNames))
	for i, name := range providerNames {
		options[i] = huh.NewOption(name, name)
	}
	v.aiForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose your AI provider").
				Options(options...).
				Value(&v.aiProvider),
		),
	).WithShowHelp(true)
	return v.aiForm.Init()
}

// ─── Sub-flow: Model only ────────────────────────

func (v *ReconfigureView) startModelSubflow() tea.Cmd {
	v.phase = phaseSubflow
	v.subflow = subflowModel
	v.modelPhase = 0
	v.modelVal = ""
	v.modelSpin = newSpinnerStep("Fetching available models...")
	return tea.Batch(v.modelSpin.spinner.Tick, v.fetchModelsForCurrent())
}

func (v *ReconfigureView) fetchModelsForCurrent() tea.Cmd {
	provider := v.cfg.AI.Provider
	return func() tea.Msg {
		if provider == "ollama" {
			models, err := ai.FetchOllamaModels()
			return modelsFetchedMsg{models: models, err: err}
		}
		models, err := ai.FetchModelsForProvider(provider)
		return modelsFetchedMsg{models: models, err: err}
	}
}

// ─── Sub-flow: Labels ────────────────────────────

func (v *ReconfigureView) startLabelsSubflow() tea.Cmd {
	v.phase = phaseSubflow
	v.subflow = subflowLabels
	v.labelDupErr = ""
	return v.showLabelMenu()
}

func (v *ReconfigureView) showLabelMenu() tea.Cmd {
	v.labelPhase = 0
	v.labelAction = ""
	v.labelForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Labels").
				Options(
					huh.NewOption("Add a new label", "add"),
					huh.NewOption("Remove labels", "remove"),
					huh.NewOption("Back to menu", "back"),
				).
				Value(&v.labelAction),
		),
	).WithShowHelp(true)
	return v.labelForm.Init()
}

// ─── Sub-flow: Poll ──────────────────────────────

func (v *ReconfigureView) startPollSubflow() tea.Cmd {
	v.phase = phaseSubflow
	v.subflow = subflowPoll
	v.pollStr = ""
	v.pollErr = ""
	v.pollForm = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Poll interval (seconds)").
				Value(&v.pollStr),
		),
	).WithShowHelp(true)
	return v.pollForm.Init()
}

// ─── Sub-flow dispatch ───────────────────────────

func (v *ReconfigureView) updateSubflow(msg tea.Msg) (tui.View, tea.Cmd) {
	switch v.subflow {
	case subflowGmail:
		return v.updateGmail(msg)
	case subflowAI:
		return v.updateAI(msg)
	case subflowModel:
		return v.updateModel(msg)
	case subflowLabels:
		return v.updateLabels(msg)
	case subflowPoll:
		return v.updatePoll(msg)
	}
	return v, nil
}

// ─── Gmail update ────────────────────────────────

func (v *ReconfigureView) updateGmail(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		v.gmailSpinner.spinner, cmd = v.gmailSpinner.spinner.Update(msg)
		return v, cmd
	case gmailReauthDoneMsg:
		if msg.err != nil {
			v.gmailSpinner.err = msg.err
			return v, nil
		}
		v.cfg.Gmail.Email = msg.email
		v.gmailSpinner.done = true
		// Sync labels to new account
		return v, v.startSave(fmt.Sprintf("Connected as %s", msg.email), func() tea.Msg {
			store, err := db.Open(config.DBPath())
			if err != nil {
				return labelSyncDoneMsg{err: err}
			}
			defer store.Close()
			v.store = store

			ts, err := gmailpkg.TokenSource(config.CredentialsPath())
			if err != nil {
				return labelSyncDoneMsg{err: err}
			}
			client, err := gmailpkg.NewClient(context.Background(), ts)
			if err != nil {
				return labelSyncDoneMsg{err: err}
			}
			customIdx := 0
			for _, l := range v.cfg.Labels {
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
					continue
				}
				store.SetLabelMappingWithColor(l.Name, gmailID, bg, tx)
			}
			store.SetState("history_id", fmt.Sprintf("%d", msg.historyID))
			return labelSyncDoneMsg{}
		})
	}
	return v, nil
}

// ─── AI update ───────────────────────────────────

func (v *ReconfigureView) updateAI(msg tea.Msg) (tui.View, tea.Cmd) {
	switch v.aiPhase {
	case 0: // Provider selection
		form, cmd := v.aiForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.aiForm = f
		}
		if v.aiForm.State == huh.StateCompleted {
			v.aiPhase = 1
			v.aiSpinner = newSpinnerStep("Fetching available models...")
			return v, tea.Batch(v.aiSpinner.spinner.Tick, v.fetchModelsForAI())
		}
		return v, cmd

	case 1: // Fetching models
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			v.aiSpinner.spinner, cmd = v.aiSpinner.spinner.Update(msg)
			return v, cmd
		case modelsFetchedMsg:
			v.aiPhase = 2
			if msg.err != nil || len(msg.models) == 0 {
				v.aiForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&v.aiModel),
					),
				).WithShowHelp(true)
				return v, v.aiForm.Init()
			}
			options := make([]huh.Option[string], 0, len(msg.models)+1)
			for _, m := range msg.models {
				options = append(options, huh.NewOption(m, m))
			}
			options = append(options, huh.NewOption("Other (custom)", "__other__"))
			v.aiForm = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Which model?").
						Options(options...).
						Value(&v.aiModel),
				),
			).WithShowHelp(true)
			return v, v.aiForm.Init()
		}
		return v, nil

	case 2: // Model selection
		form, cmd := v.aiForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.aiForm = f
		}
		if v.aiForm.State == huh.StateCompleted {
			if v.aiModel == "__other__" {
				v.aiModel = ""
				v.aiForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&v.aiModel),
					),
				).WithShowHelp(true)
				return v, v.aiForm.Init()
			}
			// Skip API key for ollama
			if v.aiProvider == "ollama" {
				return v, v.applyAIChanges()
			}
			// Check env var
			envKey := ai.EnvKeyForProvider(v.aiProvider)
			if envKey != "" {
				if val := os.Getenv(envKey); val != "" {
					v.aiAPIKey = val
					return v, v.applyAIChanges()
				}
			}
			// Reuse existing key if same provider
			if v.cfg.AI.Provider == v.aiProvider && v.cfg.AI.APIKey != "" {
				v.aiPhase = 3
				v.aiAPIKey = ""
				var reuseKey bool
				v.aiForm = huh.NewForm(
					huh.NewGroup(
						huh.NewConfirm().
							Title("Use existing API key?").
							Value(&reuseKey),
					),
				).WithShowHelp(true)
				// Store reuse decision via a wrapper
				return v, v.aiForm.Init()
			}
			v.aiPhase = 3
			v.aiForm = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title(fmt.Sprintf("Enter your API key")).
						EchoMode(huh.EchoModePassword).
						Value(&v.aiAPIKey),
				),
			).WithShowHelp(true)
			return v, v.aiForm.Init()
		}
		return v, cmd

	case 3: // API key / reuse confirm
		form, cmd := v.aiForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.aiForm = f
		}
		if v.aiForm.State == huh.StateCompleted {
			// If we offered reuse and user just confirmed without typing a key,
			// check the reuse path
			if v.aiAPIKey == "" && v.cfg.AI.Provider == v.aiProvider && v.cfg.AI.APIKey != "" {
				// The confirm form was used — check if they want to reuse
				// huh.Confirm defaults to true when submitted
				v.aiAPIKey = v.cfg.AI.APIKey
			}
			if v.aiAPIKey == "" {
				// They said no to reuse — show input
				v.aiForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter your API key").
							EchoMode(huh.EchoModePassword).
							Value(&v.aiAPIKey),
					),
				).WithShowHelp(true)
				return v, v.aiForm.Init()
			}
			return v, v.applyAIChanges()
		}
		return v, cmd

	case 4: // Validating
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			v.aiSpinner.spinner, cmd = v.aiSpinner.spinner.Update(msg)
			return v, cmd
		case validateDoneMsg:
			if msg.err != nil {
				v.aiSpinner.err = fmt.Errorf("connection failed — try different settings")
				// Return to menu after showing error briefly
				return v, nil
			}
			v.aiSpinner.done = true
			providerInfo, _ := ai.GetProvider(v.aiProvider)
			v.cfg.AI.Provider = v.aiProvider
			v.cfg.AI.Model = v.aiModel
			v.cfg.AI.APIKey = v.aiAPIKey
			v.cfg.AI.BaseURL = providerInfo.BaseURL
			return v, v.startSave(
				fmt.Sprintf("Connected to %s / %s", v.aiProvider, v.aiModel),
				nil,
			)
		}
		return v, nil
	}
	return v, nil
}

func (v *ReconfigureView) fetchModelsForAI() tea.Cmd {
	provider := v.aiProvider
	return func() tea.Msg {
		if provider == "ollama" {
			models, err := ai.FetchOllamaModels()
			return modelsFetchedMsg{models: models, err: err}
		}
		models, err := ai.FetchModelsForProvider(provider)
		return modelsFetchedMsg{models: models, err: err}
	}
}

func (v *ReconfigureView) applyAIChanges() tea.Cmd {
	v.aiPhase = 4
	v.aiSpinner = newSpinnerStep("Verifying connection...")
	return tea.Batch(v.aiSpinner.spinner.Tick, v.validateAI())
}

func (v *ReconfigureView) validateAI() tea.Cmd {
	provider := v.aiProvider
	model := v.aiModel
	apiKey := v.aiAPIKey
	labels := v.cfg.Labels
	return func() tea.Msg {
		p, _ := ai.GetProvider(provider)
		classifier := ai.NewClassifier(apiKey, p.BaseURL, model, labels)
		err := classifier.ValidateConnection(context.Background())
		return validateDoneMsg{err: err}
	}
}

// ─── Model update ────────────────────────────────

func (v *ReconfigureView) updateModel(msg tea.Msg) (tui.View, tea.Cmd) {
	switch v.modelPhase {
	case 0: // Fetching models
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			v.modelSpin.spinner, cmd = v.modelSpin.spinner.Update(msg)
			return v, cmd
		case modelsFetchedMsg:
			v.modelPhase = 1
			if msg.err != nil || len(msg.models) == 0 {
				v.modelForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&v.modelVal),
					),
				).WithShowHelp(true)
				return v, v.modelForm.Init()
			}
			options := make([]huh.Option[string], 0, len(msg.models)+1)
			for _, m := range msg.models {
				options = append(options, huh.NewOption(m, m))
			}
			options = append(options, huh.NewOption("Other (custom)", "__other__"))
			v.modelForm = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Which model?").
						Options(options...).
						Value(&v.modelVal),
				),
			).WithShowHelp(true)
			return v, v.modelForm.Init()
		}
		return v, nil

	case 1: // Model selection
		form, cmd := v.modelForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.modelForm = f
		}
		if v.modelForm.State == huh.StateCompleted {
			if v.modelVal == "__other__" {
				v.modelVal = ""
				v.modelForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&v.modelVal),
					),
				).WithShowHelp(true)
				return v, v.modelForm.Init()
			}
			// Validate
			v.modelPhase = 2
			v.modelSpin = newSpinnerStep("Verifying connection...")
			return v, tea.Batch(v.modelSpin.spinner.Tick, v.validateModel())
		}
		return v, cmd

	case 2: // Validating
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			v.modelSpin.spinner, cmd = v.modelSpin.spinner.Update(msg)
			return v, cmd
		case validateDoneMsg:
			if msg.err != nil {
				v.modelSpin.err = fmt.Errorf("connection failed — model may not support structured output")
				return v, nil
			}
			v.modelSpin.done = true
			v.cfg.AI.Model = v.modelVal
			return v, v.startSave(
				fmt.Sprintf("Connected to %s / %s", v.cfg.AI.Provider, v.modelVal),
				nil,
			)
		}
		return v, nil
	}
	return v, nil
}

func (v *ReconfigureView) validateModel() tea.Cmd {
	model := v.modelVal
	cfg := v.cfg
	return func() tea.Msg {
		apiKey := cfg.ResolveAPIKey()
		provider, _ := ai.GetProvider(cfg.AI.Provider)
		classifier := ai.NewClassifier(apiKey, provider.BaseURL, model, cfg.Labels)
		err := classifier.ValidateConnection(context.Background())
		return validateDoneMsg{err: err}
	}
}

// ─── Labels update ───────────────────────────────

func (v *ReconfigureView) updateLabels(msg tea.Msg) (tui.View, tea.Cmd) {
	switch v.labelPhase {
	case 0: // Sub-menu: add / remove / back
		form, cmd := v.labelForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.labelForm = f
		}
		if v.labelForm.State == huh.StateCompleted {
			switch v.labelAction {
			case "back":
				v.phase = phaseMenu
				v.buildMenu()
				return v, v.menuForm.Init()
			case "add":
				v.labelPhase = 1
				v.labelDupErr = ""
				v.labelName = ""
				v.labelForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Label name").
							Value(&v.labelName),
					),
				).WithShowHelp(true)
				return v, v.labelForm.Init()
			case "remove":
				if len(v.cfg.Labels) == 0 {
					// Nothing to remove — go back to label menu
					return v, v.showLabelMenu()
				}
				v.labelPhase = 3
				v.labelRemSel = nil
				options := make([]huh.Option[string], len(v.cfg.Labels))
				for i, l := range v.cfg.Labels {
					options[i] = huh.NewOption(fmt.Sprintf("%s — %s", l.Name, l.Description), l.Name)
				}
				v.labelForm = huh.NewForm(
					huh.NewGroup(
						huh.NewMultiSelect[string]().
							Title("Select labels to remove").
							Options(options...).
							Value(&v.labelRemSel),
					),
				).WithShowHelp(true)
				return v, v.labelForm.Init()
			}
		}
		return v, cmd

	case 1: // Name input
		form, cmd := v.labelForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.labelForm = f
		}
		if v.labelForm.State == huh.StateCompleted {
			if v.labelName == "" {
				// Empty name — go back to label menu
				return v, v.showLabelMenu()
			}
			// Check duplicate
			if v.isLabelDuplicate(v.labelName) {
				v.labelDupErr = fmt.Sprintf("Label %q already exists", v.labelName)
				v.labelName = ""
				v.labelForm = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Label name (try a different name)").
							Value(&v.labelName),
					),
				).WithShowHelp(true)
				return v, v.labelForm.Init()
			}
			v.labelPhase = 2
			v.labelDupErr = ""
			v.labelDesc = ""
			v.labelForm = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Description (helps AI classify)").
						Value(&v.labelDesc),
				),
			).WithShowHelp(true)
			return v, v.labelForm.Init()
		}
		return v, cmd

	case 2: // Desc input — then save
		form, cmd := v.labelForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.labelForm = f
		}
		if v.labelForm.State == huh.StateCompleted {
			desc := v.labelDesc
			if desc == "" {
				desc = "Custom label"
			}
			newLabel := config.Label{Name: v.labelName, Description: desc}
			v.cfg.Labels = append(v.cfg.Labels, newLabel)
			added := []config.Label{newLabel}
			return v, v.startSave(
				fmt.Sprintf("Added label: %s", v.labelName),
				v.syncAddedLabels(added),
			)
		}
		return v, cmd

	case 3: // Remove multi-select
		form, cmd := v.labelForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			v.labelForm = f
		}
		if v.labelForm.State == huh.StateCompleted {
			if len(v.labelRemSel) == 0 {
				// Nothing selected — go back to label menu
				return v, v.showLabelMenu()
			}
			removeSet := make(map[string]bool)
			for _, n := range v.labelRemSel {
				removeSet[n] = true
			}
			var removed []config.Label
			var kept []config.Label
			for _, l := range v.cfg.Labels {
				if removeSet[l.Name] {
					removed = append(removed, l)
				} else {
					kept = append(kept, l)
				}
			}
			v.cfg.Labels = kept
			return v, v.startSave(
				fmt.Sprintf("Removed %d label(s)", len(removed)),
				v.syncRemovedLabels(removed),
			)
		}
		return v, cmd
	}
	return v, nil
}

func (v *ReconfigureView) isLabelDuplicate(name string) bool {
	for _, l := range v.cfg.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

func (v *ReconfigureView) syncAddedLabels(added []config.Label) func() tea.Msg {
	return func() tea.Msg {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		client, err := gmailpkg.NewClient(context.Background(), ts)
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		store, err := db.Open(config.DBPath())
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		defer store.Close()
		customIdx := 0
		for _, l := range added {
			bg, tx := gmailpkg.ColorForLabel(l.Name, customIdx)
			if !gmailpkg.IsDefaultLabel(l.Name) {
				customIdx++
			}
			gmailID, err := client.CreateLabel(context.Background(), l.Name, bg, tx)
			if err != nil {
				continue
			}
			store.SetLabelMappingWithColor(l.Name, gmailID, bg, tx)
		}
		return labelSyncDoneMsg{}
	}
}

func (v *ReconfigureView) syncRemovedLabels(removed []config.Label) func() tea.Msg {
	return func() tea.Msg {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		client, err := gmailpkg.NewClient(context.Background(), ts)
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		store, err := db.Open(config.DBPath())
		if err != nil {
			return labelSyncDoneMsg{err: err}
		}
		defer store.Close()
		for _, l := range removed {
			gmailID, err := store.GetLabelMapping(l.Name)
			if err != nil {
				continue
			}
			client.DeleteLabel(context.Background(), gmailID)
			store.DeleteLabelMapping(l.Name)
		}
		return labelSyncDoneMsg{}
	}
}

// ─── Poll update ─────────────────────────────────

func (v *ReconfigureView) updatePoll(msg tea.Msg) (tui.View, tea.Cmd) {
	form, cmd := v.pollForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		v.pollForm = f
	}
	if v.pollForm.State == huh.StateCompleted {
		interval, err := strconv.Atoi(v.pollStr)
		if err != nil || interval <= 0 {
			v.pollErr = "Please enter a positive number"
			v.pollStr = ""
			v.pollForm = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Poll interval (seconds)").
						Value(&v.pollStr),
				),
			).WithShowHelp(true)
			return v, v.pollForm.Init()
		}
		v.cfg.PollInterval = interval
		return v, v.startSave(fmt.Sprintf("Poll interval set to %ds", interval), nil)
	}
	return v, cmd
}

// ─── Save phase ──────────────────────────────────

// startSave transitions to the saving phase. It saves config and optionally
// runs a background task (like syncing Gmail labels) before restarting the daemon.
func (v *ReconfigureView) startSave(statusLine string, bgWork func() tea.Msg) tea.Cmd {
	v.phase = phaseSaving
	v.saveStatus = "  " + tui.SuccessStyle.Render("✓ "+statusLine)
	v.saveSpinner = newSpinnerStep("Saving...")

	// Save config immediately
	if err := config.Save(config.DefaultPath(), v.cfg); err != nil {
		v.saveSpinner.err = fmt.Errorf("saving config: %w", err)
		return nil
	}

	if bgWork != nil {
		v.saveSpinner = newSpinnerStep("Syncing with Gmail...")
		return tea.Batch(v.saveSpinner.spinner.Tick, func() tea.Msg { return bgWork() })
	}

	// No bg work — go straight to daemon restart
	return v.startDaemonRestart()
}

func (v *ReconfigureView) startDaemonRestart() tea.Cmd {
	v.saveSpinner = newSpinnerStep("Restarting daemon...")
	return tea.Batch(v.saveSpinner.spinner.Tick, func() tea.Msg {
		mgr := service.Detect()
		if mgr == nil {
			return daemonRestartedMsg{}
		}
		running, _ := mgr.IsRunning()
		if !running {
			return daemonRestartedMsg{}
		}
		mgr.Stop()
		binaryPath := "labelr"
		if p, err := os.Executable(); err == nil {
			binaryPath = p
		}
		mgr.Install(binaryPath)
		mgr.Start()
		return daemonRestartedMsg{}
	})
}

func (v *ReconfigureView) updateSaving(msg tea.Msg) (tui.View, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		v.saveSpinner.spinner, cmd = v.saveSpinner.spinner.Update(msg)
		return v, cmd
	case labelSyncDoneMsg:
		if msg.err != nil {
			v.saveStatus += "\n  " + tui.ErrorStyle.Render("✗ "+msg.err.Error())
		} else {
			v.saveStatus += "\n  " + tui.SuccessStyle.Render("✓ Gmail synced")
		}
		return v, v.startDaemonRestart()
	case daemonRestartedMsg:
		v.saveSpinner.done = true
		v.saveStatus += "\n  " + tui.SuccessStyle.Render("✓ Config saved")
		// Return to menu
		v.phase = phaseMenu
		v.buildMenu()
		return v, v.menuForm.Init()
	}
	return v, nil
}

// ─── View ────────────────────────────────────────

func (v *ReconfigureView) View() string {
	summary := v.renderSummary()

	switch v.phase {
	case phaseMenu:
		return summary + "\n" + v.menuForm.View()

	case phaseSubflow:
		return summary + "\n" + v.renderSubflow()

	case phaseSaving:
		content := v.saveStatus
		if !v.saveSpinner.done {
			content += "\n" + v.saveSpinner.SpinnerView()
		}
		return summary + "\n" + content
	}
	return summary
}

func (v *ReconfigureView) renderSummary() string {
	bold := lipgloss.NewStyle().Bold(true)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %-12s %s\n", bold.Render("Gmail"), v.cfg.Gmail.Email))
	sb.WriteString(fmt.Sprintf("  %-12s %s / %s\n", bold.Render("Provider"), v.cfg.AI.Provider, v.cfg.AI.Model))
	sb.WriteString(fmt.Sprintf("  %-12s %d labels\n", bold.Render("Labels"), len(v.cfg.Labels)))
	sb.WriteString(fmt.Sprintf("  %-12s every %ds\n", bold.Render("Polling"), v.cfg.PollInterval))
	return sb.String()
}

func (v *ReconfigureView) renderSubflow() string {
	switch v.subflow {
	case subflowGmail:
		return v.gmailSpinner.SpinnerView() + "\n\n" +
			tui.DimStyle.Render("  A browser window will open for Google sign-in...")

	case subflowAI:
		switch v.aiPhase {
		case 0:
			return v.aiForm.View()
		case 1:
			return v.aiSpinner.SpinnerView()
		case 2, 3:
			providerLine := fmt.Sprintf("  Provider: %s", lipgloss.NewStyle().Bold(true).Render(v.aiProvider))
			if v.aiPhase == 3 && v.aiModel != "" {
				providerLine += fmt.Sprintf("  Model: %s", lipgloss.NewStyle().Bold(true).Render(v.aiModel))
			}
			return providerLine + "\n\n" + v.aiForm.View()
		case 4:
			providerLine := fmt.Sprintf("  Provider: %s  Model: %s",
				lipgloss.NewStyle().Bold(true).Render(v.aiProvider),
				lipgloss.NewStyle().Bold(true).Render(v.aiModel))
			return providerLine + "\n" + v.aiSpinner.SpinnerView()
		}

	case subflowModel:
		switch v.modelPhase {
		case 0:
			return v.modelSpin.SpinnerView()
		case 1:
			return v.modelForm.View()
		case 2:
			return v.modelSpin.SpinnerView()
		}

	case subflowLabels:
		// Show current labels list above the form
		var labelList string
		if len(v.cfg.Labels) > 0 && v.labelPhase == 0 {
			labelList = tui.DimStyle.Render(fmt.Sprintf("  Current: %d labels", len(v.cfg.Labels))) + "\n"
			for _, l := range v.cfg.Labels {
				labelList += tui.DimStyle.Render(fmt.Sprintf("    · %s", l.Name)) + "\n"
			}
			labelList += "\n"
		}
		errInfo := ""
		if v.labelDupErr != "" {
			errInfo = "  " + tui.ErrorStyle.Render(v.labelDupErr) + "\n\n"
		}
		return labelList + errInfo + v.labelForm.View()

	case subflowPoll:
		errInfo := ""
		if v.pollErr != "" {
			errInfo = "  " + tui.ErrorStyle.Render(v.pollErr) + "\n\n"
		}
		return errInfo + v.pollForm.View()
	}
	return ""
}
