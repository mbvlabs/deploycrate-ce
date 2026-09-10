package models_test

import (
	"slices"
	"testing"

	"deploycrate-ce/models"
)

func TestValidateEnvironmentProcessFormationAcceptsWebAndService(t *testing.T) {
	t.Parallel()

	webPort := int32(8080)
	servicePort := int32(13714)
	command := "ssr-worker"
	_, err := models.ValidateEnvironmentProcessFormation([]models.EnvironmentProcessInput{
		{
			Name:          "web",
			Kind:          models.EnvironmentProcessWeb,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &webPort,
		},
		{
			Name:          "ssr-worker",
			Kind:          models.EnvironmentProcessService,
			Command:       &command,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &servicePort,
		},
	})
	if err != nil {
		t.Fatalf("ValidateEnvironmentProcessFormation() error = %v", err)
	}
}

func TestValidateEnvironmentProcessFormationRejectsServiceWithoutPort(t *testing.T) {
	t.Parallel()

	webPort := int32(8080)
	command := "ssr-worker"
	_, err := models.ValidateEnvironmentProcessFormation([]models.EnvironmentProcessInput{
		{
			Name:          "web",
			Kind:          models.EnvironmentProcessWeb,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &webPort,
		},
		{
			Name:      "ssr-worker",
			Kind:      models.EnvironmentProcessService,
			Command:   &command,
			Arguments: []string{},
			Replicas:  1,
		},
	})
	if err == nil {
		t.Fatal("ValidateEnvironmentProcessFormation() accepted a service without a container port")
	}
}

func TestValidateEnvironmentProcessFormationRejectsSecondWeb(t *testing.T) {
	t.Parallel()

	port := int32(8080)
	_, err := models.ValidateEnvironmentProcessFormation([]models.EnvironmentProcessInput{
		{
			Name:          "web",
			Kind:          models.EnvironmentProcessWeb,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &port,
		},
		{
			Name:          "web",
			Kind:          models.EnvironmentProcessWeb,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &port,
		},
	})
	if err == nil {
		t.Fatal("ValidateEnvironmentProcessFormation() accepted two web processes")
	}
}

func TestValidateEnvironmentProcessFormationRejectsWorkerPort(t *testing.T) {
	t.Parallel()

	webPort := int32(8080)
	workerPort := int32(13714)
	command := "queue"
	_, err := models.ValidateEnvironmentProcessFormation([]models.EnvironmentProcessInput{
		{
			Name:          "web",
			Kind:          models.EnvironmentProcessWeb,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &webPort,
		},
		{
			Name:          "worker-1",
			Kind:          models.EnvironmentProcessWorker,
			Command:       &command,
			Arguments:     []string{},
			Replicas:      1,
			ContainerPort: &workerPort,
		},
	})
	if err == nil {
		t.Fatal("ValidateEnvironmentProcessFormation() accepted a worker with a container port")
	}
}

func TestValidateEnvironmentProcessFormationRejectsReservedServiceNames(t *testing.T) {
	t.Parallel()

	webPort := int32(8080)
	servicePort := int32(13714)
	command := "ssr-worker"
	for _, name := range []string{"web", "release"} {
		_, err := models.ValidateEnvironmentProcessFormation([]models.EnvironmentProcessInput{
			{
				Name:          "web",
				Kind:          models.EnvironmentProcessWeb,
				Arguments:     []string{},
				Replicas:      1,
				ContainerPort: &webPort,
			},
			{
				Name:          name,
				Kind:          models.EnvironmentProcessService,
				Command:       &command,
				Arguments:     []string{},
				Replicas:      1,
				ContainerPort: &servicePort,
			},
		})
		if err == nil {
			t.Fatalf("ValidateEnvironmentProcessFormation() accepted a service named %q", name)
		}
	}
}

func TestServiceKindIsLongRunningAndNotIngress(t *testing.T) {
	t.Parallel()

	if !models.IsLongRunningProcessKind(models.EnvironmentProcessService) {
		t.Fatal("service must start as a long-running Deployment Instance")
	}
	if !models.IsLongRunningProcessKind(models.EnvironmentProcessWeb) {
		t.Fatal("web must start as a long-running Deployment Instance")
	}
	if models.IsIngressProcessKind(models.EnvironmentProcessService) {
		t.Fatal("service must not be a Caddy backend or host-published process")
	}
	if !models.IsIngressProcessKind(models.EnvironmentProcessWeb) {
		t.Fatal("web must remain the only Caddy/host-publish ingress process")
	}
	if models.IsLongRunningProcessKind(models.EnvironmentProcessRelease) {
		t.Fatal("release must not create a long-running Instance")
	}
}

func TestLongRunningProcessesIncludesService(t *testing.T) {
	t.Parallel()

	state := models.EnvironmentDesiredState{
		Processes: []models.EnvironmentProcessState{
			{Name: "web", Kind: models.EnvironmentProcessWeb},
			{Name: "ssr-worker", Kind: models.EnvironmentProcessService},
			{Name: "worker-1", Kind: models.EnvironmentProcessWorker},
			{Name: "release", Kind: models.EnvironmentProcessRelease},
		},
	}
	got := make([]string, 0, 3)
	for _, process := range state.LongRunningProcesses() {
		got = append(got, process.Name)
	}
	want := []string{"web", "ssr-worker", "worker-1"}
	if !slices.Equal(got, want) {
		t.Fatalf("LongRunningProcesses() names = %q, want %q", got, want)
	}
}
