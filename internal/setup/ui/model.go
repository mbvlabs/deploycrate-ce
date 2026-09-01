package ui

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"deploycrate-ce/internal/setup"
)

type screen int

const (
	screenWelcome screen = iota
	screenChannel
	screenServer
	screenAccess
	screenAdmin
	screenDatabaseChoice
	screenDatabase
	screenStorageChoice
	screenStorageProvider
	screenStorage
	screenServerBackupPolicy
	screenDatabaseBackupPolicy
	screenValidating
	screenReview
	screenRunning
	screenHandoff
)

type runEventMsg struct{ event setup.Event }
type runClosedMsg struct{}
type validationMsg struct{ err error }
type channelReleaseMsg struct {
	channel string
	version string
	err     error
}
type rebootMsg struct{ err error }

const installerConfigStepID = "installer-config"

type Model struct {
	config              setup.Config
	host                setup.HostInfo
	dryRun              bool
	screen              screen
	inputs              []textinput.Model
	focus               int
	choice              int
	width               int
	height              int
	err                 error
	events              []setup.Event
	eventStream         <-chan setup.Event
	cancel              context.CancelFunc
	installDone         bool
	configSaved         bool
	credentialsVerified bool
	handoffConfirmation textinput.Model
	handoffFocus        int
	copyRequested       bool
	rebootStarted       bool
	activity            spinner.Model
	operations          setup.Operations
}

var (
	colorPink   = lipgloss.Color("#ff6fcf")
	colorViolet = lipgloss.Color("#9d86ff")
	colorCyan   = lipgloss.Color("#58d7f3")
	colorGreen  = lipgloss.Color("#5ee6a8")
	colorAmber  = lipgloss.Color("#ffcf70")
	colorRed    = lipgloss.Color("#ff7b88")
	colorMuted  = lipgloss.Color("#77859d")
	colorBorder = lipgloss.Color("#34415a")

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPink)
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(1, 2)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	labelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	errorStyle = lipgloss.NewStyle().Foreground(colorRed)
	okStyle    = lipgloss.NewStyle().Foreground(colorGreen)
	warnStyle  = lipgloss.NewStyle().Foreground(colorAmber)
)

func NewModel(
	cfg setup.Config,
	host setup.HostInfo,
	dryRun bool,
	operations setup.Operations,
) Model {
	activity := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(colorPink)),
	)
	return Model{
		config:     cfg,
		host:       host,
		dryRun:     dryRun,
		screen:     screenWelcome,
		width:      88,
		height:     30,
		activity:   activity,
		operations: operations,
	}
}

func NewHandoffModel(cfg setup.Config, dryRun, credentialsVerified bool) Model {
	confirmation := newHandoffConfirmation()
	return Model{
		config: cfg, dryRun: dryRun, screen: screenHandoff, width: 88, height: 30,
		installDone: true, configSaved: !dryRun, credentialsVerified: credentialsVerified,
		handoffConfirmation: confirmation,
	}
}

func (m Model) Init() tea.Cmd {
	if m.screen == screenHandoff && m.handoffConfirmation.Focused() {
		return textinput.Blink
	}
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case channelReleaseMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.config.UpdateChannel = msg.channel
		m.config.Version = msg.version
		m.setScreen(screenServer)
		return m, textinput.Blink
	case validationMsg:
		if msg.err != nil {
			m.err = msg.err
			if strings.Contains(msg.err.Error(), "database") {
				m.screen = screenDatabase
			} else {
				m.screen = screenStorage
			}
			return m, nil
		}
		if m.config.S3.Enabled {
			m.config.S3.ValidatedAt = time.Now().UTC()
		}
		m.screen = screenReview
		return m, nil
	case runEventMsg:
		m.events = append(m.events, msg.event)
		if msg.event.StepID == installerConfigStepID && msg.event.Kind == setup.EventCompleted {
			m.configSaved = true
		}
		if msg.event.Err != nil {
			m.err = msg.event.Err
		}
		if msg.event.Kind == setup.EventFinished && msg.event.Err == nil {
			m.installDone = true
			m.screen = screenHandoff
			m.credentialsVerified = false
			m.handoffConfirmation = newHandoffConfirmation()
			m.handoffFocus = 0
			m.copyRequested = false
			return m, nil
		}
		return m, waitForEvent(m.eventStream)
	case runClosedMsg:
		if !m.installDone && m.err == nil {
			m.err = errors.New("installer stopped before completion")
		}
		return m, nil
	case rebootMsg:
		m.rebootStarted = msg.err == nil
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		return m, tea.Quit
	case spinner.TickMsg:
		if (m.screen != screenRunning && m.screen != screenValidating) || m.err != nil {
			return m, nil
		}
		var command tea.Cmd
		m.activity, command = m.activity.Update(msg)
		return m, command
	case tea.PasteMsg:
		if m.screen == screenRunning || m.screen == screenValidating {
			return m, nil
		}
		if m.screen == screenHandoff {
			if m.handoffFocus != 1 {
				return m, nil
			}
			var command tea.Cmd
			m.handoffConfirmation, command = m.handoffConfirmation.Update(msg)
			return m, command
		}
		if m.focus < 0 || m.focus >= len(m.inputs) {
			return m, nil
		}
		var command tea.Cmd
		m.inputs[m.focus], command = m.inputs[m.focus].Update(msg)
		return m, command
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "ctrl+c" {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if m.screen == screenRunning || m.screen == screenValidating {
			return m, nil
		}
		if m.screen == screenHandoff {
			return m.updateHandoff(msg)
		}
		if m.screen == screenChannel || m.screen == screenDatabaseChoice ||
			m.screen == screenStorageChoice || m.screen == screenStorageProvider {
			return m.updateChoice(key)
		}
		if m.screen == screenWelcome || m.screen == screenReview {
			if key == "enter" {
				if m.screen == screenWelcome {
					m.screen = screenChannel
					m.choice = 0
					return m, nil
				}
				return m.startInstall()
			}
			return m, nil
		}
		return m.updateForm(msg)
	}

	return m, nil
}

func (m Model) View() tea.View {
	width := min(max(m.width-6, 54), 100)
	content := m.renderScreen(width - 6)
	header := titleStyle.Render("DeployCrate CE Setup") + "  " + mutedStyle.Render(m.stepLabel())
	body := panelStyle.Width(width).Render(header + "\n\n" + content)
	footer := mutedStyle.Render("ctrl+c cancel")
	if m.screen == screenHandoff {
		footer += mutedStyle.Render("  •  tab move  •  enter select")
	} else if m.screen != screenRunning && m.screen != screenValidating {
		footer += mutedStyle.Render("  •  tab move  •  enter continue")
	}
	view := tea.NewView(lipgloss.NewStyle().Margin(1, 2).Render(body + "\n" + footer))
	for _, input := range m.inputs {
		if input.Focused() {
			view.Cursor = input.Cursor()
			break
		}
	}
	if m.screen == screenHandoff && m.handoffConfirmation.Focused() {
		view.Cursor = m.handoffConfirmation.Cursor()
	}
	return view
}

func (m *Model) setScreen(next screen) {
	m.screen = next
	m.focus = 0
	m.err = nil
	m.inputs = nil
	switch next {
	case screenServer:
		m.inputs = []textinput.Model{
			newInput("ce.example.com", m.config.Domain, false),
			newInput("22", strconv.Itoa(m.config.SSHPort), false),
		}
	case screenAccess:
		m.inputs = []textinput.Model{
			newInput("Server administrator password", m.config.Secrets.ServerAdminPassword, true),
			newInput("Confirm server administrator password", "", true),
			newInput("Optional ordinary owner SSH public key", m.config.OwnerSSHPublicKey, false),
		}
	case screenAdmin:
		m.inputs = []textinput.Model{
			newInput("admin@example.com", m.config.AdminEmail, false),
			newInput("Application admin password", m.config.Secrets.AdminPassword, true),
			newInput("Confirm application admin password", "", true),
		}
	case screenDatabase:
		m.inputs = []textinput.Model{
			newInput("Database host", m.config.Database.Host, false),
			newInput("5432", strconv.Itoa(m.config.Database.Port), false),
			newInput("Database name", m.config.Database.Name, false),
			newInput("Database user", m.config.Database.User, false),
			newInput("Database password", m.config.Secrets.DatabasePassword, true),
			newInput("verify-full", "verify-full", false),
			newInput("Optional CA certificate file path", m.config.Database.TLSCAPath, false),
		}
	case screenStorage:
		m.inputs = []textinput.Model{
			newInput("https://s3.example.com or blank for AWS", m.config.S3.Endpoint, false),
			newInput("Region", m.config.S3.Region, false),
			newInput("Bucket", m.config.S3.Bucket, false),
			newInput("Optional object prefix", m.config.S3.Prefix, false),
			newInput("true or false", strconv.FormatBool(m.config.S3.UsePathStyle), false),
			newInput("Access key ID", m.config.Secrets.S3AccessKeyID, true),
			newInput("Secret access key", m.config.Secrets.S3SecretAccessKey, true),
		}
	case screenServerBackupPolicy:
		m.inputs = []textinput.Model{
			newInput("0 2 * * *", m.config.S3.ServerPolicy.Schedule, false),
			newInput("7", strconv.Itoa(m.config.S3.ServerPolicy.Retention.KeepDaily), false),
			newInput("4", strconv.Itoa(m.config.S3.ServerPolicy.Retention.KeepWeekly), false),
			newInput("6", strconv.Itoa(m.config.S3.ServerPolicy.Retention.KeepMonthly), false),
		}
	case screenDatabaseBackupPolicy:
		m.inputs = []textinput.Model{
			newInput("0 */6 * * *", m.config.S3.DatabasePolicy.Schedule, false),
			newInput("12", strconv.Itoa(m.config.S3.DatabasePolicy.Retention.KeepLast), false),
			newInput("7", strconv.Itoa(m.config.S3.DatabasePolicy.Retention.KeepDaily), false),
			newInput("4", strconv.Itoa(m.config.S3.DatabasePolicy.Retention.KeepWeekly), false),
			newInput("6", strconv.Itoa(m.config.S3.DatabasePolicy.Retention.KeepMonthly), false),
		}
	}
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
}

func newInput(placeholder, value string, password bool) textinput.Model {
	input := textinput.New()
	input.Prompt = "› "
	input.Placeholder = placeholder
	input.CharLimit = 4096
	input.SetWidth(68)
	input.SetValue(value)
	if password {
		input.EchoMode = textinput.EchoPassword
		input.EchoCharacter = '•'
	}
	styles := input.Styles()
	styles.Cursor.Color = colorPink
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(colorPink)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#edf3ff"))
	styles.Blurred.Prompt = mutedStyle
	styles.Blurred.Text = mutedStyle
	input.SetStyles(styles)
	return input
}

func newHandoffConfirmation() textinput.Model {
	input := newInput("CONFIRM", "", false)
	input.CharLimit = len("CONFIRM")
	return input
}

func (m Model) updateForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "tab" || key == "shift+tab" || key == "up" || key == "down" {
		if key == "shift+tab" || key == "up" {
			m.focus--
		} else {
			m.focus++
		}
		if m.focus < 0 {
			m.focus = len(m.inputs)
		}
		if m.focus > len(m.inputs) {
			m.focus = 0
		}
		return m, m.focusInputs()
	}
	if key == "enter" && m.focus == len(m.inputs) {
		if err := m.commitForm(); err != nil {
			m.err = err
			return m, nil
		}
		return m.advanceAfterForm()
	}

	commands := make([]tea.Cmd, len(m.inputs))
	for index := range m.inputs {
		m.inputs[index], commands[index] = m.inputs[index].Update(msg)
	}
	return m, tea.Batch(commands...)
}

func (m *Model) focusInputs() tea.Cmd {
	commands := make([]tea.Cmd, len(m.inputs))
	for index := range m.inputs {
		if index == m.focus {
			commands[index] = m.inputs[index].Focus()
		} else {
			m.inputs[index].Blur()
		}
	}
	return tea.Batch(commands...)
}

func (m *Model) commitForm() error {
	value := func(index int) string { return strings.TrimSpace(m.inputs[index].Value()) }
	switch m.screen {
	case screenServer:
		port, err := strconv.Atoi(value(1))
		if err != nil || port < 1 || port > 65535 {
			return errors.New("SSH port must be between 1 and 65535")
		}
		if value(0) == "" || strings.ContainsAny(value(0), " /:") {
			return errors.New("enter a domain without protocol or path")
		}
		m.config.Domain, m.config.SSHPort = value(0), port
	case screenAccess:
		if value(0) == "" {
			return errors.New("server administrator password is required")
		}
		if value(0) != value(1) {
			return errors.New("server administrator passwords do not match")
		}
		m.config.Secrets.ServerAdminPassword = value(0)
		if value(2) != "" {
			if err := setup.ValidateSSHPublicKey(value(2)); err != nil {
				return err
			}
			m.config.OwnerSSHPublicKey = value(2)
		} else {
			m.config.OwnerSSHPublicKey = ""
		}
		if m.config.Secrets.SSHPrivateKey == "" {
			publicKey, privateKey, err := setup.GenerateSSHKeyPair()
			if err != nil {
				return err
			}
			m.config.SSHPublicKey = publicKey
			m.config.Secrets.SSHPrivateKey = privateKey
		}
		m.config.GeneratedSSHKey = true
	case screenAdmin:
		if _, err := mail.ParseAddress(value(0)); err != nil {
			return errors.New("enter a valid application admin email")
		}
		if len(value(1)) < 8 {
			return errors.New("application admin password must be at least 8 characters")
		}
		if value(1) != value(2) {
			return errors.New("application admin passwords do not match")
		}
		m.config.AdminEmail = value(0)
		m.config.Secrets.AdminPassword = value(1)
	case screenDatabase:
		port, err := strconv.Atoi(value(1))
		if err != nil || port < 1 || port > 65535 {
			return errors.New("database port must be between 1 and 65535")
		}
		if value(0) == "" || value(2) == "" || value(3) == "" || value(4) == "" {
			return errors.New("all database fields are required")
		}
		allowedSSL := map[string]bool{
			"disable":     true,
			"require":     true,
			"verify-ca":   true,
			"verify-full": true,
		}
		if !allowedSSL[value(5)] {
			return errors.New("SSL mode must be disable, require, verify-ca, or verify-full")
		}
		m.config.Database.Host, m.config.Database.Port = value(0), port
		m.config.Database.Name, m.config.Database.User = value(2), value(3)
		m.config.Secrets.DatabasePassword, m.config.Database.SSLMode = value(4), value(5)
		if value(6) != "" {
			if _, err := os.Stat(value(6)); err != nil {
				return fmt.Errorf("read database CA certificate: %w", err)
			}
		}
		m.config.Database.TLSCAPath = value(6)
	case screenStorage:
		pathStyle, err := strconv.ParseBool(value(4))
		if err != nil {
			return errors.New("path-style access must be true or false")
		}
		if value(1) == "" || value(2) == "" || value(5) == "" || value(6) == "" {
			return errors.New("region, bucket, access key, and secret key are required")
		}
		if m.operations.NormalizeObjectStorage == nil {
			return errors.New("object storage normalization is unavailable")
		}
		normalized, err := m.operations.NormalizeObjectStorage(setup.S3Config{
			Provider: m.config.S3.Provider, Endpoint: value(0), Region: value(1),
			Bucket: value(2), Prefix: value(3), UsePathStyle: pathStyle,
		})
		if err != nil {
			return err
		}
		m.config.S3.Endpoint, m.config.S3.Region, m.config.S3.Bucket = normalized.Endpoint, normalized.Region, normalized.Bucket
		m.config.S3.Prefix, m.config.S3.UsePathStyle = normalized.Prefix, normalized.UsePathStyle
		m.config.Secrets.S3AccessKeyID, m.config.Secrets.S3SecretAccessKey = value(5), value(6)
	case screenServerBackupPolicy:
		retention, err := parseRetention(0, value(1), value(2), value(3))
		if err != nil {
			return err
		}
		m.config.S3.ServerPolicy = setup.BackupPolicyConfig{
			Schedule:  value(0),
			Retention: retention,
		}
	case screenDatabaseBackupPolicy:
		keepLast, err := strconv.Atoi(value(1))
		if err != nil || keepLast < 1 {
			return errors.New("database recent retention must be at least 1")
		}
		retention, err := parseRetention(keepLast, value(2), value(3), value(4))
		if err != nil {
			return err
		}
		m.config.S3.DatabasePolicy = setup.BackupPolicyConfig{
			Schedule:  value(0),
			Retention: retention,
		}
	}
	m.err = nil
	return nil
}

func parseRetention(keepLast int, daily, weekly, monthly string) (setup.BackupRetention, error) {
	values := []string{daily, weekly, monthly}
	parsed := make([]int, len(values))
	for index, value := range values {
		count, err := strconv.Atoi(value)
		if err != nil || count < 1 {
			return setup.BackupRetention{}, errors.New(
				"daily, weekly, and monthly retention must be at least 1",
			)
		}
		parsed[index] = count
	}
	return setup.BackupRetention{
		KeepLast: keepLast, KeepDaily: parsed[0], KeepWeekly: parsed[1], KeepMonthly: parsed[2],
	}, nil
}

func (m Model) advanceAfterForm() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenServer:
		m.setScreen(screenAccess)
	case screenAccess:
		m.setScreen(screenAdmin)
	case screenAdmin:
		m.screen = screenDatabaseChoice
		m.choice = 0
		m.inputs = nil
	case screenDatabase:
		m.screen = screenStorageChoice
		m.choice = 0
		m.inputs = nil
	case screenStorage:
		m.setScreen(screenServerBackupPolicy)
	case screenServerBackupPolicy:
		if !m.config.Database.External {
			m.setScreen(screenDatabaseBackupPolicy)
			return m, textinput.Blink
		}
		m.screen = screenValidating
		return m, tea.Batch(
			validateRemoteServices(m.config, m.dryRun, m.operations.ValidateRemoteServices),
			m.activity.Tick,
		)
	case screenDatabaseBackupPolicy:
		m.screen = screenValidating
		return m, tea.Batch(
			validateRemoteServices(m.config, m.dryRun, m.operations.ValidateRemoteServices),
			m.activity.Tick,
		)
	}
	return m, textinput.Blink
}

func (m Model) updateChoice(key string) (tea.Model, tea.Cmd) {
	if key == "up" || key == "down" || key == "left" || key == "right" || key == "tab" {
		m.choice = 1 - m.choice
		return m, nil
	}
	if key != "enter" {
		return m, nil
	}
	if m.screen == screenChannel {
		channel := setup.UpdateChannelStable
		if m.choice == 1 {
			channel = setup.UpdateChannelEdge
		}
		if m.dryRun {
			m.config.UpdateChannel = channel
			m.setScreen(screenServer)
			return m, textinput.Blink
		}
		m.err = nil
		return m, resolveChannelRelease(channel)
	}
	if m.screen == screenDatabaseChoice {
		m.config.Database.External = m.choice == 1
		if m.config.Database.External {
			m.config.Database.SSLMode = "verify-full"
			m.setScreen(screenDatabase)
			return m, textinput.Blink
		}
		m.config.Database = setup.DatabaseConfig{
			External: false,
			Host:     "127.0.0.1",
			Port:     5432,
			Name:     "deploycrate_ce",
			User:     "deploycrate",
			SSLMode:  "disable",
		}
		m.screen = screenStorageChoice
		m.choice = 0
		return m, nil
	}
	if m.screen == screenStorageProvider {
		m.config.S3.Provider = "s3"
		if m.choice == 1 {
			m.config.S3.Provider = "r2"
			m.config.S3.Region = "auto"
			m.config.S3.UsePathStyle = true
		}
		m.setScreen(screenStorage)
		return m, textinput.Blink
	}
	m.config.S3.Enabled = m.choice == 1
	if m.config.S3.Enabled {
		m.screen = screenStorageProvider
		m.choice = 0
		return m, nil
	}
	m.screen = screenValidating
	return m, tea.Batch(
		validateRemoteServices(m.config, m.dryRun, m.operations.ValidateRemoteServices),
		m.activity.Tick,
	)
}

func resolveChannelRelease(channel string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		version, err := setup.ResolveReleaseVersion(ctx, channel)
		return channelReleaseMsg{channel: channel, version: version, err: err}
	}
}

func validateRemoteServices(
	cfg setup.Config,
	dryRun bool,
	validate func(context.Context, setup.Config) error,
) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			return validationMsg{}
		}
		if validate == nil {
			return validationMsg{err: errors.New("remote service validation is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := validate(ctx, cfg); err != nil {
			return validationMsg{err: err}
		}
		return validationMsg{}
	}
}

func (m Model) startInstall() (tea.Model, tea.Cmd) {
	if err := m.config.Validate(m.operations.NormalizeObjectStorage); err != nil {
		m.err = err
		return m, nil
	}
	m.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	stream := make(chan setup.Event, 8)
	m.eventStream = stream
	m.screen = screenRunning
	m.events = nil
	go func(cfg setup.Config) {
		if !m.dryRun {
			normalized, err := setup.SaveConfig(cfg)
			if err != nil {
				stream <- setup.Event{Kind: setup.EventFailed, StepID: installerConfigStepID, Description: "Persist installer configuration", Err: err}
				close(stream)
				return
			}
			cfg = normalized
			stream <- setup.Event{Kind: setup.EventCompleted, StepID: installerConfigStepID, Description: "Persist installer configuration"}
		}
		runner := setup.NewRunner(cfg, m.dryRun, m.operations)
		err := runner.Execute(ctx, cfg, func(event setup.Event) { stream <- event })
		if err != nil {
			stream <- setup.Event{Kind: setup.EventFailed, Description: "Installation stopped", Err: err}
		}
		close(stream)
	}(m.config)
	return m, tea.Batch(waitForEvent(stream), m.activity.Tick)
}

func waitForEvent(stream <-chan setup.Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-stream
		if !ok {
			return runClosedMsg{}
		}
		return runEventMsg{event: event}
	}
}

func (m Model) updateHandoff(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.rebootStarted {
		return m, nil
	}

	key := message.String()
	if key == "tab" || key == "shift+tab" || key == "up" || key == "down" {
		if m.handoffFocus == 0 {
			m.handoffFocus = 1
			m.handoffConfirmation.Focus()
			return m, textinput.Blink
		}
		m.handoffFocus = 0
		m.handoffConfirmation.Blur()
		return m, nil
	}

	if key == "enter" && m.handoffFocus == 0 {
		m.err = nil
		m.copyRequested = true
		return m, tea.SetClipboard(m.handoffDetails())
	}

	if key == "enter" {
		if strings.TrimSpace(m.handoffConfirmation.Value()) != "CONFIRM" {
			m.err = errors.New(
				"type CONFIRM exactly to remove transient secrets and bootstrap binaries, then reboot",
			)
			return m, nil
		}
		m.err = nil
		m.rebootStarted = true
		return m, reboot(m.dryRun)
	}
	if m.handoffFocus == 0 {
		return m, nil
	}
	m.err = nil
	var command tea.Cmd
	m.handoffConfirmation, command = m.handoffConfirmation.Update(message)
	return m, command
}

func reboot(dryRun bool) tea.Cmd {
	return func() tea.Msg {
		if dryRun {
			return rebootMsg{}
		}
		if err := setup.CompleteCredentialHandoff(); err != nil {
			return rebootMsg{err: err}
		}
		return rebootMsg{err: exec.Command("systemctl", "reboot").Run()}
	}
}

func (m Model) renderScreen(width int) string {
	var content string
	switch m.screen {
	case screenWelcome:
		content = labelStyle.Render("Ready to configure this VPS") + "\n\n" +
			fmt.Sprintf(
				"Detected %s %s on %s\n%d MB memory available · %d MB disk free\n\n",
				m.host.Distribution,
				m.host.Version,
				m.host.Architecture,
				m.host.MemoryMB,
				m.host.DiskFreeMB,
			) +
			warnStyle.Render(
				"The admin and deploycrate users receive passwordless sudo and Docker access; deploycrate remains non-login.",
			) + "\n\nPress enter to begin."
	case screenChannel:
		content = m.renderChoice(
			"Which release channel should this installation follow?",
			[]string{"Stable — recommended releases", "Edge — latest successful master build"},
		) + "\n" + warnStyle.Render(
			"Edge receives features sooner and may be less reliable.",
		)
	case screenServer:
		content = m.renderForm([]string{"Public domain", "SSH port"}) + "\n\n" +
			warnStyle.Render("Required DNS records before continuing") + "\n" +
			m.renderDNSRecords(m.serverScreenDomain()) + "\n" +
			mutedStyle.Render("Use DNS-only mode if your provider offers HTTP proxying.")
	case screenAccess:
		content = m.renderForm(
			[]string{
				"Server administrator password",
				"Confirm password",
				"Ordinary owner SSH public key",
			},
		) + "\n" + mutedStyle.Render(
			"At least 12 characters is recommended, not required. A unique Ed25519 admin key is always generated.",
		)
	case screenAdmin:
		content = m.renderForm(
			[]string{"Application admin email", "Application admin password", "Confirm password"},
		)
	case screenDatabaseChoice:
		content = m.renderChoice(
			"Where should PostgreSQL run?",
			[]string{"Local Docker PostgreSQL", "Externally hosted PostgreSQL"},
		)
	case screenDatabase:
		content = m.renderForm(
			[]string{
				"Host",
				"Port",
				"Database",
				"User",
				"Password",
				"SSL mode",
				"CA certificate path",
			},
		)
	case screenStorageChoice:
		content = m.renderChoice(
			"Configure S3-compatible storage for backups?",
			[]string{"Not now", "Yes, store destination details"},
		)
	case screenStorageProvider:
		content = m.renderChoice(
			"Which object storage provider should backups use?",
			[]string{"Generic S3-compatible storage", "Cloudflare R2"},
		)
	case screenStorage:
		content = m.renderForm(
			[]string{
				"Endpoint", "Region", "Bucket", "Prefix", "Path-style access",
				"Access key ID", "Secret access key",
			},
		)
	case screenServerBackupPolicy:
		content = m.renderForm(
			[]string{
				"Server cron schedule", "Daily recovery points", "Weekly recovery points",
				"Monthly recovery points",
			},
		)
	case screenDatabaseBackupPolicy:
		content = m.renderForm(
			[]string{
				"Database cron schedule", "Recent recovery points", "Daily recovery points",
				"Weekly recovery points", "Monthly recovery points",
			},
		)
	case screenValidating:
		content = m.activity.View() + " " + labelStyle.Render(
			"Validating configuration",
		) + "\n\n" + mutedStyle.Render(
			"Checking database connectivity and object-storage write, head, get, list, and delete capabilities.",
		)
	case screenReview:
		content = m.renderReview()
	case screenRunning:
		content = m.renderProgress()
	case screenHandoff:
		content = m.renderHandoff()
	}
	if m.err != nil {
		content += "\n\n" + errorStyle.Render("Error: "+m.err.Error())
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}

func (m Model) renderForm(labels []string) string {
	var b strings.Builder
	for index, input := range m.inputs {
		b.WriteString(labelStyle.Render(labels[index]))
		b.WriteRune('\n')
		b.WriteString(input.View())
		b.WriteString("\n\n")
	}
	button := mutedStyle.Render("[ Continue ]")
	if m.focus == len(m.inputs) {
		button = lipgloss.NewStyle().Bold(true).Foreground(colorPink).Render("[ Continue ]")
	}
	b.WriteString(button)
	return b.String()
}

func (m Model) renderChoice(question string, options []string) string {
	var b strings.Builder
	b.WriteString(labelStyle.Render(question))
	b.WriteString("\n\n")
	for index, option := range options {
		marker := "○"
		style := mutedStyle
		if index == m.choice {
			marker = "●"
			style = lipgloss.NewStyle().Bold(true).Foreground(colorPink)
		}
		b.WriteString(style.Render(marker + " " + option))
		b.WriteRune('\n')
	}
	return b.String()
}

func (m Model) renderReview() string {
	database := "Local PostgreSQL 17 container"
	if m.config.Database.External {
		database = fmt.Sprintf(
			"External PostgreSQL at %s:%d (%s)",
			m.config.Database.Host,
			m.config.Database.Port,
			m.config.Database.SSLMode,
		)
	}
	storage := "Not configured"
	if m.config.S3.Enabled {
		hostname := "AWS S3"
		if endpoint, err := url.Parse(
			m.config.S3.Endpoint,
		); err == nil &&
			endpoint.Hostname() != "" {
			hostname = endpoint.Hostname()
		}
		storage = fmt.Sprintf(
			"%s via %s, %s / %s",
			strings.ToUpper(m.config.S3.Provider),
			hostname,
			m.config.S3.Region,
			m.config.S3.Bucket,
		)
		if m.config.S3.Prefix != "" {
			storage += "/" + m.config.S3.Prefix
		}
	}
	rows := []string{
		labelStyle.Render("Server") + "       https://" + m.config.Domain,
		labelStyle.Render("Updates") + "      " + strings.ToUpper(m.config.UpdateChannel) + " channel",
		labelStyle.Render(
			"App DNS A",
		) + "    " + m.config.Domain + " -> " + m.config.PublicIPv4 + " (DNS only)",
		labelStyle.Render(
			"Registry DNS A",
		) + " " + registryDomain(
			m.config.Domain,
		) + " -> " + m.config.PublicIPv4 + " (DNS only)",
		labelStyle.Render(
			"SSH",
		) + fmt.Sprintf(
			"          %s@%s:%d",
			m.config.AdminUser,
			m.config.Domain,
			m.config.SSHPort,
		),
		labelStyle.Render("Admin SSH key") + " " + setup.SSHFingerprint(m.config.SSHPublicKey),
		labelStyle.Render("Database") + "     " + database,
		labelStyle.Render("Backups") + "      " + storage,
		labelStyle.Render("App admin") + "     " + m.config.AdminEmail,
	}
	if m.config.OwnerSSHPublicKey != "" {
		rows = append(
			rows,
			labelStyle.Render("Owner SSH key")+" "+setup.SSHFingerprint(m.config.OwnerSSHPublicKey),
		)
	}
	if m.config.S3.Enabled {
		serverRetention := m.config.S3.ServerPolicy.Retention
		rows = append(rows, labelStyle.Render("Server backup")+fmt.Sprintf(
			" %s, %d daily / %d weekly / %d monthly",
			m.config.S3.ServerPolicy.Schedule,
			serverRetention.KeepDaily,
			serverRetention.KeepWeekly,
			serverRetention.KeepMonthly,
		))
		if !m.config.Database.External {
			databaseRetention := m.config.S3.DatabasePolicy.Retention
			rows = append(rows, labelStyle.Render("Database backup")+fmt.Sprintf(
				" %s, %d recent / %d daily / %d weekly / %d monthly",
				m.config.S3.DatabasePolicy.Schedule,
				databaseRetention.KeepLast,
				databaseRetention.KeepDaily,
				databaseRetention.KeepWeekly,
				databaseRetention.KeepMonthly,
			))
		}
	}
	return strings.Join(
		rows,
		"\n",
	) + "\n\n" + warnStyle.Render(
		"This will configure the host and later disable root SSH access.",
	) + "\n\nPress enter to install."
}

func (m Model) renderProgress() string {
	var b strings.Builder
	if m.err == nil {
		b.WriteString(m.activity.View())
		b.WriteString(" ")
		b.WriteString(labelStyle.Render(m.progressStatus()))
		b.WriteString("\n\n")
	}
	start := max(0, len(m.events)-12)
	for _, event := range m.events[start:] {
		symbol := "·"
		style := mutedStyle
		switch event.Kind {
		case setup.EventStarted:
			symbol, style = "◉", lipgloss.NewStyle().Foreground(colorCyan)
		case setup.EventCompleted:
			symbol, style = "✓", okStyle
		case setup.EventSkipped:
			symbol, style = "✓", mutedStyle
		case setup.EventFailed:
			symbol, style = "×", errorStyle
		}
		line := event.Description
		if event.Kind == setup.EventLog {
			line = event.Line
		}
		if line != "" {
			b.WriteString(style.Render(symbol + " " + line))
			b.WriteRune('\n')
		}
	}
	if m.err != nil {
		b.WriteString("\n")
		if m.configSaved {
			b.WriteString(
				warnStyle.Render("Fix the reported issue, then run: sudo bootstrap resume"),
			)
		} else {
			b.WriteString(warnStyle.Render("Fix the reported issue, then retry the installation."))
		}
	}
	return b.String()
}

func (m Model) progressStatus() string {
	if len(m.events) == 0 {
		if m.dryRun {
			return "Preparing installation..."
		}
		return "Saving installer configuration..."
	}
	for _, event := range slices.Backward(m.events) {

		switch event.Kind {
		case setup.EventStarted:
			return event.Description + "..."
		case setup.EventCompleted, setup.EventSkipped:
			return "Preparing the next setup step..."
		case setup.EventFailed:
			return "Installation stopped"
		}
	}
	return "Installation is running..."
}

func (m Model) renderHandoff() string {
	var b strings.Builder
	b.WriteString(okStyle.Bold(true).Render("Setup complete. Final confirmation required."))
	b.WriteString("\n\n")
	b.WriteString(labelStyle.Render("Application URL") + "  https://" + m.config.Domain + "\n")
	b.WriteString(
		labelStyle.Render(
			"App DNS A",
		) + "        " + m.config.Domain + " -> " + m.config.PublicIPv4 + " (DNS only)\n",
	)
	b.WriteString(
		labelStyle.Render(
			"Registry DNS A",
		) + "   " + registryDomain(
			m.config.Domain,
		) + " -> " + m.config.PublicIPv4 + " (DNS only)\n",
	)
	b.WriteString(labelStyle.Render("Server admin") + "     " + m.config.AdminUser + "\n")
	b.WriteString(
		labelStyle.Render("Admin password") + "   " + m.config.Secrets.ServerAdminPassword + "\n",
	)
	b.WriteString(
		labelStyle.Render(
			"SSH command",
		) + fmt.Sprintf(
			"       ssh -p %d %s@%s\n",
			m.config.SSHPort,
			m.config.AdminUser,
			m.config.Domain,
		),
	)
	b.WriteString(
		labelStyle.Render(
			"SSH fingerprint",
		) + "   " + setup.SSHFingerprint(
			m.config.SSHPublicKey,
		) + "\n",
	)
	if m.config.GeneratedSSHKey {
		b.WriteString("\n" + warnStyle.Render("Generated SSH private key, shown once") + "\n")
		b.WriteString(m.config.Secrets.SSHPrivateKey + "\n")
	}
	recoveryChecksum, checksumErr := setup.SSHCARecoveryBundleChecksum()
	if checksumErr == nil && !m.credentialsVerified {
		b.WriteString("\n" + warnStyle.Render("SSH CA recovery material, shown once") + "\n")
		b.WriteString(
			labelStyle.Render("Bundle") + "            " + setup.SSHCARecoveryBundlePath + "\n",
		)
		b.WriteString(labelStyle.Render("SHA-256") + "           " + recoveryChecksum + "\n")
		b.WriteString(
			labelStyle.Render(
				"Age passphrase",
			) + "    " + m.config.Secrets.SSHCARecoveryPassphrase + "\n",
		)
		b.WriteString("Copy the bundle off this server and store the passphrase separately.\n")
	}
	if m.config.S3.Enabled && !m.credentialsVerified {
		b.WriteString("\n" + warnStyle.Render("Backup recovery material, shown once") + "\n")
		b.WriteString(
			labelStyle.Render("Restic password") + "    " + m.config.Secrets.ResticPassword + "\n",
		)
		b.WriteString(
			labelStyle.Render("Age identity") + "       " + m.config.Secrets.AgeIdentity + "\n",
		)
		b.WriteString(
			"Store both values off-server. Losing them makes the corresponding backups unusable.\n",
		)
	}
	b.WriteString(labelStyle.Render("App admin") + "       " + m.config.AdminEmail + "\n")
	b.WriteString(
		labelStyle.Render("App password") + "     " + m.config.Secrets.AdminPassword + "\n\n",
	)
	copyButton := mutedStyle.Render("[ Copy details ]")
	if m.handoffFocus == 0 {
		copyButton = lipgloss.NewStyle().Bold(true).Foreground(colorPink).Render("[ Copy details ]")
		copyButton += "  " + labelStyle.Render("Press Enter to copy all details.")
	}
	b.WriteString(copyButton + "\n")
	if m.copyRequested {
		b.WriteString(okStyle.Render("Copy request sent to your terminal clipboard.") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Final step") + "\n")
	if m.credentialsVerified {
		b.WriteString(
			"Your previous confirmation was recorded, but secret cleanup did not finish.\n",
		)
	} else {
		b.WriteString(
			"Copy every credential above, then verify the SSH command from a second terminal.\n",
		)
	}
	b.WriteString(
		warnStyle.Render(
			"Type CONFIRM to acknowledge recovery handoff, remove transient secrets, and reboot.",
		) + "\n\n",
	)
	b.WriteString(m.handoffConfirmation.View())
	return b.String()
}

func (m Model) handoffDetails() string {
	var b strings.Builder
	b.WriteString("DeployCrate CE setup details\n\n")
	b.WriteString("Application URL: https://" + m.config.Domain + "\n")
	b.WriteString(
		"Application DNS A record: " + m.config.Domain + " -> " + m.config.PublicIPv4 + " (DNS only)\n",
	)
	b.WriteString(
		"Registry DNS A record: " + registryDomain(
			m.config.Domain,
		) + " -> " + m.config.PublicIPv4 + " (DNS only)\n",
	)
	b.WriteString("Server administrator: " + m.config.AdminUser + "\n")
	b.WriteString("Server administrator password: " + m.config.Secrets.ServerAdminPassword + "\n")
	b.WriteString(
		fmt.Sprintf(
			"SSH command: ssh -p %d %s@%s\n",
			m.config.SSHPort,
			m.config.AdminUser,
			m.config.Domain,
		),
	)
	b.WriteString("SSH fingerprint: " + setup.SSHFingerprint(m.config.SSHPublicKey) + "\n")
	if m.config.GeneratedSSHKey {
		b.WriteString("\nGenerated SSH private key:\n")
		b.WriteString(m.config.Secrets.SSHPrivateKey + "\n")
	}
	if recoveryChecksum, err := setup.SSHCARecoveryBundleChecksum(); err == nil &&
		!m.credentialsVerified {
		b.WriteString("\nSSH CA recovery bundle: " + setup.SSHCARecoveryBundlePath + "\n")
		b.WriteString("SSH CA recovery SHA-256: " + recoveryChecksum + "\n")
		b.WriteString(
			"SSH CA recovery age passphrase: " + m.config.Secrets.SSHCARecoveryPassphrase + "\n",
		)
		b.WriteString("Store the bundle off-server and keep this passphrase separately.\n")
	}
	if m.config.S3.Enabled && !m.credentialsVerified {
		b.WriteString("\nBackup Restic password: " + m.config.Secrets.ResticPassword + "\n")
		b.WriteString("Backup age identity: " + m.config.Secrets.AgeIdentity + "\n")
		b.WriteString("Store both backup recovery values off-server.\n")
	}
	b.WriteString("\nApp admin: " + m.config.AdminEmail + "\n")
	b.WriteString("Admin password: " + m.config.Secrets.AdminPassword + "\n")
	return b.String()
}

func (m Model) serverScreenDomain() string {
	if len(m.inputs) == 0 {
		return m.config.Domain
	}
	domain := strings.TrimSpace(m.inputs[0].Value())
	if domain == "" {
		return "<your domain>"
	}
	return domain
}

func (m Model) renderDNSRecords(domain string) string {
	return labelStyle.Render("A") + " " + domain + " -> " + m.config.PublicIPv4 + "\n" +
		labelStyle.Render("A") + " " + registryDomain(domain) + " -> " + m.config.PublicIPv4
}

func registryDomain(domain string) string {
	return "registry-" + domain
}

func (m Model) stepLabel() string {
	labels := map[screen]string{
		screenWelcome:              "welcome",
		screenChannel:              "release channel",
		screenServer:               "server",
		screenAccess:               "access",
		screenAdmin:                "admin",
		screenDatabaseChoice:       "database",
		screenDatabase:             "database",
		screenStorageChoice:        "backups",
		screenStorageProvider:      "backups",
		screenStorage:              "backups",
		screenServerBackupPolicy:   "backup policy",
		screenDatabaseBackupPolicy: "backup policy",
		screenValidating:           "validation",
		screenReview:               "review",
		screenRunning:              "installing",
		screenHandoff:              "handoff",
	}
	return labels[m.screen]
}
