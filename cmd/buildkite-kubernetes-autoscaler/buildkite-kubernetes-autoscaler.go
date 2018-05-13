package main

import (
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/buildkite/go-buildkite/buildkite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const DEFAULT_MINIMUM_DAYS = 45

type HostResult struct {
	Host  string
	Certs []x509.Certificate
}

func main() {
	fmt.Println("Starting buildkite autoscaling")
	kubernetesClient := kubernetesClient()
	buildkiteClient := buildkiteClient()
	var scaleDownCounter time.Time

	// TODO: Implement quit ability
	nextTime := time.Now().Truncate(time.Minute)
	for {
		nextTime = nextTime.Add(time.Minute)
		performDesiredReplicaEvaluation(kubernetesClient, buildkiteClient, &scaleDownCounter)
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

// TODO: Split up nicely
// TODO: Configurable scale up / down values
func performDesiredReplicaEvaluation(kubernetesClient *kubernetes.Clientset, buildkiteClient *buildkite.Client, scaleDownCounter *time.Time) {
	// Get build counts from Buildkite
	buildListOptions := &buildkite.BuildsListOptions{
		State: []string{ "running", "scheduled" },
	}
	builds, _, err := buildkiteClient.Builds.List(buildListOptions)
	check(err)	

	runningBuilds := 0
	scheduledBuilds := 0
	for _, build := range builds {
		if *build.State == "running" {
			runningBuilds += 1
		} else if *build.State == "scheduled" {
			scheduledBuilds += 1
		} else {
			fmt.Fprintln(os.Stderr, "Unexpected build State value")
			os.Exit(1)
		}
	}
	
	// Get current replica count
	targetDeploymentName := os.Getenv("TARGET_DEPLOYMENT_NAME")
	deployment, err := kubernetesClient.AppsV1().Deployments("buildkite").Get(targetDeploymentName, metav1.GetOptions{})
	check(err)
	currentReplicas := int(deployment.Status.Replicas)
	
	fmt.Printf("Current status: %d running, %d scheduled, %d current replicas", runningBuilds, scheduledBuilds, currentReplicas)

	// Make adjustments
	// If anything is running or scheduled, ensure we have enough
	// If nothing is running, slowly scale down over time
	var neededReplicas = int(scheduledBuilds + runningBuilds)
	var targetReplicas = int(currentReplicas)
	if (runningBuilds > 0 || scheduledBuilds > 0) {
		if (neededReplicas > currentReplicas) {
			targetReplicas = neededReplicas
			fmt.Printf("Scaling up to the needed replica count...\n")
		}
	} else if (scaleDownCounter == nil) {
		*scaleDownCounter = time.Now()
		fmt.Printf("Beginning cool down period to scale down replicas...\n")
	} else {
		coolDownLength := int(time.Now().Sub(*scaleDownCounter).Seconds())
		fmt.Printf("Now %d seconds out of %d into cool down period\n", coolDownLength, 300)

		if (coolDownLength > 300) { // 300 is scale down rate
			targetReplicas = currentReplicas - 20 // 20 is scale down size
			scaleDownCounter = nil
			fmt.Printf("Scaling down replicas due to no jobs scheduled or running for cool down period...\n")
		} else {
			fmt.Printf("Waiting cool down period to scale down replicas...\n")
		}
	}

	minReplicas := 1
	maxReplicas := 50
	if targetReplicas > maxReplicas {
		targetReplicas = maxReplicas
	} else if targetReplicas < minReplicas {
		targetReplicas = minReplicas
	}

	deployment.Spec.Replicas = int32Ptr(int32(targetReplicas))
	_, updateErr := kubernetesClient.AppsV1().Deployments("buildkite").Update(deployment)
	if (updateErr != nil) {
		fmt.Fprintln(os.Stderr, updateErr)
	}
}

func int32Ptr(i int32) *int32 { return &i }