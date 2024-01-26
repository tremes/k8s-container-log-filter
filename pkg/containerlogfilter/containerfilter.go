package containerlogfilter

import (
	"bufio"
	"context"
	"fmt"
	fileUtils "k8s-container-log-filter/pkg/fileutils"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type ContainterLogFilter struct {
	kubeClient     kubernetes.Clientset
	logRequestsObj LogRequestsObject
}

func New(kubeClient kubernetes.Clientset, logRequests LogRequestsObject) *ContainterLogFilter {
	return &ContainterLogFilter{
		kubeClient:     kubeClient,
		logRequestsObj: logRequests,
	}
}

func (c *ContainterLogFilter) Run(ctx context.Context) {
	namespaceToAggregatedData := c.createNamespaceToAggregatedDataMap()

	var wg sync.WaitGroup
	for namespace, aggregatedData := range namespaceToAggregatedData {
		log.Default().Printf("Start checking namespace %s for the Pod name pattern %s\n", namespace, aggregatedData.PodNameRegExpr)
		wg.Add(1)
		go func(namespace string, aggregatedData AggregatedNamespaceData) {
			defer wg.Done()
			podNameRegex := regexp.MustCompile(aggregatedData.PodNameRegExpr)
			podToContainers, err := c.createPodToContainersMap(ctx, namespace, *podNameRegex)
			if err != nil {
				fmt.Printf("Failed to get matching pod names: %v\n", err)
				return
			}

			var wgContainers sync.WaitGroup
			for podName, containersaNames := range podToContainers {
				wgContainers.Add(len(containersaNames))
				for _, container := range containersaNames {
					go func(namespace, podName, containerName string, messages []string) {
						defer wgContainers.Done()
						stringData, err := c.getAndFilterContainerLogs(ctx, namespace, podName, containerName, messages)
						if err != nil {
							log.Fatalf("Can't read the container logs for namespace %s and container %s: %v\n", namespace, containerName, err)
							return
						}
						if len(stringData) == 0 {
							//log.Default().Printf("Not found anything in the namespace %s for Pod name %s", namespace, podName)
							return
						}
						path := fmt.Sprintf("log_data/%s/%s/%s", namespace, podName, containerName)
						dirPath := fmt.Sprintf("log_data/%s/%s/", namespace, podName)
						err = os.MkdirAll(dirPath, os.ModePerm)
						if err != nil {
							log.Fatalf("failed to create output dir: %v", err)
						}
						err = fileUtils.WriteToFile(path, stringData)
						if err != nil {
							log.Fatalf("failed to write to file %s: %v", path, err)
						}

					}(namespace, podName, container, aggregatedData.Messages)
				}
			}
			wgContainers.Wait()

		}(namespace, aggregatedData)
	}
	wg.Wait()
}

// createPodToContainersMap lists all the Pods in the provided namespace
// and checks whether each Pod name matches the provided regular expression.
// If there is a match then add all the Pod related containers to the map.
// It returns map when key is the Pod name and the value is a slice of container names.
func (c *ContainterLogFilter) createPodToContainersMap(ctx context.Context, namespace string, podNameRegex regexp.Regexp) (map[string][]string, error) {
	podContainers := make(map[string][]string)

	podList, err := c.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, pod := range podList.Items {
		if podNameRegex.MatchString(pod.Name) {
			var containerNames []string
			for _, c := range pod.Spec.Containers {
				containerNames = append(containerNames, c.Name)
			}
			podContainers[pod.Name] = containerNames
		}
	}

	return podContainers, nil
}

func (c *ContainterLogFilter) createNamespaceToAggregatedDataMap() map[string]AggregatedNamespaceData {
	mapNamespaceToRegex := make(map[string]AggregatedNamespaceData)
	for _, logRequest := range c.logRequestsObj.LogRequests {
		aggregatedData := mapNamespaceToRegex[logRequest.Namespace]
		// TODO: use set for the messages to avoid duplicates?
		aggregatedData.Messages = append(aggregatedData.Messages, logRequest.Messages...)
		if aggregatedData.PodNameRegExpr == "" {
			aggregatedData.PodNameRegExpr = logRequest.PodNameRegExpr.String()
		} else {
			aggregatedData.PodNameRegExpr = fmt.Sprintf("%s|%s", aggregatedData.PodNameRegExpr, logRequest.PodNameRegExpr.String())
		}
		mapNamespaceToRegex[logRequest.Namespace] = aggregatedData
	}
	return mapNamespaceToRegex
}

func (c *ContainterLogFilter) getAndFilterContainerLogs(ctx context.Context, namespace, podName, containerName string, messages []string) ([]string, error) {
	sinceSeconds := int64(86400)
	req := c.kubeClient.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:    containerName,
		SinceSeconds: &sinceSeconds,
	})
	reader, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(reader)

	var matchedLines []string
	for scanner.Scan() {
		text := scanner.Text()
		// TODO match regex instead ??
		if stringInSlice(messages, text) {
			matchedLines = append(matchedLines, text)
		}
	}

	return matchedLines, nil
}

func stringInSlice(slc []string, s string) bool {
	for _, slcStr := range slc {
		if strings.Contains(s, slcStr) {
			return true
		}
	}
	return false
}
