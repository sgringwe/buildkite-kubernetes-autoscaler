package main

import (
	"fmt"
	"os"
	"time"
	"strconv"

	"github.com/buildkite/go-buildkite/buildkite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const DEFAULT_MINIMUM_DAYS = 45

type AutoscalingStatus struct {
	ScaleDownStart time.Time
	Status string // unknown, cooling, correct
}

func main() {
	fmt.Println("Starting buildkite autoscaling")
	kubernetesClient := kubernetesClient()
	buildkiteClient := buildkiteClient()
	var autoscalingStatus AutoscalingStatus
	autoscalingStatus.Status = "unknown"

	// Evaluate every 30 seconds
	evaluateTicker := time.NewTicker(30 * time.Second)

	// TODO: Implement quit ability
	for {
		select {
		case <-evaluateTicker.C:
			performDesiredReplicaEvaluation(kubernetesClient, buildkiteClient, &autoscalingStatus)
		}
	}
}

func kubernetesClient() *kubernetes.Clientset {
	var err error
	var config *rest.Config
	config, err = rest.InClusterConfig()
	check(err)
	client, err := kubernetes.NewForConfig(config)
	check(err)
	return client
}

func buildkiteClient() *buildkite.Client {
	buildkiteApiToken := os.Getenv("BUILDKITE_API_TOKEN")

	config, err := buildkite.NewTokenConfig(buildkiteApiToken, false)
	check(err)
	client := buildkite.NewClient(config.Client())

	return client
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildkiteInformation(buildkiteClient *buildkite.Client) (int, int) {
	// Get build counts from Buildkite
	buildListOptions := &buildkite.BuildsListOptions{
		State: []string{ "running", "scheduled" },
	}
	
	builds, _, err := buildkiteClient.Builds.List(buildListOptions)
	check(err)	

	runningBuilds := 0
	scheduledBuilds := 0
	for _, build := range builds {
		for _, job := range build.Jobs {
			if job.State == nil {
				continue
			}

			if *job.State == "running" {
				runningBuilds += 1
			} else if *job.State == "scheduled" {
				scheduledBuilds += 1
			}
		}
	}

	return runningBuilds, scheduledBuilds
}

func performDesiredReplicaEvaluation(kubernetesClient *kubernetes.Clientset, buildkiteClient *buildkite.Client, autoscalingStatus *AutoscalingStatus) {
	runningBuilds, scheduledBuilds := buildkiteInformation(buildkiteClient)
	
	// Get current replica count
	targetDeploymentName := os.Getenv("TARGET_DEPLOYMENT_NAME")
	targetDeploymentNamespace := os.Getenv("TARGET_DEPLOYMENT_NAMESPACE")
	deployment, err := kubernetesClient.AppsV1().Deployments(targetDeploymentNamespace).Get(targetDeploymentName, metav1.GetOptions{})
	check(err)
	currentReplicas := int(deployment.Status.Replicas)
	
	fmt.Printf("Current status: Autoscaler: %s; Builds: %d running, %d scheduled; Deployment: %d current replicas\n", autoscalingStatus.Status, runningBuilds, scheduledBuilds, currentReplicas)

	// Begin th main logic for determine the right replica count.
	var neededReplicas = int(scheduledBuilds + runningBuilds)
	var targetReplicas = int(currentReplicas)
	if (runningBuilds > 0 || scheduledBuilds > 0) {
		// Builds are currently running or are queued and need agents. If there is room to add
		// more replicas, target what we need. Otherwise we are at the right number of replicas, the
		// maximum.
		if (currentReplicas < maxReplicas() && neededReplicas > currentReplicas) {
			autoscalingStatus.Status = "correct"
			targetReplicas = neededReplicas
			fmt.Printf("Scaling up to %d replicas or maximum.\n", targetReplicas)
		} else {
			autoscalingStatus.Status = "correct"
		}
	} else if (autoscalingStatus.Status != "cooling") {
		// We have 0 running or scheduled builds. Let's start cool down period to begin scaling down.
		autoscalingStatus.Status = "cooling"
		autoscalingStatus.ScaleDownStart = time.Now()
		fmt.Printf("Beginning cool down period to scale down replicas...\n")
	} else {
		// We are already in cool down. Check if we have waited long enough, and if we have
		// scale down some replicas.
		coolDownLength := int(time.Now().Sub(autoscalingStatus.ScaleDownStart).Seconds())
		fmt.Printf("Now %d seconds out of %d into cool down period\n", coolDownLength, scaleDownFrequency())

		if (coolDownLength > scaleDownFrequency()) {
			targetReplicas = currentReplicas - scaleDownSize()
			autoscalingStatus.ScaleDownStart = time.Now()
			fmt.Printf("Scaling down replicas due to no jobs scheduled or running for cool down period...\n")
		}
	}

	if targetReplicas > maxReplicas() {
		targetReplicas = maxReplicas()
	} else if targetReplicas < minReplicas() {
		targetReplicas = minReplicas()
	}

	if targetReplicas != currentReplicas {
		deployment.Spec.Replicas = int32Ptr(int32(targetReplicas))
		_, updateErr := kubernetesClient.AppsV1().Deployments(targetDeploymentNamespace).Update(deployment)
		if (updateErr != nil) {
			fmt.Fprintln(os.Stderr, updateErr)
		}
	}
}

func minReplicas() (int) {
	rv, err := strconv.Atoi(os.Getenv("MINIMUM_REPLICAS"))
	check(err)
	return rv
}

func maxReplicas() (int) {
	rv, err := strconv.Atoi(os.Getenv("MAXIMUM_REPLICAS"))
	check(err)
	return rv
}

func scaleDownSize() (int) {
	rv, err := strconv.Atoi(os.Getenv("SCALE_DOWN_SIZE"))
	if (err != nil) {
		rv = 20 // default
	}
	return rv
}

func scaleDownFrequency() (int) {
	rv, err := strconv.Atoi(os.Getenv("SCALE_DOWN_FREQUENCY"))
	if (err != nil) {
		rv = 300 // default
	}
	return rv
}

func int32Ptr(i int32) *int32 { return &i }