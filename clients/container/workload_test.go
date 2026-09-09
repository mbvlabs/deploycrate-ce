package container

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestPrepareWorkloadRunAliasesServiceWithoutPublish(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareWorkloadRun(
		validWorkloadRunSpec(t, "ssr-worker", "service", "1", 13714),
	)
	if err != nil {
		t.Fatalf("PrepareWorkloadRun() error = %v", err)
	}
	if !containsSequence(prepared.Arguments, "--network-alias", "ssr-worker") {
		t.Fatalf(
			"PrepareWorkloadRun() arguments = %q, want --network-alias ssr-worker",
			prepared.Arguments,
		)
	}
	if slices.Contains(prepared.Arguments, "--publish") {
		t.Fatalf("PrepareWorkloadRun() arguments = %q, want no --publish", prepared.Arguments)
	}
}

func TestPrepareWorkloadRunAliasesWebWithPublish(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareWorkloadRun(validWorkloadRunSpec(t, "web", "web", "1", 8080))
	if err != nil {
		t.Fatalf("PrepareWorkloadRun() error = %v", err)
	}
	if !containsSequence(prepared.Arguments, "--network-alias", "web") {
		t.Fatalf("PrepareWorkloadRun() arguments = %q, want --network-alias web", prepared.Arguments)
	}
	if !slices.Contains(prepared.Arguments, "--publish") {
		t.Fatalf("PrepareWorkloadRun() arguments = %q, want --publish", prepared.Arguments)
	}
}

func TestPrepareWorkloadRunLeavesWorkerDark(t *testing.T) {
	t.Parallel()

	prepared, err := PrepareWorkloadRun(validWorkloadRunSpec(t, "worker-1", "worker", "1", 0))
	if err != nil {
		t.Fatalf("PrepareWorkloadRun() error = %v", err)
	}
	if slices.Contains(prepared.Arguments, "--network-alias") {
		t.Fatalf("PrepareWorkloadRun() arguments = %q, want no --network-alias", prepared.Arguments)
	}
	if slices.Contains(prepared.Arguments, "--publish") {
		t.Fatalf("PrepareWorkloadRun() arguments = %q, want no --publish", prepared.Arguments)
	}
}

func TestPrepareWorkloadRunSharesServiceAliasAcrossReplicas(t *testing.T) {
	t.Parallel()

	first := validWorkloadRunSpec(t, "ssr-worker", "service", "1", 13714)
	second := validWorkloadRunSpec(t, "ssr-worker", "service", "2", 13714)
	preparedFirst, err := PrepareWorkloadRun(first)
	if err != nil {
		t.Fatalf("PrepareWorkloadRun() first replica error = %v", err)
	}
	preparedSecond, err := PrepareWorkloadRun(second)
	if err != nil {
		t.Fatalf("PrepareWorkloadRun() second replica error = %v", err)
	}
	if !containsSequence(preparedFirst.Arguments, "--network-alias", "ssr-worker") {
		t.Fatalf(
			"PrepareWorkloadRun() first replica arguments = %q, want --network-alias ssr-worker",
			preparedFirst.Arguments,
		)
	}
	if !containsSequence(preparedSecond.Arguments, "--network-alias", "ssr-worker") {
		t.Fatalf(
			"PrepareWorkloadRun() second replica arguments = %q, want --network-alias ssr-worker",
			preparedSecond.Arguments,
		)
	}
}

func TestPrepareWorkloadRunRejectsUnsafeNetworkAlias(t *testing.T) {
	t.Parallel()

	spec := validWorkloadRunSpec(t, "ssr-worker", "service", "1", 13714)
	spec.ProcessName = "-ssr-worker"
	if _, err := PrepareWorkloadRun(spec); err == nil {
		t.Fatal("PrepareWorkloadRun() accepted a process name that is not a Docker network alias")
	}
}

func validWorkloadRunSpec(
	t *testing.T,
	processName, processKind, replica string,
	containerPort int32,
) WorkloadRunSpec {
	t.Helper()
	return WorkloadRunSpec{
		ApplicationID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		EnvironmentID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TargetID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		DeploymentID:   uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		InstanceID:     uuid.New(),
		ReleaseID:      uuid.MustParse("55555555-5555-5555-5555-555555555555"),
		ProcessName:    processName,
		ProcessKind:    processKind,
		ProcessReplica: replica,
		ContainerName: "app-env-" + processName + "-" + replica + "-" +
			uuid.MustParse("66666666-6666-6666-6666-666666666666").String(),
		ImageReference: "registry.example/app@sha256:" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		NetworkName:   "dc-env-22222222-2222-2222-2222-222222222222",
		RestartPolicy: "unless-stopped",
		ContainerPort: containerPort,
	}
}

func containsSequence(arguments []string, sequence ...string) bool {
	for index := 0; index+len(sequence) <= len(arguments); index++ {
		if slices.Equal(arguments[index:index+len(sequence)], sequence) {
			return true
		}
	}
	return false
}
