package main

import (
	"context"
	"encoding/json"
	"flag"
	"k8s-container-log-filter/pkg/containerlogfilter"
	"log"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	startTime := time.Now()
	kubeConfigPath, logRequestsFile, timeout, sinceSeconds := parseArgs()
	kubeCli, err := initKubeClient(kubeConfigPath)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client client: %v\n", err)
	}

	log.Default().Printf("Timeout set to %d minutes", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Minute)
	defer cancel()

	log.Default().Printf("Reading log requests data from %s file", logRequestsFile)
	logRequests, err := readInputDataAndUnmarshal(logRequestsFile)
	if err != nil {
		log.Fatalf("Failed to read log requests data: %v", err)
		return
	}
	log.Default().Printf("Logs filtered back %d hours", sinceSeconds/60/60)
	clf := containerlogfilter.New(*kubeCli, logRequests, sinceSeconds)
	clf.Run(ctx)
	executionTime := time.Now().Sub(startTime)
	log.Default().Printf("Program finished in %s", executionTime)
}

func initKubeClient(kubeConfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func readInputDataAndUnmarshal(fileName string) (containerlogfilter.LogRequestsObject, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return containerlogfilter.LogRequestsObject{}, err
	}
	var logRequests containerlogfilter.LogRequestsObject
	err = json.Unmarshal(data, &logRequests)
	if err != nil {
		return containerlogfilter.LogRequestsObject{}, err
	}
	return logRequests, nil
}

func parseArgs() (string, string, int, int64) {
	var kubeConfigPath, logRequestsFile string
	var timeout int
	var sinceSeconds int64

	flag.StringVar(&kubeConfigPath, "kubeconfig", "", "Path to kubeconcfig file")
	flag.IntVar(&timeout, "timeout", 2, "Timeout in minutes. Default value is 2 minutes")
	flag.StringVar(&logRequestsFile, "log_requests_file", "log_requests.json", "Path to the file with log requests definition")
	flag.Int64Var(&sinceSeconds, "since_seconds", 86400, " Tells how old logs should be filtered")

	flag.Parse()
	if kubeConfigPath == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}
	return kubeConfigPath, logRequestsFile, timeout, sinceSeconds
}
