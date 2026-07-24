package ui

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"deploycrate-ce/internal/setup"
	"deploycrate-ce/internal/storage"
)

type screen int

const (
	screenWelcome screen = iota
	screenServer
	screenAccess
	screenAdmin
	screenDatabaseChoice
	screenDatabase
	screenStorageChoice
	screenStorage
	screenValidating
	screenReview
	screenRunning
	screenHandoff
)

type runEventMsg struct{ event setup.Event }
type runClosedMsg struct{}
type validationMsg struct{ err error }
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
	rebootStarted       bool
	activity            spinner.Model
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
	panelStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Padding(1, 2)
	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	labelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	errorStyle = lipgloss.NewStyle().Foreground(colorRed)
	okStyle    = lipgloss.NewStyle().Foreground(colorGreen)
	warnStyle  = lipgloss.NewStyle().Foreground(colorAmber)
)

func NewModel(cfg setup.Config, host setup.HostInfo, dryRun bool) Model {
	activity := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(colorPink)),
	)
	return Model{config: cfg, host: host, dryRun: dryRun, screen: screenWelcome, width: 88, height: 30, activity: activity}
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
	if m.screen == screenHandoff {
		return textinput.Blink
	}
	return nil
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
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
			return m, textinput.Blink
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
		if m.screen == screenDatabaseChoice || m.screen == screenStorageChoice {
			return m.updateChoice(key)
		}
		if m.screen == screenWelcome || m.screen == screenReview {
			if key == "enter" {
				if m.screen == screenWelcome {
					m.setScreen(screenServer)
					return m, textinput.Blink
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
	if m.screen != screenRunning && m.screen != screenValidating && m.screen != screenHandoff {
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
			newInput("Linux password", m.config.Secrets.LinuxPassword, true),
			newInput("Confirm Linux password", "", true),
			newInput("ssh-ed25519 AAAA... or blank to generate", m.config.SSHPublicKey, false),
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
			newInput("Access key ID", m.config.Secrets.S3AccessKeyID, true),
			newInput("Secret access key", m.config.Secrets.S3SecretAccessKey, true),
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
	input.Focus()
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
		if len(value(0)) < 12 {
			return errors.New("Linux password must be at least 12 characters")
		}
		if value(0) != value(1) {
			return errors.New("Linux passwords do not match")
		}
		m.config.Secrets.LinuxPassword = value(0)
		if value(2) == "" {
			publicKey, privateKey, err := setup.GenerateSSHKeyPair()
			if err != nil {
				return err
			}
			m.config.SSHPublicKey = publicKey
			m.config.Secrets.SSHPrivateKey = privateKey
			m.config.GeneratedSSHKey = true
		} else {
			if err := setup.ValidateSSHPublicKey(value(2)); err != nil {
				return err
			}
			m.config.SSHPublicKey = value(2)
			m.config.Secrets.SSHPrivateKey = ""
			m.config.GeneratedSSHKey = false
		}
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
		allowedSSL := map[string]bool{"disable": true, "require": true, "verify-ca": true, "verify-full": true}
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
		if value(1) == "" || value(2) == "" || value(3) == "" || value(4) == "" {
			return errors.New("region, bucket, access key, and secret key are required")
		}
		m.config.S3.Endpoint, m.config.S3.Region, m.config.S3.Bucket = value(0), value(1), value(2)
		m.config.Secrets.S3AccessKeyID, m.config.Secrets.S3SecretAccessKey = value(3), value(4)
	}
	m.err = nil
	return nil
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
		m.screen = screenValidating
		return m, tea.Batch(validateRemoteServices(m.config, m.dryRun), m.activity.Tick)
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
	if m.screen == screenDatabaseChoice {
		m.config.Database.External = m.choice == 1
		if m.config.Database.External {
			m.config.Database.SSLMode = "verify-full"
			m.setScreen(screenDatabase)
			return m, textinput.Blink
		}
		m.config.Database = setup.DatabaseConfig{External: false, Host: "127.0.0.1", Port: 5432, Name: "deploycrate_ce", User: "deploycrate", SSLMode: "disable"}
		m.screen = screenStorageChoice
		m.choice = 0
		return m, nil
	}
	m.config.S3.Enabled = m.choice == 1
	if m.config.S3.Enabled {
		m.setScreen(screenStorage)
		return m, textinput.Blink
	}
	m.screen = screenValidating
	return m, tea.Batch(validateRemoteServices(m.config, m.dryRun), m.activity.Tick)
}

func validateRemoteServices(cfg setup.Config, dryRun bool) tea.Cmd {
	return func() tea.Msg {
		if dryRun || !cfg.Database.External {
			return validationMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		db, err := storage.NewPostgres(ctx, cfg.DatabaseURL())
		if err != nil {
			return validationMsg{err: fmt.Errorf("external database validation failed: %w", err)}
		}
		if err := db.Close(); err != nil {
			return validationMsg{err: fmt.Errorf("close external database validation connection: %w", err)}
		}
		return validationMsg{}
	}
}

func (m Model) startInstall() (tea.Model, tea.Cmd) {
	if err := m.config.Validate(); err != nil {
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
		runner := setup.NewRunner(cfg, m.dryRun)
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
	if message.String() == "enter" {
		if strings.TrimSpace(m.handoffConfirmation.Value()) != "CONFIRM" {
			m.err = errors.New("type CONFIRM exactly to remove transient secrets and reboot")
			return m, nil
		}
		m.err = nil
		m.rebootStarted = true
		return m, reboot(m.dryRun)
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
			fmt.Sprintf("Detected %s %s on %s\n%d MB memory available · %d MB disk free\n\n", m.host.Distribution, m.host.Version, m.host.Architecture, m.host.MemoryMB, m.host.DiskFreeMB) +
			warnStyle.Render("The deploycrate user will receive unrestricted passwordless sudo and Docker access.") + "\n\nPress enter to begin."
	case screenServer:
		content = m.renderForm([]string{"Public domain", "SSH port"})
	case screenAccess:
		content = m.renderForm([]string{"Linux password", "Confirm password", "Existing SSH public key"}) + "\n" + mutedStyle.Render("Leave the SSH key blank to generate an Ed25519 key pair for one-time handoff.")
	case screenAdmin:
		content = m.renderForm([]string{"Application admin email", "Application admin password", "Confirm password"})
	case screenDatabaseChoice:
		content = m.renderChoice("Where should PostgreSQL run?", []string{"Local Docker PostgreSQL", "Externally hosted PostgreSQL"})
	case screenDatabase:
		content = m.renderForm([]string{"Host", "Port", "Database", "User", "Password", "SSL mode", "CA certificate path"})
	case screenStorageChoice:
		content = m.renderChoice("Configure S3-compatible storage for backups?", []string{"Not now", "Yes, store destination details"})
	case screenStorage:
		content = m.renderForm([]string{"Endpoint", "Region", "Bucket", "Access key ID", "Secret access key"})
	case screenValidating:
		content = m.activity.View() + " " + labelStyle.Render("Validating configuration") + "\n\n" + mutedStyle.Render("Checking external database connectivity. S3 validation is pending the backup adapter.")
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
		database = fmt.Sprintf("External PostgreSQL at %s:%d (%s)", m.config.Database.Host, m.config.Database.Port, m.config.Database.SSLMode)
	}
	storage := "Not configured"
	if m.config.S3.Enabled {
		storage = fmt.Sprintf("%s / %s", m.config.S3.Region, m.config.S3.Bucket)
	}
	rows := []string{
		labelStyle.Render("Server") + "       https://" + m.config.Domain,
		labelStyle.Render("SSH") + fmt.Sprintf("          deploycrate@%s:%d", m.config.Domain, m.config.SSHPort),
		labelStyle.Render("SSH key") + "      " + setup.SSHFingerprint(m.config.SSHPublicKey),
		labelStyle.Render("Database") + "     " + database,
		labelStyle.Render("Backups") + "      " + storage,
		labelStyle.Render("App admin") + "     " + m.config.AdminEmail,
	}
	return strings.Join(rows, "\n") + "\n\n" + warnStyle.Render("This will configure the host and later disable root SSH access.") + "\n\nPress enter to install."
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
			b.WriteString(warnStyle.Render("Fix the reported issue, then run: sudo deploycrate resume"))
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
	for index := len(m.events) - 1; index >= 0; index-- {
		event := m.events[index]
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
	b.WriteString(labelStyle.Render("Linux user") + "      deploycrate\n")
	b.WriteString(labelStyle.Render("Linux password") + "  " + m.config.Secrets.LinuxPassword + "\n")
	b.WriteString(labelStyle.Render("SSH command") + fmt.Sprintf("       ssh -p %d deploycrate@%s\n", m.config.SSHPort, m.config.Domain))
	b.WriteString(labelStyle.Render("SSH fingerprint") + "   " + setup.SSHFingerprint(m.config.SSHPublicKey) + "\n")
	if m.config.GeneratedSSHKey {
		b.WriteString("\n" + warnStyle.Render("Generated SSH private key, shown once") + "\n")
		b.WriteString(m.config.Secrets.SSHPrivateKey + "\n")
	}
	b.WriteString(labelStyle.Render("App admin") + "       " + m.config.AdminEmail + "\n")
	b.WriteString(labelStyle.Render("Admin password") + "   " + m.config.Secrets.AdminPassword + "\n\n")
	b.WriteString(labelStyle.Render("Final step") + "\n")
	if m.credentialsVerified {
		b.WriteString("Your previous confirmation was recorded, but secret cleanup did not finish.\n")
	} else {
		b.WriteString("Copy every credential above, then verify the SSH command from a second terminal.\n")
	}
	b.WriteString(warnStyle.Render("Type CONFIRM and press enter to permanently remove transient secrets and reboot.") + "\n\n")
	b.WriteString(m.handoffConfirmation.View())
	return b.String()
}

func (m Model) stepLabel() string {
	labels := map[screen]string{
		screenWelcome: "welcome", screenServer: "server", screenAccess: "access", screenAdmin: "admin",
		screenDatabaseChoice: "database", screenDatabase: "database", screenStorageChoice: "backups",
		screenStorage: "backups", screenValidating: "validation", screenReview: "review",
		screenRunning: "installing", screenHandoff: "handoff",
	}
	return labels[m.screen]
}
