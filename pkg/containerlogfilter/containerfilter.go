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
	"sync/atomic"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
)

type ContainterLogFilter struct {
	kubeClient                kubernetes.Clientset
	logRequestsObj            LogRequestsObject
	sinceSeconds              *int64
	numberOfContainersScanned atomic.Int32
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

		messagesRegex, err := listOfMessagesToRegex(logRequest.Messages)
		if err != nil {
			log.Default().Printf("Can't compile regex for %s: %v", logRequest.Namespace, err)
			continue
		}

		for podNameRegex := range logRequest.PodNameRegex {
			wg.Add(1)
			go func(logRequest LogRequest, podNameRegexStr string) {
				defer wg.Done()
				podNameRegex, err := regexp.Compile(podNameRegexStr)
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
							MessageRegex:  messagesRegex,
						}
						go func() {
							defer wgContainers.Done()
							c.getLogAndWriteToFile(ctx, containerLogReq)
						}()
					}
				}
				wgContainers.Wait()
			}(logRequest, podNameRegex)
		}
	}
	wg.Wait()
	log.Default().Printf("Number of checked containers %d", c.numberOfContainersScanned.Load())
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
				PodNameRegex: sets.Set[string](sets.NewString(logRequest.PodNameRegex)),
				Messages:     sets.Set[string](sets.NewString(logRequest.Messages...)),
			}
			continue
		}

		existingLogRequest.Messages = existingLogRequest.Messages.Union(sets.New[string](logRequest.Messages...))
		existingLogRequest.PodNameRegex = existingLogRequest.PodNameRegex.Union(sets.New[string](logRequest.PodNameRegex))
		mapNamespaceToLogRequest[logRequest.Namespace] = existingLogRequest
	}
	return mapNamespaceToLogRequest
}

func (c *ContainterLogFilter) getAndFilterContainerLogs(ctx context.Context, containerLogRequest ContainerLogRequest) (string, error) {
	req := c.kubeClient.CoreV1().Pods(containerLogRequest.Namespace).GetLogs(containerLogRequest.PodName, &corev1.PodLogOptions{
		Container:    containerLogRequest.ContainerName,
		SinceSeconds: c.sinceSeconds,
		Timestamps:   true,
	})
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()
	scanner := bufio.NewScanner(stream)
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Bytes()
		if containerLogRequest.MessageRegex.Match(line) {
			line = append(line, '\n')
			_, err = sb.Write(line)
			if err != nil {
				log.Default().Printf("Failed to write line for container %s in the %s: %v",
					containerLogRequest.ContainerName, containerLogRequest.Namespace, err)
			}
		}
	}
	return sb.String(), nil
}

func (c *ContainterLogFilter) getLogAndWriteToFile(ctx context.Context, containerLogReq ContainerLogRequest) {
	stringData, err := c.getAndFilterContainerLogs(ctx, containerLogReq)
	if err != nil {
		log.Default().Printf("Can't read the container logs for namespace %s and container %s: %v\n",
			containerLogReq.Namespace, containerLogReq.ContainerName, err)
		return
	}
	c.numberOfContainersScanned.Add(1)
	if len(stringData) == 0 {
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
}

func listOfMessagesToRegex(messages sets.Set[string]) (*regexp.Regexp, error) {
	regexStr := messages.UnsortedList()[0]
	if len(messages) == 1 {
		return regexp.Compile(regexStr)
	}

	for m := range messages {
		regexStr = fmt.Sprintf("%s|%s", regexStr, m)
	}
	return regexp.Compile(regexStr)
}
