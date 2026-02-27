package setup

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pankajbeniwal/labelr/internal/ai"
	"github.com/pankajbeniwal/labelr/internal/config"
	gmailpkg "github.com/pankajbeniwal/labelr/internal/gmail"
	"github.com/pankajbeniwal/labelr/internal/tui"
)

// ──────────────────────────────────────────
// Step 1: Gmail OAuth
// ──────────────────────────────────────────

type gmailDoneMsg struct {
	email     string
	historyID uint64
	err       error
}

type gmailStep struct {
	deps    *Deps
	spinner SpinnerStep
	email   string
	done    bool
	err     error
}

func newGmailStep(deps *Deps) *gmailStep {
	return &gmailStep{
		deps:    deps,
		spinner: newSpinnerStep("Connecting to Gmail..."),
	}
}

func (s *gmailStep) CanGoBack() bool { return false }
func (s *gmailStep) Done() bool      { return s.done }

func (s *gmailStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyQuit}
}

func (s *gmailStep) Init() tea.Cmd {
	// Check if already authenticated
	if s.deps.Cfg.Gmail.Email != "" {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err == nil {
			ctx := context.Background()
			client, err := gmailpkg.NewClient(ctx, ts)
			if err == nil {
				email, _, err := client.GetProfile(ctx)
				if err == nil {
					s.email = email
					s.done = true
					return nil
				}
			}
		}
	}
	return tea.Batch(s.spinner.spinner.Tick, s.authenticate())
}

func (s *gmailStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd
	case gmailDoneMsg:
		if msg.err != nil {
			s.err = msg.err
			s.spinner.err = msg.err
			return s, nil
		}
		s.email = msg.email
		s.deps.Cfg.Gmail.Email = msg.email
		// Store historyID for later use in finish step
		s.deps.Store.SetState("history_id", fmt.Sprintf("%d", msg.historyID))
		s.spinner.done = true
		s.done = true
	case tea.KeyMsg:
		if key.Matches(msg, tui.KeyQuit) {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *gmailStep) View() string {
	if s.done {
		return fmt.Sprintf("  %s Connected as %s",
			tui.SuccessStyle.Render("✓"),
			lipgloss.NewStyle().Bold(true).Render(s.email))
	}
	return s.spinner.SpinnerView() + "\n\n" + tui.DimStyle.Render("  A browser window will open for Google sign-in...")
}

func (s *gmailStep) authenticate() tea.Cmd {
	return func() tea.Msg {
		// Run the OAuth flow (opens browser, saves token)
		_, err := gmailpkg.Authenticate(config.CredentialsPath())
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("authentication failed: %w", err)}
		}

		// Now get a token source from the saved token
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("creating token source: %w", err)}
		}

		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("creating client: %w", err)}
		}

		email, historyID, err := client.GetProfile(ctx)
		if err != nil {
			return gmailDoneMsg{err: fmt.Errorf("getting email: %w", err)}
		}

		return gmailDoneMsg{email: email, historyID: historyID}
	}
}

// ──────────────────────────────────────────
// Step 2: AI Provider Selection
// ──────────────────────────────────────────

type modelsFetchedMsg struct {
	models []string
	err    error
}

type aiStep struct {
	deps     *Deps
	phase    int // 0=provider, 1=model-fetch, 2=model-select, 3=apikey
	form     *huh.Form
	spinner  SpinnerStep
	provider string
	model    string
	apiKey   string
	models   []string
	done     bool
}

func newAIStep(deps *Deps) *aiStep {
	return &aiStep{
		deps: deps,
	}
}

func (s *aiStep) CanGoBack() bool { return true }
func (s *aiStep) Done() bool      { return s.done }

func (s *aiStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter}
}

func (s *aiStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false
	providerNames := ai.ProviderNamesOrdered()
	options := make([]huh.Option[string], len(providerNames))
	for i, name := range providerNames {
		options[i] = huh.NewOption(name, name)
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Choose your AI provider").
				Options(options...).
				Value(&s.provider),
		),
	).WithShowHelp(true)

	return s.form.Init()
}

func (s *aiStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch s.phase {
	case 0: // Provider selection form
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.phase = 1
			s.spinner = newSpinnerStep("Fetching available models...")
			return s, tea.Batch(s.spinner.spinner.Tick, s.fetchModels())
		}
		return s, cmd

	case 1: // Fetching models
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
			return s, cmd
		case modelsFetchedMsg:
			if msg.err != nil || len(msg.models) == 0 {
				// Fall back to text input
				s.phase = 2
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&s.model),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			s.models = msg.models
			s.phase = 2
			options := make([]huh.Option[string], 0, len(msg.models)+1)
			for _, m := range msg.models {
				options = append(options, huh.NewOption(m, m))
			}
			options = append(options, huh.NewOption("Other (custom)", "__other__"))
			s.form = huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Which model?").
						Options(options...).
						Value(&s.model),
				),
			).WithShowHelp(true)
			return s, s.form.Init()
		}
		return s, nil

	case 2: // Model selection
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			if s.model == "__other__" {
				s.model = ""
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Enter model name").
							Value(&s.model),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			// Move to API key (skip for ollama)
			if s.provider == "ollama" {
				s.deps.Cfg.AI.Provider = s.provider
				s.deps.Cfg.AI.Model = s.model
				s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
				s.done = true
				return s, nil
			}
			s.phase = 3
			// Check for env var
			envKey := ai.EnvKeyForProvider(s.provider)
			if envKey != "" {
				if val := os.Getenv(envKey); val != "" {
					s.apiKey = val
					s.deps.Cfg.AI.Provider = s.provider
					s.deps.Cfg.AI.Model = s.model
					s.deps.Cfg.AI.APIKey = s.apiKey
					s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
					s.done = true
					return s, nil
				}
			}
			s.form = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("API key").
						EchoMode(huh.EchoModePassword).
						Value(&s.apiKey),
				),
			).WithShowHelp(true)
			return s, s.form.Init()
		}
		return s, cmd

	case 3: // API key input
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.deps.Cfg.AI.Provider = s.provider
			s.deps.Cfg.AI.Model = s.model
			s.deps.Cfg.AI.APIKey = s.apiKey
			s.deps.Cfg.AI.BaseURL = ai.ProviderBaseURL(s.provider)
			s.done = true
		}
		return s, cmd
	}

	return s, nil
}

func (s *aiStep) View() string {
	switch s.phase {
	case 0:
		return s.form.View()
	case 1:
		return s.spinner.SpinnerView()
	case 2, 3:
		providerLine := fmt.Sprintf("  Provider: %s", lipgloss.NewStyle().Bold(true).Render(s.provider))
		if s.phase == 3 && s.model != "" {
			providerLine += fmt.Sprintf("  Model: %s", lipgloss.NewStyle().Bold(true).Render(s.model))
		}
		return providerLine + "\n\n" + s.form.View()
	}
	return ""
}

func (s *aiStep) fetchModels() tea.Cmd {
	return func() tea.Msg {
		if s.provider == "ollama" {
			models, err := ai.FetchOllamaModels()
			return modelsFetchedMsg{models: models, err: err}
		}
		models, err := ai.FetchModelsForProvider(s.provider)
		return modelsFetchedMsg{models: models, err: err}
	}
}

// ──────────────────────────────────────────
// Step 3: Validate AI Connection
// ──────────────────────────────────────────

type validateDoneMsg struct {
	err error
}

type validateStep struct {
	deps    *Deps
	spinner SpinnerStep
	done    bool
	err     error
}

func newValidateStep(deps *Deps) *validateStep {
	return &validateStep{
		deps:    deps,
		spinner: newSpinnerStep("Validating AI connection..."),
	}
}

func (s *validateStep) CanGoBack() bool { return true }
func (s *validateStep) Done() bool      { return s.done }

func (s *validateStep) HelpKeys() []key.Binding {
	if s.err != nil {
		return []key.Binding{tui.KeyEnter, tui.KeyBack, tui.KeyQuit}
	}
	return []key.Binding{tui.KeyQuit}
}

func (s *validateStep) Init() tea.Cmd {
	s.done = false
	s.err = nil
	s.spinner = newSpinnerStep("Validating AI connection...")
	return tea.Batch(s.spinner.spinner.Tick, s.validate())
}

func (s *validateStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd
	case validateDoneMsg:
		if msg.err != nil {
			s.err = msg.err
			s.spinner.err = msg.err
			return s, nil
		}
		s.spinner.done = true
		s.done = true
	case tea.KeyMsg:
		if s.err != nil && key.Matches(msg, tui.KeyEnter) {
			// Retry
			return s, s.Init()
		}
		if key.Matches(msg, tui.KeyQuit) {
			return s, tea.Quit
		}
	}
	return s, nil
}

func (s *validateStep) View() string {
	view := s.spinner.SpinnerView()
	if s.err != nil {
		view += "\n\n" + tui.DimStyle.Render("  Press enter to retry or esc to go back")
	}
	return view
}

func (s *validateStep) validate() tea.Cmd {
	return func() tea.Msg {
		cfg := s.deps.Cfg
		apiKey := cfg.ResolveAPIKey()
		classifier := ai.NewClassifier(apiKey, cfg.AI.BaseURL, cfg.AI.Model, cfg.Labels)
		err := classifier.ValidateConnection(context.Background())
		return validateDoneMsg{err: err}
	}
}

// ──────────────────────────────────────────
// Step 4: Labels
// ──────────────────────────────────────────

type labelsStep struct {
	deps        *Deps
	form        *huh.Form
	phase       int // 0=multiselect, 1=add-custom-loop, 2=name-input, 3=desc-input
	selected    []string
	customDescs map[string]string // name -> description for custom labels
	adding      bool
	newLabel    string
	newDesc     string
	dupErr      string
	done        bool
}

func newLabelsStep(deps *Deps) *labelsStep {
	return &labelsStep{deps: deps, customDescs: make(map[string]string)}
}

func (s *labelsStep) CanGoBack() bool { return true }
func (s *labelsStep) Done() bool      { return s.done }

func (s *labelsStep) HelpKeys() []key.Binding {
	return []key.Binding{tui.KeyUp, tui.KeyDown, tui.KeyEnter}
}

func (s *labelsStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false

	defaults := config.DefaultLabels()
	options := make([]huh.Option[string], len(defaults))
	for i, l := range defaults {
		options[i] = huh.NewOption(l.Name, l.Name).Selected(true)
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select labels to use").
				Options(options...).
				Value(&s.selected),
		),
	).WithShowHelp(true)

	return s.form.Init()
}

func (s *labelsStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch s.phase {
	case 0: // Multi-select defaults
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			s.phase = 1
			return s, s.showAddCustom()
		}
		return s, cmd

	case 1: // "Add a custom label?" confirm
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			if s.adding {
				// They want to add — show name input
				s.phase = 2
				s.dupErr = ""
				s.newLabel = ""
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Label name").
							Value(&s.newLabel),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			// They chose not to add more — done
			s.finalize()
		}
		return s, cmd

	case 2: // Name input
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			// Check for duplicate
			if s.isDuplicate(s.newLabel) {
				s.dupErr = fmt.Sprintf("Label %q already exists", s.newLabel)
				s.newLabel = ""
				s.form = huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Label name (try a different name)").
							Value(&s.newLabel),
					),
				).WithShowHelp(true)
				return s, s.form.Init()
			}
			// Ask for description
			s.phase = 3
			s.dupErr = ""
			s.newDesc = ""
			s.form = huh.NewForm(
				huh.NewGroup(
					huh.NewInput().
						Title("Description (helps AI classify)").
						Value(&s.newDesc),
				),
			).WithShowHelp(true)
			return s, s.form.Init()
		}
		return s, cmd

	case 3: // Description input
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			// Add label with description
			s.selected = append(s.selected, s.newLabel)
			desc := s.newDesc
			if desc == "" {
				desc = "Custom label"
			}
			s.customDescs[s.newLabel] = desc
			s.newLabel = ""
			s.newDesc = ""
			s.phase = 1
			return s, s.showAddCustom()
		}
		return s, cmd
	}

	return s, nil
}

func (s *labelsStep) isDuplicate(name string) bool {
	for _, existing := range s.selected {
		if existing == name {
			return true
		}
	}
	return false
}

func (s *labelsStep) showAddCustom() tea.Cmd {
	s.adding = true
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add a custom label?").
				Value(&s.adding),
		),
	).WithShowHelp(true)
	return s.form.Init()
}

func (s *labelsStep) finalize() {
	// Build final label list
	defaults := config.DefaultLabels()
	defaultMap := make(map[string]config.Label)
	for _, l := range defaults {
		defaultMap[l.Name] = l
	}

	var labels []config.Label
	for _, name := range s.selected {
		if l, ok := defaultMap[name]; ok {
			labels = append(labels, l)
		} else {
			desc := s.customDescs[name]
			if desc == "" {
				desc = "Custom label"
			}
			labels = append(labels, config.Label{Name: name, Description: desc})
		}
	}
	s.deps.Cfg.Labels = labels
	s.done = true
}

func (s *labelsStep) View() string {
	selectedInfo := ""
	if len(s.selected) > 0 && s.phase >= 1 {
		selectedInfo = tui.DimStyle.Render(fmt.Sprintf("  Selected: %d labels", len(s.selected))) + "\n\n"
	}
	errInfo := ""
	if s.dupErr != "" {
		errInfo = "  " + tui.ErrorStyle.Render(s.dupErr) + "\n\n"
	}
	return selectedInfo + errInfo + s.form.View()
}

// ──────────────────────────────────────────
// Step 5: Finish (create labels, start daemon)
// ──────────────────────────────────────────

type labelCreateDoneMsg struct {
	err error
}

type testQueueDoneMsg struct {
	count int
	err   error
}

type daemonStartDoneMsg struct {
	err error
}

type finishStep struct {
	deps        *Deps
	spinner     SpinnerStep
	form        *huh.Form
	testConfirm bool
	testQueued  int
	phase       int // 0=creating-labels, 1=offer-test, 2=queuing-test, 3=starting-daemon, 4=done
	done        bool
}

func newFinishStep(deps *Deps) *finishStep {
	return &finishStep{
		deps: deps,
	}
}

func (s *finishStep) CanGoBack() bool { return s.phase == 0 }
func (s *finishStep) Done() bool      { return s.done }

func (s *finishStep) HelpKeys() []key.Binding {
	if s.phase == 1 {
		return []key.Binding{tui.KeyEnter}
	}
	return []key.Binding{tui.KeyQuit}
}

func (s *finishStep) Init() tea.Cmd {
	s.phase = 0
	s.done = false
	s.spinner = newSpinnerStep("Creating labels in Gmail...")
	return tea.Batch(s.spinner.spinner.Tick, s.createLabels())
}

func (s *finishStep) Update(msg tea.Msg) (Step, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		s.spinner.spinner, cmd = s.spinner.spinner.Update(msg)
		return s, cmd

	case labelCreateDoneMsg:
		if msg.err != nil {
			s.spinner.err = msg.err
			return s, nil
		}
		s.spinner.done = true
		// Save config now
		config.Save(config.DefaultPath(), s.deps.Cfg)

		// Offer test run
		s.phase = 1
		s.testConfirm = true
		s.form = huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Label your 10 most recent emails to test?").
					Value(&s.testConfirm),
			),
		).WithShowHelp(true)
		return s, s.form.Init()

	case testQueueDoneMsg:
		if msg.err != nil {
			s.spinner.err = msg.err
		} else {
			s.testQueued = msg.count
			s.spinner.done = true
		}
		// Move to start daemon
		s.phase = 3
		s.spinner = newSpinnerStep("Starting background service...")
		return s, tea.Batch(s.spinner.spinner.Tick, s.startDaemon())

	case daemonStartDoneMsg:
		if msg.err != nil {
			s.spinner.err = msg.err
		} else {
			s.spinner.done = true
		}
		s.phase = 4
		s.done = true
		return s, nil

	case tea.KeyMsg:
		if key.Matches(msg, tui.KeyQuit) {
			return s, tea.Quit
		}
	}

	// Delegate to form if in offer-test phase
	if s.form != nil && s.phase == 1 {
		form, cmd := s.form.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			s.form = f
		}
		if s.form.State == huh.StateCompleted {
			if s.testConfirm {
				// Queue 10 recent emails
				s.phase = 2
				s.spinner = newSpinnerStep("Queuing recent emails...")
				return s, tea.Batch(s.spinner.spinner.Tick, s.queueTestEmails())
			}
			// Skip test — go straight to daemon
			s.phase = 3
			s.spinner = newSpinnerStep("Starting background service...")
			return s, tea.Batch(s.spinner.spinner.Tick, s.startDaemon())
		}
		return s, cmd
	}

	return s, nil
}

func (s *finishStep) View() string {
	switch s.phase {
	case 0:
		return s.spinner.SpinnerView()
	case 1:
		return "  " + tui.SuccessStyle.Render("✓ Labels created") + "\n\n" + s.form.View()
	case 2:
		lines := "  " + tui.SuccessStyle.Render("✓ Labels created")
		return lines + "\n" + s.spinner.SpinnerView()
	case 3:
		lines := "  " + tui.SuccessStyle.Render("✓ Labels created")
		if s.testQueued > 0 {
			lines += "\n  " + tui.SuccessStyle.Render(fmt.Sprintf("✓ Queued %d emails for labeling", s.testQueued))
		}
		return lines + "\n" + s.spinner.SpinnerView()
	default:
		lines := "  " + tui.SuccessStyle.Render("✓ Labels created")
		if s.testQueued > 0 {
			lines += "\n  " + tui.SuccessStyle.Render(fmt.Sprintf("✓ Queued %d emails for labeling", s.testQueued))
		}
		if s.spinner.done {
			lines += "\n  " + tui.SuccessStyle.Render("✓ Background service started")
		} else if s.spinner.err != nil {
			lines += "\n  " + tui.ErrorStyle.Render("✗ "+s.spinner.err.Error())
		}
		lines += "\n\n  " + lipgloss.NewStyle().Bold(true).Render("Setup complete!") +
			"\n  " + tui.DimStyle.Render("Use 'labelr status' to monitor.")
		return lines
	}
}

func (s *finishStep) createLabels() tea.Cmd {
	return func() tea.Msg {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return labelCreateDoneMsg{err: err}
		}

		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return labelCreateDoneMsg{err: err}
		}

		customIdx := 0
		for _, label := range s.deps.Cfg.Labels {
			bgColor, textColor := gmailpkg.ColorForLabel(label.Name, customIdx)
			if !gmailpkg.IsDefaultLabel(label.Name) {
				customIdx++
			}
			gmailID, err := client.CreateLabel(ctx, label.Name, bgColor, textColor)
			if err != nil {
				return labelCreateDoneMsg{err: fmt.Errorf("creating label %q: %w", label.Name, err)}
			}
			s.deps.Store.SetLabelMappingWithColor(label.Name, gmailID, bgColor, textColor)
		}

		return labelCreateDoneMsg{}
	}
}

func (s *finishStep) queueTestEmails() tea.Cmd {
	return func() tea.Msg {
		ts, err := gmailpkg.TokenSource(config.CredentialsPath())
		if err != nil {
			return testQueueDoneMsg{err: err}
		}

		ctx := context.Background()
		client, err := gmailpkg.NewClient(ctx, ts)
		if err != nil {
			return testQueueDoneMsg{err: err}
		}

		msgs, err := client.ListRecentMessages(ctx, 10)
		if err != nil {
			return testQueueDoneMsg{err: err}
		}

		count := 0
		for _, m := range msgs {
			if err := s.deps.Store.InsertMessage(m.ID, m.ThreadID); err != nil {
				continue
			}
			count++
		}
		return testQueueDoneMsg{count: count}
	}
}

func (s *finishStep) startDaemon() tea.Cmd {
	return func() tea.Msg {
		mgr := s.deps.Mgr
		if mgr == nil {
			return daemonStartDoneMsg{err: fmt.Errorf("unsupported OS for background service")}
		}

		binaryPath, err := os.Executable()
		if err != nil {
			return daemonStartDoneMsg{err: err}
		}

		if err := mgr.Install(binaryPath); err != nil {
			return daemonStartDoneMsg{err: fmt.Errorf("installing service: %w", err)}
		}

		if err := mgr.Start(); err != nil {
			return daemonStartDoneMsg{err: fmt.Errorf("starting service: %w", err)}
		}

		return daemonStartDoneMsg{}
	}
}
