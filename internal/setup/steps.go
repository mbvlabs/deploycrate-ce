package setup

import "context"

// stubStep preserves the orchestration boundary without mutating the host.
// Each stub is intended to be replaced when the corresponding CE capability
// and its public contract exist in the main application.
type stubStep struct {
	id          string
	description string
}

func (s stubStep) ID() string             { return s.id }
func (s stubStep) Describe(Config) string { return s.description }

func (s stubStep) Check(context.Context, Config, Runtime) (CheckResult, error) {
	return CheckResult{}, nil
}

func (s stubStep) Apply(_ context.Context, _ Config, _ Runtime, report Reporter) error {
	report(Event{
		Kind:   EventLog,
		StepID: s.id,
		Line:   "stub only: no server changes were applied",
	})
	return nil
}

func DefaultSteps() []Step {
	return []Step{
		stubStep{id: "host-packages", description: "Install baseline host packages"},
		stubStep{id: "deploycrate-user", description: "Create unrestricted deploycrate user and SSH access"},
		stubStep{id: "host-safety", description: "Configure journald, fail2ban, swap, and pressure guards"},
		stubStep{id: "docker", description: "Install and configure Docker Engine"},
		stubStep{id: "database", description: "Configure local or external PostgreSQL"},
		stubStep{id: "application-config", description: "Write protected application configuration"},
		stubStep{id: "database-migrations", description: "Apply embedded database migrations"},
		stubStep{id: "application-admin", description: "Create or update the application administrator"},
		stubStep{id: "backup-destination", description: "Configure S3-compatible backup storage"},
		stubStep{id: "application-service", description: "Install the DeployCrate CE service and Caddy"},
		stubStep{id: "health-check", description: "Verify the application health endpoint"},
		stubStep{id: "ssh-hardening", description: "Harden SSH and configure the firewall"},
	}
}

func NewRunner(cfg Config, _ bool) Runner {
	return Runner{
		Steps: DefaultSteps(),
		Store: NewStateStore(),
		Run: Runtime{
			DryRun: true,
			Shell:  NewShell(true, cfg.SecretValues()),
		},
	}
}
