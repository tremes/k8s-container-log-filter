# Kubernetes Container Log Filter
This is a simple tool that helps you filter and collect specific messages from Kubernetes containers. The requests (for container logs and particular messages) are specified as log requests in the JSON format as:

```json
{
    "log_requests": [
        {
            "namespace": "openshift-monitoring",
            "pod_name_regex": "prometheus",
            "messages": [
                "Deleting obsolete block",
                "successfully synced all caches",
                "UpdatingNodeExporter failed"
            ]
        },
        {
            "namespace": "kube-apiserver",
            "pod_name_regex": "apiserver",
            "messages": [
                "Write to database call failed"
            ]
        }
    ]
}

```

Log request is identified by the namespace name, Pod name (as regular expression) and the list of messages to be filtered. 

## Build

You can build the binary with:

```
make build
```

This creates the `k8s-container-log-filter` binary in the same directory. 

## Running

Valid connection to a Kubernetes cluster is required and by default it reads your `${HOME}/.kube/config` file for the connection. So you can simply run:

```
./k8s-container-log-filter
```

### Arguments

There are following arguments:

* `-kubeconfig` - to override the default kubeconfig path `${HOME}/.kube/config`
* `-timeout` - to override the default timeout of **2 minutes**
* `-since_hours` - to override the default parameter of 24 hours - how long history of a container log to filter 
* `-log_requests_file` - to override the default input file `log_requests.json` (expected in the same directory)

## Output

The output data is written to the same directory in `log_data` directory. The structure of the ouput data is the following:

```
log_data/<namespace_name>/<pod_name>/<container_name>.log
```