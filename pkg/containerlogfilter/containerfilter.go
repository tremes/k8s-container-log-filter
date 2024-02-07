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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
)

type ContainterLogFilter struct {
	kubeClient     kubernetes.Clientset
	logRequestsObj LogRequestsObject
	sinceSeconds   *int64
}

func New(kubeClient kubernetes.Clientset, logRequests LogRequestsObject, sinceSeconds int64) *ContainterLogFilter {
	return &ContainterLogFilter{
		kubeClient:     kubeClient,
		logRequestsObj: logRequests,
		sinceSeconds:   &sinceSeconds,
	}
}

func (c *ContainterLogFilter) Run(ctx context.Context) {
	namespaceToLogRequest := c.createNamespaceToLogRequestMap()

	var wg sync.WaitGroup
	for _, logRequest := range namespaceToLogRequest {
		log.Default().Printf("Start checking namespace %s for the Pod name pattern %s\n", logRequest.Namespace, logRequest.PodNameRegex)
		wg.Add(1)
		go func(logRequest LogRequest) {
			defer wg.Done()
			podNameRegex, err := regexp.Compile(logRequest.PodNameRegex)
			if err != nil {
				log.Fatalf("Failed to compile Pod name regular expression %s for the namespace %s: %v\n",
					logRequest.PodNameRegex, logRequest.Namespace, err)
				return
			}
			podToContainers, err := c.createPodToContainersMap(ctx, logRequest.Namespace, *podNameRegex)
			if err != nil {
				log.Fatalf("Failed to get matching pod names for namespace %s: %v\n", logRequest.Namespace, err)
				return
			}

			var wgContainers sync.WaitGroup
			for podName, containersaNames := range podToContainers {
				wgContainers.Add(len(containersaNames))
				for _, container := range containersaNames {
					containerLogReq := ContainerLogRequest{
						Namespace:     logRequest.Namespace,
						ContainerName: container,
						PodName:       podName,
						Messages:      logRequest.Messages,
					}
					go func(containerLogReq ContainerLogRequest) {
						defer wgContainers.Done()
						stringData, err := c.getAndFilterContainerLogs(ctx, containerLogReq)
						if err != nil {
							log.Default().Printf("Can't read the container logs for namespace %s and container %s: %v\n",
								containerLogReq.Namespace, containerLogReq.ContainerName, err)
							return
						}
						if len(stringData) == 0 {
							//log.Default().Printf("Not found anything in the namespace %s for Pod name %s", namespace, podName)
							return
						}
						path := fmt.Sprintf("log_data/%s/%s/%s.log",
							containerLogReq.Namespace, containerLogReq.PodName, containerLogReq.ContainerName)
						dirPath := fmt.Sprintf("log_data/%s/%s/", containerLogReq.Namespace, containerLogReq.PodName)
						err = os.MkdirAll(dirPath, os.ModePerm)
						if err != nil {
							log.Fatalf("failed to create output dir: %v", err)
						}
						err = fileUtils.WriteToFile(path, stringData)
						if err != nil {
							log.Fatalf("failed to write to file %s: %v", path, err)
						}

					}(containerLogReq)
				}
			}
			wgContainers.Wait()

		}(logRequest)
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

// createNamespaceToLogRequestMap creates and returns a map that maps a namespace name to a specific log request.
// If there are two or more log requests for the same namespace,
// the log request attributes (such as Pod name regex, Messages) are aggregated into a single log request.
func (c *ContainterLogFilter) createNamespaceToLogRequestMap() map[string]LogRequest {
	mapNamespaceToLogRequest := make(map[string]LogRequest)
	for _, logRequest := range c.logRequestsObj.LogRequests {
		existingLogRequest, ok := mapNamespaceToLogRequest[logRequest.Namespace]

		if !ok {
			mapNamespaceToLogRequest[logRequest.Namespace] = LogRequest{
				Namespace:    logRequest.Namespace,
				PodNameRegex: logRequest.PodNameRegex,
				Messages:     sets.Set[string](sets.NewString(logRequest.Messages...)),
			}
			continue
		}

		existingLogRequest.Messages = existingLogRequest.Messages.Union(sets.New[string](logRequest.Messages...))
		existingLogRequest.PodNameRegex = fmt.Sprintf("%s|%s", existingLogRequest.PodNameRegex, logRequest.PodNameRegex)
		mapNamespaceToLogRequest[logRequest.Namespace] = existingLogRequest
	}
	return mapNamespaceToLogRequest
}

func (c *ContainterLogFilter) getAndFilterContainerLogs(ctx context.Context, containerLogRequest ContainerLogRequest) ([]string, error) {
	req := c.kubeClient.CoreV1().Pods(containerLogRequest.Namespace).GetLogs(containerLogRequest.PodName, &corev1.PodLogOptions{
		Container:    containerLogRequest.ContainerName,
		SinceSeconds: c.sinceSeconds,
		Timestamps:   true,
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
		if stringInSlice(containerLogRequest.Messages, text) {
			matchedLines = append(matchedLines, text)
		}
	}

	return matchedLines, nil
}

func stringInSlice(set sets.Set[string], s string) bool {
	for str := range set {
		if strings.Contains(s, str) {
			return true
		}
	}
	return false
}
